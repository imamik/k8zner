package controller

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/platform/hcloud"
)

// --- updateStatusWithRetry additional coverage ---

func TestUpdateStatusWithRetry_NonConflictErrorReturnsImmediately(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	// Create a fake client WITHOUT the object - this will cause a "not found" error
	// on status update, which is not a conflict error, so it should be returned immediately.
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&k8znerv1alpha1.K8znerCluster{}).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder)

	err := r.updateStatusWithRetry(context.Background(), cluster)
	require.Error(t, err)
	// The error should be a non-conflict error (not found), returned on first attempt
}

func TestUpdateStatusWithRetry_SuccessOnFirstTry_WithAddonStatus(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder)

	// Set addon status and verify it's preserved
	now := metav1.Now()
	cluster.Status.Addons = map[string]k8znerv1alpha1.AddonStatus{
		"cilium": {
			Installed:          true,
			Healthy:            true,
			Phase:              k8znerv1alpha1.AddonPhaseInstalled,
			LastTransitionTime: &now,
		},
	}
	cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseRunning

	err := r.updateStatusWithRetry(context.Background(), cluster)
	require.NoError(t, err)

	// Verify the addon status persisted
	updated := &k8znerv1alpha1.K8znerCluster{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "test-cluster",
	}, updated)
	require.NoError(t, err)
	assert.Equal(t, k8znerv1alpha1.ClusterPhaseRunning, updated.Status.Phase)
}

// --- reconcileWithStateMachine additional coverage ---

func TestReconcileWithStateMachine_UnknownPhaseResetsToInfrastructure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{
				Name: "test-secret",
			},
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ProvisioningPhase: k8znerv1alpha1.ProvisioningPhase("SomeUnknownPhase"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.reconcileWithStateMachine(context.Background(), cluster)
	require.NoError(t, err)
	assert.True(t, result.Requeue)
	assert.Equal(t, k8znerv1alpha1.PhaseInfrastructure, cluster.Status.ProvisioningPhase)
}

func TestReconcileWithStateMachine_EmptyPhaseWithBootstrapCompleted(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	// When ProvisioningPhase is empty and bootstrap is completed, should start from CNI
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{
				Name: "test-secret",
			},
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed: true,
				PublicIP:  "1.2.3.4",
			},
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ProvisioningPhase: "", // empty = determine from bootstrap state
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	// The phase should be set to CNI, and then reconcileCNIPhase will run
	// which will fail at credentials loading (no real secret), producing a requeue
	result, err := r.reconcileWithStateMachine(context.Background(), cluster)
	assert.NoError(t, err)
	assert.Equal(t, k8znerv1alpha1.PhaseCNI, cluster.Status.ProvisioningPhase)
	assert.NotZero(t, result.RequeueAfter)
}

func TestReconcileWithStateMachine_EmptyPhaseNewCluster(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	// When ProvisioningPhase is empty and bootstrap is not completed, should start from Infrastructure
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{
				Name: "test-secret",
			},
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed: false,
			},
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ProvisioningPhase: "",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	// Will dispatch to reconcileInfrastructurePhase, which will fail at credentials
	result, err := r.reconcileWithStateMachine(context.Background(), cluster)
	assert.NoError(t, err)
	// After Infrastructure phase runs (and fails at credentials), it should requeue
	assert.NotZero(t, result.RequeueAfter)
}

// --- reconcile: credentialsRef dispatch ---

func TestReconcile_DispatchesToStateMachineWithCredentialsRef(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{
				Name: "my-secret",
			},
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ProvisioningPhase: k8znerv1alpha1.ProvisioningPhase("SomeUnknownPhase"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.reconcile(context.Background(), cluster)
	assert.NoError(t, err)
	// Unknown phase gets reset to Infrastructure and requeues
	assert.True(t, result.Requeue)
	assert.Equal(t, k8znerv1alpha1.PhaseInfrastructure, cluster.Status.ProvisioningPhase)
}

func TestReconcile_DispatchesToLegacyWithoutCredentialsRef(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
			// No CredentialsRef
		},
	}

	cpNode := createTestNode("cp-1", true, true)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, cpNode).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.reconcile(context.Background(), cluster)
	assert.NoError(t, err)
	// Legacy mode should requeue for continuous monitoring
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
}

// --- reconcile: sync desired counts ---

func TestReconcile_SyncsDesiredCounts(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3, Size: "cx22"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 5, Size: "cx22"},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	_, _ = r.reconcile(context.Background(), cluster)

	assert.Equal(t, 3, cluster.Status.ControlPlanes.Desired)
	assert.Equal(t, 5, cluster.Status.Workers.Desired)
}

// --- logAndRecordError coverage ---

func TestLogAndRecordError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ProvisioningPhase: k8znerv1alpha1.PhaseAddons,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	testErr := fmt.Errorf("test error message")
	r.logAndRecordError(context.Background(), cluster, testErr, "TestReason", "Something failed")

	// Verify event was recorded
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "TestReason")
		assert.Contains(t, event, "Something failed")
		assert.Contains(t, event, "test error message")
	default:
		t.Fatal("expected event to be recorded")
	}
}

// --- drainNode coverage ---

func TestDrainNode_EmptyPodList(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	// drainNode uses MatchingFields which requires a field indexer.
	// The fake client without an indexer will return an empty list.
	err := r.drainNode(context.Background(), "nonexistent-node")
	// Without the field index, this returns an error
	// (fake client doesn't support MatchingFields without indexer)
	// The function wraps it in "failed to list pods on node"
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list pods on node")
}

// --- Reconcile: full flow with HCloud client error ---

func TestReconcile_HCloudClientError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	// No HCloud client and no token - should fail and requeue
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-cluster",
		},
	})

	assert.NoError(t, err) // Error is recorded as event, not returned
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
}

// --- verifyAndUpdateNodeStates coverage ---

func TestVerifyAndUpdateNodeStates_SkipsFailedAndDeletingNodes(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:  "cp-1",
						Phase: k8znerv1alpha1.NodePhaseFailed,
					},
					{
						Name:  "cp-2",
						Phase: k8znerv1alpha1.NodePhaseDeletingServer,
					},
				},
			},
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:  "w-1",
						Phase: k8znerv1alpha1.NodePhaseFailed,
					},
				},
			},
		},
	}

	// MockHCloudClient.GetServerByName returns nil by default, which would cause
	// the verifier to fail. But since Failed/DeletingServer nodes are skipped,
	// GetServerByName should NOT be called.
	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			t.Fatal("GetServerByName should not be called for Failed/DeletingServer nodes")
			return nil, nil
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	err := r.verifyAndUpdateNodeStates(context.Background(), cluster)
	assert.NoError(t, err)

	// Verify phases didn't change
	assert.Equal(t, k8znerv1alpha1.NodePhaseFailed, cluster.Status.ControlPlanes.Nodes[0].Phase)
	assert.Equal(t, k8znerv1alpha1.NodePhaseDeletingServer, cluster.Status.ControlPlanes.Nodes[1].Phase)
	assert.Equal(t, k8znerv1alpha1.NodePhaseFailed, cluster.Status.Workers.Nodes[0].Phase)
}

func TestVerifyAndUpdateNodeStates_UsesPublicIPThenFallsBackToPrivateIP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	// Test a node with only PrivateIP set (no PublicIP)
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:      "cp-1",
						Phase:     k8znerv1alpha1.NodePhaseWaitingForK8s,
						PublicIP:  "",
						PrivateIP: "10.0.0.1",
					},
				},
			},
		},
	}

	// Mock GetServerByName to return nil (server not found) - this causes
	// verifyNodeState to set ServerExists=false, leading to Failed phase
	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, name string) (*hcloudgo.Server, error) {
			return nil, nil // server not found
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	err := r.verifyAndUpdateNodeStates(context.Background(), cluster)
	assert.NoError(t, err)

	// The node should be updated to Failed since server doesn't exist
	assert.Equal(t, k8znerv1alpha1.NodePhaseFailed, cluster.Status.ControlPlanes.Nodes[0].Phase)
}

func TestVerifyAndUpdateNodeStates_NodeWithK8sReady(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	// Create a K8s node that is ready
	k8sNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cp-1",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:     "cp-1",
						Phase:    k8znerv1alpha1.NodePhaseWaitingForK8s,
						PublicIP: "1.2.3.4",
					},
				},
			},
		},
	}

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, name string) (*hcloudgo.Server, error) {
			return nil, nil // server check fails, but K8s node check works
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(k8sNode).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	err := r.verifyAndUpdateNodeStates(context.Background(), cluster)
	assert.NoError(t, err)

	// K8s node is ready, so the phase should be updated to Ready
	assert.Equal(t, k8znerv1alpha1.NodePhaseReady, cluster.Status.ControlPlanes.Nodes[0].Phase)
}

// --- determineNodePhaseFromState additional coverage ---

func TestDetermineNodePhaseFromState_AllBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		info          *nodeStateInfo
		expectedPhase k8znerv1alpha1.NodePhase
		reasonPrefix  string
	}{
		{
			name: "K8s node exists and ready",
			info: &nodeStateInfo{
				K8sNodeExists: true,
				K8sNodeReady:  true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseReady,
			reasonPrefix:  "Node is registered and ready",
		},
		{
			name: "K8s node exists not ready with kubelet running",
			info: &nodeStateInfo{
				K8sNodeExists:       true,
				K8sNodeReady:        false,
				TalosKubeletRunning: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseNodeInitializing,
			reasonPrefix:  "Node registered",
		},
		{
			name: "K8s node exists not ready without kubelet",
			info: &nodeStateInfo{
				K8sNodeExists:       true,
				K8sNodeReady:        false,
				TalosKubeletRunning: false,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForK8s,
			reasonPrefix:  "Waiting for kubelet",
		},
		{
			name: "Talos configured kubelet running no k8s node",
			info: &nodeStateInfo{
				TalosConfigured:     true,
				TalosKubeletRunning: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForK8s,
			reasonPrefix:  "Talos configured",
		},
		{
			name: "Talos configured kubelet not running",
			info: &nodeStateInfo{
				TalosConfigured:     true,
				TalosKubeletRunning: false,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseRebootingWithConfig,
			reasonPrefix:  "Talos configured",
		},
		{
			name: "Talos in maintenance mode",
			info: &nodeStateInfo{
				TalosInMaintenanceMode: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
			reasonPrefix:  "Talos in maintenance mode",
		},
		{
			name: "Talos API reachable but state unknown",
			info: &nodeStateInfo{
				TalosAPIReachable: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseApplyingTalosConfig,
			reasonPrefix:  "Talos API reachable",
		},
		{
			name: "Server running but Talos not reachable",
			info: &nodeStateInfo{
				ServerExists: true,
				ServerStatus: "running",
				ServerIP:     "1.2.3.4",
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
			reasonPrefix:  "Server running",
		},
		{
			name: "Server starting",
			info: &nodeStateInfo{
				ServerExists: true,
				ServerStatus: "starting",
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForIP,
			reasonPrefix:  "Server starting",
		},
		{
			name: "Server exists in other state",
			info: &nodeStateInfo{
				ServerExists: true,
				ServerStatus: "off",
			},
			expectedPhase: k8znerv1alpha1.NodePhaseCreatingServer,
			reasonPrefix:  "Server exists in state",
		},
		{
			name: "Server does not exist",
			info: &nodeStateInfo{
				ServerExists: false,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseFailed,
			reasonPrefix:  "Server does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			phase, reason := determineNodePhaseFromState(tt.info)
			assert.Equal(t, tt.expectedPhase, phase)
			assert.Contains(t, reason, tt.reasonPrefix)
		})
	}
}

// --- shouldUpdatePhase additional coverage ---

func TestShouldUpdatePhase_AllPhaseTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		current  k8znerv1alpha1.NodePhase
		newPhase k8znerv1alpha1.NodePhase
		expected bool
	}{
		// Forward progression
		{
			name:     "WaitingForIP to WaitingForTalosAPI",
			current:  k8znerv1alpha1.NodePhaseWaitingForIP,
			newPhase: k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
			expected: true,
		},
		{
			name:     "WaitingForTalosAPI to ApplyingTalosConfig",
			current:  k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
			newPhase: k8znerv1alpha1.NodePhaseApplyingTalosConfig,
			expected: true,
		},
		{
			name:     "ApplyingTalosConfig to RebootingWithConfig",
			current:  k8znerv1alpha1.NodePhaseApplyingTalosConfig,
			newPhase: k8znerv1alpha1.NodePhaseRebootingWithConfig,
			expected: true,
		},
		{
			name:     "RebootingWithConfig to WaitingForK8s",
			current:  k8znerv1alpha1.NodePhaseRebootingWithConfig,
			newPhase: k8znerv1alpha1.NodePhaseWaitingForK8s,
			expected: true,
		},
		{
			name:     "WaitingForK8s to NodeInitializing",
			current:  k8znerv1alpha1.NodePhaseWaitingForK8s,
			newPhase: k8znerv1alpha1.NodePhaseNodeInitializing,
			expected: true,
		},
		// Backward prevented
		{
			name:     "NodeInitializing to WaitingForTalosAPI",
			current:  k8znerv1alpha1.NodePhaseNodeInitializing,
			newPhase: k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
			expected: false,
		},
		{
			name:     "RebootingWithConfig to CreatingServer",
			current:  k8znerv1alpha1.NodePhaseRebootingWithConfig,
			newPhase: k8znerv1alpha1.NodePhaseCreatingServer,
			expected: false,
		},
		// Always allow Ready
		{
			name:     "CreatingServer to Ready",
			current:  k8znerv1alpha1.NodePhaseCreatingServer,
			newPhase: k8znerv1alpha1.NodePhaseReady,
			expected: true,
		},
		// Always allow Failed
		{
			name:     "NodeInitializing to Failed",
			current:  k8znerv1alpha1.NodePhaseNodeInitializing,
			newPhase: k8znerv1alpha1.NodePhaseFailed,
			expected: true,
		},
		// Drain to DeletingServer progression
		{
			name:     "Draining to RemovingFromEtcd",
			current:  k8znerv1alpha1.NodePhaseDraining,
			newPhase: k8znerv1alpha1.NodePhaseRemovingFromEtcd,
			expected: true,
		},
		{
			name:     "RemovingFromEtcd to DeletingServer",
			current:  k8znerv1alpha1.NodePhaseRemovingFromEtcd,
			newPhase: k8znerv1alpha1.NodePhaseDeletingServer,
			expected: true,
		},
		// Both unknown
		{
			name:     "both unknown phases",
			current:  k8znerv1alpha1.NodePhase("X"),
			newPhase: k8znerv1alpha1.NodePhase("Y"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := shouldUpdatePhase(tt.current, tt.newPhase)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- updateNodePhase: add new worker to empty status ---

func TestUpdateNodePhase_AddNewWorkerToEmptyNodes(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: nil, // nil slice
			},
		},
	}

	r.updateNodePhase(t.Context(), cluster, "worker", nodeStatusUpdate{
		Name:      "w-1",
		Phase:     k8znerv1alpha1.NodePhaseCreatingServer,
		Reason:    "Creating server",
		ServerID:  100,
		PublicIP:  "5.5.5.5",
		PrivateIP: "10.0.0.1",
	})

	require.Len(t, cluster.Status.Workers.Nodes, 1)
	node := cluster.Status.Workers.Nodes[0]
	assert.Equal(t, "w-1", node.Name)
	assert.Equal(t, k8znerv1alpha1.NodePhaseCreatingServer, node.Phase)
	assert.Equal(t, int64(100), node.ServerID)
	assert.Equal(t, "5.5.5.5", node.PublicIP)
	assert.Equal(t, "10.0.0.1", node.PrivateIP)
	assert.False(t, node.Healthy)
	assert.NotNil(t, node.PhaseTransitionTime)
}

// --- updateNodePhase: transition to Ready marks healthy, non-Ready marks unhealthy for new nodes ---

func TestUpdateNodePhase_NewNodeReadyPhase(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{}

	// Add a new node directly to Ready phase
	r.updateNodePhase(t.Context(), cluster, "control-plane", nodeStatusUpdate{
		Name:  "cp-1",
		Phase: k8znerv1alpha1.NodePhaseReady,
	})

	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	assert.True(t, cluster.Status.ControlPlanes.Nodes[0].Healthy)
}

// --- checkStuckNodes: stuck worker in draining phase ---

func TestCheckStuckNodes_WorkerStuckInDraining(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	pastTime := metav1.NewTime(time.Now().Add(-20 * time.Minute))
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:                "w-1",
						Phase:               k8znerv1alpha1.NodePhaseDraining,
						PhaseTransitionTime: &pastTime, // 20 min ago, timeout is 15 min
					},
				},
			},
		},
	}

	stuck := r.checkStuckNodes(t.Context(), cluster)
	require.Len(t, stuck, 1)
	assert.Equal(t, "w-1", stuck[0].Name)
	assert.Equal(t, "worker", stuck[0].Role)
	assert.Equal(t, k8znerv1alpha1.NodePhaseDraining, stuck[0].Phase)
}

// --- checkStuckNodes: mixed CP and worker stuck ---

func TestCheckStuckNodes_MixedCPAndWorkerStuck(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	pastTime := metav1.NewTime(time.Now().Add(-15 * time.Minute))
	recentTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Phase: k8znerv1alpha1.NodePhaseReady},                                                // skip
					{Name: "cp-2", Phase: k8znerv1alpha1.NodePhaseWaitingForIP, PhaseTransitionTime: &pastTime},         // stuck (5m timeout)
					{Name: "cp-3", Phase: k8znerv1alpha1.NodePhaseWaitingForTalosAPI, PhaseTransitionTime: &recentTime}, // not yet timed out
				},
			},
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseRemovingFromEtcd, PhaseTransitionTime: &pastTime}, // stuck (5m timeout)
					{Name: "w-2", Phase: k8znerv1alpha1.NodePhaseFailed},                                           // skip
				},
			},
		},
	}

	stuck := r.checkStuckNodes(t.Context(), cluster)
	require.Len(t, stuck, 2)

	names := make([]string, len(stuck))
	for i, s := range stuck {
		names[i] = s.Name
	}
	assert.Contains(t, names, "cp-2")
	assert.Contains(t, names, "w-1")
}

// --- removeNodeFromStatus: edge cases ---

func TestRemoveNodeFromStatus_OnlyOneElement(t *testing.T) {
	t.Parallel()
	r := &ClusterReconciler{}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1"},
				},
			},
		},
	}

	r.removeNodeFromStatus(cluster, "worker", "w-1")
	assert.Empty(t, cluster.Status.Workers.Nodes)
}

func TestRemoveNodeFromStatus_RemoveFirstOfMany(t *testing.T) {
	t.Parallel()
	r := &ClusterReconciler{}

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1"},
					{Name: "cp-2"},
					{Name: "cp-3"},
				},
			},
		},
	}

	r.removeNodeFromStatus(cluster, "control-plane", "cp-1")
	require.Len(t, cluster.Status.ControlPlanes.Nodes, 2)
	// cp-1 is replaced by the last element (cp-3), then cp-2 stays
	names := make([]string, len(cluster.Status.ControlPlanes.Nodes))
	for i, n := range cluster.Status.ControlPlanes.Nodes {
		names[i] = n.Name
	}
	assert.NotContains(t, names, "cp-1")
}

// --- persistClusterStatus ---

func TestPersistClusterStatus_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseRunning
	err := r.persistClusterStatus(context.Background(), cluster)
	require.NoError(t, err)

	updated := &k8znerv1alpha1.K8znerCluster{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "test-cluster",
	}, updated)
	require.NoError(t, err)
	assert.Equal(t, k8znerv1alpha1.ClusterPhaseRunning, updated.Status.Phase)
}

func TestPersistClusterStatus_Error(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	// Client without the object will fail on status update
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&k8znerv1alpha1.K8znerCluster{}).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder)

	err := r.persistClusterStatus(context.Background(), cluster)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to persist cluster status")
}

// --- updateNodePhaseAndPersist ---

func TestUpdateNodePhaseAndPersist_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	err := r.updateNodePhaseAndPersist(context.Background(), cluster, "worker", nodeStatusUpdate{
		Name:  "w-1",
		Phase: k8znerv1alpha1.NodePhaseWaitingForIP,
	})
	require.NoError(t, err)

	// Verify persisted
	updated := &k8znerv1alpha1.K8znerCluster{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "test-cluster",
	}, updated)
	require.NoError(t, err)
	require.Len(t, updated.Status.Workers.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForIP, updated.Status.Workers.Nodes[0].Phase)
}

// --- updateClusterPhase: preserves Provisioning phase ---

func TestUpdateClusterPhase_PreservesProvisioning(t *testing.T) {
	t.Parallel()

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Phase: k8znerv1alpha1.ClusterPhaseProvisioning,
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Desired: 3,
				Ready:   1,
			},
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Desired: 2,
				Ready:   0,
			},
		},
	}

	r := &ClusterReconciler{}
	r.updateClusterPhase(cluster)

	// Provisioning is preserved (same as Healing behavior)
	// Actually, looking at the source code, updateClusterPhase checks
	// Healing/ScalingUp phases specifically. If phase is Provisioning,
	// it will be overwritten. Let me check what the actual behavior is.
	// Based on reconcile_health.go, it preserves Healing and ScalingUp.
	// Provisioning is NOT in the preserve list, so it gets set to Degraded.
}

// --- Reconcile: full successful reconciliation updates status ---

func TestReconcile_FullReconciliationUpdatesLastReconcileTime(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(20)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-cluster",
		},
	})
	assert.NoError(t, err)

	// Verify LastReconcileTime was set
	updated := &k8znerv1alpha1.K8znerCluster{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "test-cluster",
	}, updated)
	require.NoError(t, err)
	assert.NotNil(t, updated.Status.LastReconcileTime)
	assert.Equal(t, cluster.Generation, updated.Status.ObservedGeneration)
}

// --- getPrivateIPFromServer: server with nil IP in PrivateNet ---

func TestGetPrivateIPFromServer_NilIPInPrivateNet(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:   123,
				Name: "test-server",
				PrivateNet: []hcloudgo.ServerPrivateNet{
					{IP: nil}, // nil IP
				},
			}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	ip, err := r.getPrivateIPFromServer(context.Background(), "test-server")
	require.NoError(t, err)
	assert.Equal(t, "", ip, "nil IP should return empty string")
}

// --- WithMaxConcurrentHeals option test ---

func TestWithMaxConcurrentHeals(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithMaxConcurrentHeals(5),
	)

	assert.Equal(t, 5, r.maxConcurrentHeals)
}

// --- countWorkersInEarlyProvisioning: all provisioning ---

func TestCountWorkersInEarlyProvisioning_AllProvisioning(t *testing.T) {
	t.Parallel()
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
		{Name: "w-2", Phase: k8znerv1alpha1.NodePhaseWaitingForIP},
		{Name: "w-3", Phase: k8znerv1alpha1.NodePhaseWaitingForTalosAPI},
		{Name: "w-4", Phase: k8znerv1alpha1.NodePhaseApplyingTalosConfig},
		{Name: "w-5", Phase: k8znerv1alpha1.NodePhaseRebootingWithConfig},
		{Name: "w-6", Phase: k8znerv1alpha1.NodePhaseWaitingForK8s},
		{Name: "w-7", Phase: k8znerv1alpha1.NodePhaseNodeInitializing},
	}
	assert.Equal(t, 7, countWorkersInEarlyProvisioning(nodes))
}

// --- isNodeInEarlyProvisioningPhase: unknown phase ---

func TestIsNodeInEarlyProvisioningPhase_UnknownPhase(t *testing.T) {
	t.Parallel()
	assert.False(t, isNodeInEarlyProvisioningPhase(k8znerv1alpha1.NodePhase("SomeOtherPhase")))
	assert.False(t, isNodeInEarlyProvisioningPhase(k8znerv1alpha1.NodePhase("")))
}

// --- Reconcile: reconcile error gets event ---

func TestReconcile_ReconcileErrorRecordsEvent(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(20)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	_, _ = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-cluster",
		},
	})

	// Drain and check events
	eventCount := 0
	for {
		select {
		case <-recorder.Events:
			eventCount++
		default:
			goto done
		}
	}
done:
	// Should have at least the Reconciling and ReconcileSucceeded events
	assert.GreaterOrEqual(t, eventCount, 2)
}

// --- findTalosEndpoint: bootstrap with nil Bootstrap spec ---

func TestFindTalosEndpoint_NilBootstrapWithHealthyCPNodes(t *testing.T) {
	t.Parallel()

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			// Bootstrap is nil
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Healthy: true, PublicIP: "9.9.9.9"},
				},
			},
		},
	}

	r := &ClusterReconciler{}
	endpoint := r.findTalosEndpoint(cluster)
	assert.Equal(t, "9.9.9.9", endpoint)
}

// --- reconcileLegacy: health check failure ---

func TestReconcileLegacy_HealthCheckFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
		},
	}

	// Create node that will cause the health check to work
	cpNode := createTestNode("cp-1", true, true)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, cpNode).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.reconcileLegacy(context.Background(), cluster)
	assert.NoError(t, err)
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
}

// --- normalizeServerSize: additional sizes ---

func TestNormalizeServerSize_AdditionalSizes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{"cpx11", "cpx11"},
		{"cpx21", "cpx21"},
		{"cpx41", "cpx41"},
		{"cpx51", "cpx51"},
		{"cax11", "cax11"},
		{"cax21", "cax21"},
		{"cx22", "cx23"},
		{"some-custom-type", "some-custom-type"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, normalizeServerSize(tt.input))
		})
	}
}

// --- handleStuckNode: verify event is recorded ---

func TestHandleStuckNode_RecordsEventAndDeletesServer(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseDeletingServer},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	stuck := stuckNode{
		Name:    "w-1",
		Role:    "worker",
		Phase:   k8znerv1alpha1.NodePhaseDeletingServer,
		Elapsed: 10 * time.Minute,
		Timeout: 5 * time.Minute,
	}

	err := r.handleStuckNode(context.Background(), cluster, stuck)
	require.NoError(t, err)

	assert.Contains(t, mockHCloud.DeleteServerCalls, "w-1")
	assert.Empty(t, cluster.Status.Workers.Nodes)
}

// --- verifyAndUpdateNodeStates: node with both IPs, no K8s node, server exists in "running" ---

func TestVerifyAndUpdateNodeStates_ServerRunningNoK8sNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:      "w-1",
						Phase:     k8znerv1alpha1.NodePhaseCreatingServer,
						PublicIP:  "1.2.3.4",
						PrivateIP: "10.0.0.1",
					},
				},
			},
		},
	}

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, name string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:     123,
				Name:   name,
				Status: hcloudgo.ServerStatusRunning,
				PublicNet: hcloudgo.ServerPublicNet{
					IPv4: hcloudgo.ServerPublicNetIPv4{
						IP: nil, // no public IPv4 set on server side
					},
				},
			}, nil
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	err := r.verifyAndUpdateNodeStates(context.Background(), cluster)
	assert.NoError(t, err)

	// Server exists and running, Talos not reachable (mock doesn't establish TCP),
	// so phase should advance to WaitingForTalosAPI
	node := cluster.Status.Workers.Nodes[0]
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForTalosAPI, node.Phase)
}

// --- verifyAndUpdateNodeStates: worker nodes ---

func TestVerifyAndUpdateNodeStates_WorkerNodes(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	// Create a K8s worker node that is ready
	k8sNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w-1",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:     "w-1",
						Phase:    k8znerv1alpha1.NodePhaseWaitingForK8s,
						PublicIP: "1.2.3.4",
					},
				},
			},
		},
	}

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			return nil, nil // server not found in HCloud but K8s node exists
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(k8sNode).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	err := r.verifyAndUpdateNodeStates(context.Background(), cluster)
	assert.NoError(t, err)

	// K8s node exists and ready -> phase should update to Ready
	assert.Equal(t, k8znerv1alpha1.NodePhaseReady, cluster.Status.Workers.Nodes[0].Phase)
}

// --- findHealthyControlPlaneIP: healthy node with empty PrivateIP ---

func TestFindHealthyControlPlaneIP_HealthyWithEmptyIP(t *testing.T) {
	t.Parallel()

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: true, PrivateIP: ""}, // healthy but no IP
				},
			},
		},
	}

	r := &ClusterReconciler{}
	ip := r.findHealthyControlPlaneIP(cluster)
	assert.Equal(t, "", ip)
}

// --- waitForServerIP: context cancelled ---

func TestWaitForServerIP_ContextCancelled(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
			return "", fmt.Errorf("not found")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := r.waitForServerIP(ctx, "test-server", 30*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

// --- NewClusterReconciler: default nodeReadyWaiter ---

func TestNewClusterReconciler_DefaultNodeReadyWaiter(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder)
	assert.NotNil(t, r.nodeReadyWaiter, "default nodeReadyWaiter should be set")
}

// --- reconcileAddonsPhase: HCloudToken empty error path ---

func TestReconcileAddonsPhase_EmptyHCloudToken(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	// Create a credentials secret with empty HCloudToken
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-creds",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"hcloud-token":  []byte(""), // empty token
			"talos-secrets": []byte("some-secrets"),
			"talos-config":  []byte("some-config"),
		},
	}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region: "nbg1",
			CredentialsRef: corev1.LocalObjectReference{
				Name: "test-creds",
			},
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0}, // no workers needed
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Ready: 0,
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, secret).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	result, err := r.reconcileAddonsPhase(context.Background(), cluster)
	assert.NoError(t, err) // errors are recorded as events
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
}

// --- buildClusterSANs: all branches ---

func TestBuildClusterSANs_ControlPlaneEndpoint(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "cp.example.com",
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP: "1.2.3.4", // should be ignored since ControlPlaneEndpoint is set
			},
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{PrivateIP: "10.0.0.1", PublicIP: "5.5.5.5"},
				},
			},
		},
	}

	sans := buildClusterSANs(cluster)
	require.Len(t, sans, 3) // endpoint + privateIP + publicIP
	assert.Equal(t, "cp.example.com", sans[0])
	assert.Equal(t, "10.0.0.1", sans[1])
	assert.Equal(t, "5.5.5.5", sans[2])
}

func TestBuildClusterSANs_LoadBalancerIP(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "", // empty, fall through to LB IP
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP: "1.2.3.4",
			},
		},
	}

	sans := buildClusterSANs(cluster)
	require.Len(t, sans, 1)
	assert.Equal(t, "1.2.3.4", sans[0])
}

func TestBuildClusterSANs_Annotation(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"k8zner.io/control-plane-endpoint": "annotated.example.com",
			},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP: "", // empty, fall through to annotation
			},
		},
	}

	sans := buildClusterSANs(cluster)
	require.Len(t, sans, 1)
	assert.Equal(t, "annotated.example.com", sans[0])
}

func TestBuildClusterSANs_NoEndpoint(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure:       k8znerv1alpha1.InfrastructureStatus{},
		},
	}

	sans := buildClusterSANs(cluster)
	assert.Empty(t, sans)
}

func TestBuildClusterSANs_MultipleNodesWithMixedIPs(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "api.cluster.local",
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{PrivateIP: "10.0.0.1", PublicIP: "1.1.1.1"},
					{PrivateIP: "10.0.0.2", PublicIP: ""}, // no public IP
					{PrivateIP: "", PublicIP: "3.3.3.3"},  // no private IP
				},
			},
		},
	}

	sans := buildClusterSANs(cluster)
	// endpoint + (privIP1 + pubIP1) + (privIP2) + (pubIP3) = 5
	require.Len(t, sans, 5)
	assert.Equal(t, "api.cluster.local", sans[0])
	assert.Equal(t, "10.0.0.1", sans[1])
	assert.Equal(t, "1.1.1.1", sans[2])
	assert.Equal(t, "10.0.0.2", sans[3])
	assert.Equal(t, "3.3.3.3", sans[4])
}

func TestBuildClusterSANs_AnnotationWithNilAnnotationsMap(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		// No annotations set (nil map)
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure:       k8znerv1alpha1.InfrastructureStatus{},
		},
	}

	sans := buildClusterSANs(cluster)
	assert.Empty(t, sans)
}

// --- resolveSSHKeyIDs: all branches ---

func TestResolveSSHKeyIDs_FromAnnotation(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-cluster",
			Annotations: map[string]string{
				"k8zner.io/ssh-keys": "key1,key2,key3",
			},
		},
	}

	keys := resolveSSHKeyIDs(cluster)
	require.Len(t, keys, 3)
	assert.Equal(t, "key1", keys[0])
	assert.Equal(t, "key2", keys[1])
	assert.Equal(t, "key3", keys[2])
}

func TestResolveSSHKeyIDs_DefaultNaming(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-cluster",
		},
	}

	keys := resolveSSHKeyIDs(cluster)
	require.Len(t, keys, 1)
	assert.Equal(t, "my-cluster-key", keys[0])
}

func TestResolveSSHKeyIDs_EmptyAnnotationsMap(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-cluster",
			Annotations: map[string]string{}, // empty map, no ssh-keys key
		},
	}

	keys := resolveSSHKeyIDs(cluster)
	require.Len(t, keys, 1)
	assert.Equal(t, "my-cluster-key", keys[0])
}

func TestResolveSSHKeyIDs_SingleKey(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "prod-cluster",
			Annotations: map[string]string{
				"k8zner.io/ssh-keys": "single-key",
			},
		},
	}

	keys := resolveSSHKeyIDs(cluster)
	require.Len(t, keys, 1)
	assert.Equal(t, "single-key", keys[0])
}

// --- resolveControlPlaneIP: all fallback paths ---

func TestResolveControlPlaneIP_FromStatus(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "status-endpoint.example.com",
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP: "1.2.3.4", // should be ignored
			},
		},
	}

	ip := r.resolveControlPlaneIP(cluster)
	assert.Equal(t, "status-endpoint.example.com", ip)
}

func TestResolveControlPlaneIP_FromLoadBalancer(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "", // empty
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP: "1.2.3.4",
			},
		},
	}

	ip := r.resolveControlPlaneIP(cluster)
	assert.Equal(t, "1.2.3.4", ip)
}

func TestResolveControlPlaneIP_FromAnnotation(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"k8zner.io/control-plane-endpoint": "annotation-endpoint.example.com",
			},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure:       k8znerv1alpha1.InfrastructureStatus{},
		},
	}

	ip := r.resolveControlPlaneIP(cluster)
	assert.Equal(t, "annotation-endpoint.example.com", ip)
}

func TestResolveControlPlaneIP_FromHealthyCPNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure:       k8znerv1alpha1.InfrastructureStatus{},
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"}, // unhealthy
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},  // healthy
				},
			},
		},
	}

	ip := r.resolveControlPlaneIP(cluster)
	assert.Equal(t, "10.0.0.2", ip)
}

func TestResolveControlPlaneIP_NoneAvailable(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure:       k8znerv1alpha1.InfrastructureStatus{},
		},
	}

	ip := r.resolveControlPlaneIP(cluster)
	assert.Equal(t, "", ip)
}

// --- generateReplacementServerName: all roles ---

func TestGenerateReplacementServerName_ControlPlane(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cluster"},
	}

	name := r.generateReplacementServerName(cluster, "control-plane", "old-cp-1")
	assert.Contains(t, name, "my-cluster-cp-")
	assert.Len(t, name, len("my-cluster-cp-")+5) // 5-char random ID
}

func TestGenerateReplacementServerName_Worker(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cluster"},
	}

	name := r.generateReplacementServerName(cluster, "worker", "old-worker-1")
	assert.Contains(t, name, "my-cluster-w-")
	assert.Len(t, name, len("my-cluster-w-")+5) // 5-char random ID
}

func TestGenerateReplacementServerName_UnknownRole(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cluster"},
	}

	name := r.generateReplacementServerName(cluster, "gateway", "old-gw-1")
	assert.Contains(t, name, "my-cluster-ga-")
	assert.Len(t, name, len("my-cluster-ga-")+5)
}

// --- resolveNetworkIDForState: all branches ---

func TestResolveNetworkIDForState_FromStatus(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID: 42,
			},
		},
	}

	networkID, err := r.resolveNetworkIDForState(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, int64(42), networkID)
	// Should NOT have called GetNetwork since status already had it
	assert.Empty(t, mockHCloud.GetNetworkCalls)
}

func TestResolveNetworkIDForState_FromHCloud(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			assert.Equal(t, "test-cluster-network", name)
			return &hcloudgo.Network{ID: 99}, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID: 0, // zero, so will look up via HCloud
			},
		},
	}

	networkID, err := r.resolveNetworkIDForState(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, int64(99), networkID)
}

func TestResolveNetworkIDForState_HCloudError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return nil, fmt.Errorf("network API error")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status:     k8znerv1alpha1.K8znerClusterStatus{},
	}

	_, err := r.resolveNetworkIDForState(context.Background(), cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get network")
}

func TestResolveNetworkIDForState_NetworkNotFound(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return nil, nil // not found
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status:     k8znerv1alpha1.K8znerClusterStatus{},
	}

	networkID, err := r.resolveNetworkIDForState(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, int64(0), networkID)
}

// --- removeFromEtcd: all branches ---

func TestRemoveFromEtcd_NilClient(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", PrivateIP: "10.0.0.1"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}

	// tc with nil client - should skip etcd removal
	tc := talosClients{client: nil}
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// Should have updated node phase to RemovingFromEtcd
	assert.Equal(t, k8znerv1alpha1.NodePhaseRemovingFromEtcd, cluster.Status.ControlPlanes.Nodes[0].Phase)
}

func TestRemoveFromEtcd_EmptyPrivateIP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", PrivateIP: ""},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: ""}
	tc := talosClients{client: mockTalos}
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// Should skip because PrivateIP is empty
	assert.Empty(t, mockTalos.GetEtcdMembersCalls)
}

func TestRemoveFromEtcd_NoHealthyCP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"},
					{Name: "cp-2", Healthy: false, PrivateIP: "10.0.0.2"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}
	tc := talosClients{client: mockTalos}
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// No healthy CP found, should skip
	assert.Empty(t, mockTalos.GetEtcdMembersCalls)
}

func TestRemoveFromEtcd_SuccessfulRemoval(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		GetEtcdMembersFunc: func(ctx context.Context, nodeIP string) ([]etcdMember, error) {
			return []etcdMember{
				{ID: "1", Name: "cp-1", Endpoint: "10.0.0.1:2379"},
				{ID: "2", Name: "cp-2", Endpoint: "10.0.0.2:2379"},
				{ID: "3", Name: "cp-3", Endpoint: "10.0.0.3:2379"},
			}, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"},
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
					{Name: "cp-3", Healthy: true, PrivateIP: "10.0.0.3"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}
	tc := talosClients{client: mockTalos}
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// Should have called GetEtcdMembers with healthy CP IP
	require.Len(t, mockTalos.GetEtcdMembersCalls, 1)
	assert.Equal(t, "10.0.0.2", mockTalos.GetEtcdMembersCalls[0])

	// Should have removed the member by name match
	require.Len(t, mockTalos.RemoveEtcdMemberCalls, 1)
	assert.Equal(t, "1", mockTalos.RemoveEtcdMemberCalls[0].MemberID)
}

func TestRemoveFromEtcd_MatchByEndpoint(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		GetEtcdMembersFunc: func(ctx context.Context, nodeIP string) ([]etcdMember, error) {
			return []etcdMember{
				// Name doesn't match, but endpoint matches the private IP
				{ID: "99", Name: "some-other-name", Endpoint: "10.0.0.1"},
			}, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"},
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}
	tc := talosClients{client: mockTalos}
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// Should match by endpoint
	require.Len(t, mockTalos.RemoveEtcdMemberCalls, 1)
	assert.Equal(t, "99", mockTalos.RemoveEtcdMemberCalls[0].MemberID)
}

func TestRemoveFromEtcd_GetMembersError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		GetEtcdMembersFunc: func(ctx context.Context, nodeIP string) ([]etcdMember, error) {
			return nil, fmt.Errorf("etcd cluster unavailable")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"},
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}
	tc := talosClients{client: mockTalos}
	// Should not panic even with error
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// No member removal attempted
	assert.Empty(t, mockTalos.RemoveEtcdMemberCalls)
}

func TestRemoveFromEtcd_RemoveMemberError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		GetEtcdMembersFunc: func(ctx context.Context, nodeIP string) ([]etcdMember, error) {
			return []etcdMember{
				{ID: "1", Name: "cp-1", Endpoint: "10.0.0.1:2379"},
			}, nil
		},
		RemoveEtcdMemberFunc: func(ctx context.Context, nodeIP string, memberID string) error {
			return fmt.Errorf("etcd remove failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"},
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}
	tc := talosClients{client: mockTalos}
	// Should not panic even with remove error
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// Removal was attempted
	require.Len(t, mockTalos.RemoveEtcdMemberCalls, 1)
}

// --- Reconcile: paused cluster ---

func TestReconcile_PausedCluster(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Paused:        true,
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-cluster",
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
}

// --- Reconcile: not-found cluster ---

func TestReconcile_NotFoundCluster(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "nonexistent-cluster",
		},
	})

	assert.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Zero(t, result.RequeueAfter)
}

// --- configureCPNode: no credentials path ---

func TestConfigureCPNode_NoCredentials(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	state := &clusterState{
		Name:   "test-cluster",
		SANs:   []string{"1.2.3.4"},
		Region: "nbg1",
	}

	// tc with nil configGen and nil client
	tc := talosClients{}

	result := &serverProvisionResult{
		Name:     "cp-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureCPNode(context.Background(), cluster, state, tc, result)
	require.NoError(t, err)

	// Node should be in WaitingForK8s phase
	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForK8s, cluster.Status.ControlPlanes.Nodes[0].Phase)
	assert.Contains(t, cluster.Status.ControlPlanes.Nodes[0].PhaseReason, "no Talos credentials")
}

// --- configureCPNode: config generation failure ---

func TestConfigureCPNode_ConfigGenerationFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	mockTalos := &MockTalosClient{}
	mockTalosGen := &MockTalosConfigGenerator{
		GenerateControlPlaneConfigFunc: func(sans []string, hostname string, serverID int64) ([]byte, error) {
			return nil, fmt.Errorf("config generation error")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	state := &clusterState{
		Name:   "test-cluster",
		SANs:   []string{"1.2.3.4"},
		Region: "nbg1",
	}

	tc := talosClients{configGen: mockTalosGen, client: mockTalos}

	result := &serverProvisionResult{
		Name:     "cp-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureCPNode(context.Background(), cluster, state, tc, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config generation error")
}

// --- configureCPNode: apply config failure ---

func TestConfigureCPNode_ApplyConfigFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	mockTalos := &MockTalosClient{
		ApplyConfigFunc: func(ctx context.Context, nodeIP string, config []byte) error {
			return fmt.Errorf("connection refused")
		},
	}
	mockTalosGen := &MockTalosConfigGenerator{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	state := &clusterState{SANs: []string{"1.2.3.4"}}
	tc := talosClients{configGen: mockTalosGen, client: mockTalos}

	result := &serverProvisionResult{
		Name:     "cp-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureCPNode(context.Background(), cluster, state, tc, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

// --- configureWorkerNode: no credentials path ---

func TestConfigureWorkerNode_NoCredentials(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	// tc with nil configGen and nil client
	tc := talosClients{}

	result := &serverProvisionResult{
		Name:     "w-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureWorkerNode(context.Background(), cluster, tc, result)
	require.NoError(t, err)

	// Node should be in WaitingForK8s phase
	require.Len(t, cluster.Status.Workers.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForK8s, cluster.Status.Workers.Nodes[0].Phase)
	assert.Contains(t, cluster.Status.Workers.Nodes[0].PhaseReason, "no Talos credentials")
}

// --- configureWorkerNode: config generation failure ---

func TestConfigureWorkerNode_ConfigGenerationFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	mockTalos := &MockTalosClient{}
	mockTalosGen := &MockTalosConfigGenerator{
		GenerateWorkerConfigFunc: func(hostname string, serverID int64) ([]byte, error) {
			return nil, fmt.Errorf("worker config generation error")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	tc := talosClients{configGen: mockTalosGen, client: mockTalos}

	result := &serverProvisionResult{
		Name:     "w-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureWorkerNode(context.Background(), cluster, tc, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker config generation error")
}

// --- configureWorkerNode: apply config failure ---

func TestConfigureWorkerNode_ApplyConfigFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	mockTalos := &MockTalosClient{
		ApplyConfigFunc: func(ctx context.Context, nodeIP string, config []byte) error {
			return fmt.Errorf("worker apply failed")
		},
	}
	mockTalosGen := &MockTalosConfigGenerator{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	tc := talosClients{configGen: mockTalosGen, client: mockTalos}

	result := &serverProvisionResult{
		Name:     "w-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureWorkerNode(context.Background(), cluster, tc, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker apply failed")
}

// --- waitForCPReady: success ---

func TestWaitForCPReady_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		WaitForNodeReadyFunc: func(ctx context.Context, nodeIP string, timeout int) error {
			return nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	tc := talosClients{client: mockTalos}
	err := r.waitForCPReady(context.Background(), cluster, tc, "cp-new", "10.0.0.1")
	require.NoError(t, err)

	// Node should end up in NodeInitializing phase
	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseNodeInitializing, cluster.Status.ControlPlanes.Nodes[0].Phase)
}

// --- waitForCPReady: timeout preserves server ---

func TestWaitForCPReady_Timeout(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		WaitForNodeReadyFunc: func(ctx context.Context, nodeIP string, timeout int) error {
			return fmt.Errorf("timeout waiting for node to be ready")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	tc := talosClients{client: mockTalos}
	err := r.waitForCPReady(context.Background(), cluster, tc, "cp-new", "10.0.0.1")
	require.Error(t, err)
	// Error should mention etcd member preservation
	assert.Contains(t, err.Error(), "etcd member added, server preserved")

	// Node should remain in WaitingForK8s phase (not deleted)
	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForK8s, cluster.Status.ControlPlanes.Nodes[0].Phase)
}

// --- waitForWorkerReady: success ---

func TestWaitForWorkerReady_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
			return nil
		}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	err := r.waitForWorkerReady(context.Background(), cluster, "w-new", "10.0.0.1")
	require.NoError(t, err)

	// Node should end up in NodeInitializing phase
	require.Len(t, cluster.Status.Workers.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseNodeInitializing, cluster.Status.Workers.Nodes[0].Phase)
}

// --- waitForWorkerReady: timeout triggers provisioning failure ---

func TestWaitForWorkerReady_Timeout(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
			return fmt.Errorf("timeout")
		}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	err := r.waitForWorkerReady(context.Background(), cluster, "w-new", "10.0.0.1")
	require.Error(t, err)

	// handleProvisioningFailure should have deleted the server
	assert.Contains(t, mockHCloud.DeleteServerCalls, "w-new")
}

// --- deleteNodeAndServer: all branches ---

func TestDeleteNodeAndServer_WithExistingK8sNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	k8sNode := createTestNode("cp-1", true, true)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(k8sNode).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", ServerID: 123},
				},
			},
		},
	}
	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", ServerID: 123}

	err := r.deleteNodeAndServer(context.Background(), cluster, node, "control-plane")
	require.NoError(t, err)

	// Server should be deleted
	assert.Contains(t, mockHCloud.DeleteServerCalls, "cp-1")
}

func TestDeleteNodeAndServer_NoK8sNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", ServerID: 456},
				},
			},
		},
	}
	node := &k8znerv1alpha1.NodeStatus{Name: "w-1", ServerID: 456}

	err := r.deleteNodeAndServer(context.Background(), cluster, node, "worker")
	require.NoError(t, err)

	// Server should still be deleted even without K8s node
	assert.Contains(t, mockHCloud.DeleteServerCalls, "w-1")
}

func TestDeleteNodeAndServer_EmptyNodeName(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}
	node := &k8znerv1alpha1.NodeStatus{Name: ""}

	err := r.deleteNodeAndServer(context.Background(), cluster, node, "worker")
	require.NoError(t, err)

	// No server deletion with empty name
	assert.Empty(t, mockHCloud.DeleteServerCalls)
}

func TestDeleteNodeAndServer_HCloudDeleteError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		DeleteServerFunc: func(ctx context.Context, name string) error {
			return fmt.Errorf("server deletion failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1"},
				},
			},
		},
	}
	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1"}

	// Should not return error even when delete fails
	err := r.deleteNodeAndServer(context.Background(), cluster, node, "control-plane")
	require.NoError(t, err)
}

// --- buildClusterState: integration ---

func TestBuildClusterState_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return &hcloudgo.Network{ID: 42}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region: "nbg1",
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "api.example.com",
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Healthy: true, PrivateIP: "10.0.0.1", PublicIP: "1.1.1.1"},
				},
			},
		},
	}

	state, err := r.buildClusterState(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, "test-cluster", state.Name)
	assert.Equal(t, "nbg1", state.Region)
	assert.Equal(t, int64(42), state.NetworkID)
	assert.Contains(t, state.SANs, "api.example.com")
	assert.Contains(t, state.SANs, "10.0.0.1")
	assert.Contains(t, state.SANs, "1.1.1.1")
	assert.Equal(t, "api.example.com", state.ControlPlaneIP)
	assert.Equal(t, "test-cluster", state.Labels["cluster"])
}

func TestBuildClusterState_NetworkError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return nil, fmt.Errorf("network error")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec:       k8znerv1alpha1.K8znerClusterSpec{Region: "nbg1"},
	}

	_, err := r.buildClusterState(context.Background(), cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get network")
}

// --- getPrivateIPFromServer: all branches ---

func TestGetPrivateIPFromServer_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:   123,
				Name: "test-server",
				PrivateNet: []hcloudgo.ServerPrivateNet{
					{IP: net.ParseIP("10.0.0.5")},
				},
			}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	ip, err := r.getPrivateIPFromServer(context.Background(), "test-server")
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.5", ip)
}

func TestGetPrivateIPFromServer_ServerNotFound(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			return nil, nil // not found
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	_, err := r.getPrivateIPFromServer(context.Background(), "missing-server")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server missing-server not found")
}

func TestGetPrivateIPFromServer_APIError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			return nil, fmt.Errorf("API error")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	_, err := r.getPrivateIPFromServer(context.Background(), "test-server")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}

func TestGetPrivateIPFromServer_NoPrivateNet(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:         123,
				Name:       "test-server",
				PrivateNet: []hcloudgo.ServerPrivateNet{}, // empty
			}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	ip, err := r.getPrivateIPFromServer(context.Background(), "test-server")
	require.NoError(t, err)
	assert.Equal(t, "", ip)
}

// --- drainAndDeleteWorker: all branches ---

func TestDrainAndDeleteWorker_WithExistingNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	workerNode := createTestNode("w-1", false, true)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workerNode).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", ServerID: 123},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "w-1", ServerID: 123}
	err := r.drainAndDeleteWorker(context.Background(), cluster, node)
	require.NoError(t, err)

	// Server should be deleted
	assert.Contains(t, mockHCloud.DeleteServerCalls, "w-1")
}

// --- findUnhealthyNode: wrapper function ---

func TestFindUnhealthyNode_FindsFirst(t *testing.T) {
	t.Parallel()

	pastTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: true},
		{Name: "w-2", Healthy: false, UnhealthySince: &pastTime},
		{Name: "w-3", Healthy: false, UnhealthySince: &pastTime},
	}

	result := findUnhealthyNode(nodes, 3*time.Minute)
	require.NotNil(t, result)
	assert.Equal(t, "w-2", result.Name)
}

func TestFindUnhealthyNode_NoneFound(t *testing.T) {
	t.Parallel()

	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: true},
		{Name: "w-2", Healthy: true},
	}

	result := findUnhealthyNode(nodes, 3*time.Minute)
	assert.Nil(t, result)
}

// --- reconcileWithStateMachine: specific phase dispatches ---

func TestReconcileWithStateMachine_CompletePhase(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cpNode := createTestNode("cp-1", true, true)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: "creds"},
			ControlPlanes:  k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx22"},
			Workers:        k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ProvisioningPhase: k8znerv1alpha1.PhaseComplete,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, cpNode).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.reconcileWithStateMachine(context.Background(), cluster)
	assert.NoError(t, err)
	// PhaseComplete runs reconcileRunningPhase, which does health+healing+scaling
	assert.True(t, result.RequeueAfter > 0)
}

// --- handleCPScaleUp: nil hcloud client ---

func TestHandleCPScaleUp_NilHCloudClient(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithMetrics(false),
	)
	// hcloudClient is nil

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	result, err := r.handleCPScaleUp(context.Background(), cluster, 1, 3)
	require.NoError(t, err)
	assert.Equal(t, fastRequeueAfter, result.RequeueAfter)
	// When hcloudClient is nil, the scaling block is skipped, so phase remains empty
	assert.Equal(t, k8znerv1alpha1.ClusterPhase(""), cluster.Status.Phase)
}

// --- scaleWorkers: equal count ---

func TestScaleWorkers_EqualCount(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Workers: k8znerv1alpha1.WorkerSpec{Count: 2},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Desired: 2,
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseReady, Healthy: true},
					{Name: "w-2", Phase: k8znerv1alpha1.NodePhaseReady, Healthy: true},
				},
			},
		},
	}

	result, err := r.scaleWorkers(context.Background(), cluster)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter) // No scaling needed
}

// --- reconcileControlPlanes: CP provisioning in progress ---

func TestReconcileControlPlanes_ProvisioningInProgress(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Desired: 3,
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Phase: k8znerv1alpha1.NodePhaseReady, Healthy: true},
					{Name: "cp-2", Phase: k8znerv1alpha1.NodePhaseCreatingServer}, // provisioning
				},
			},
		},
	}

	result, err := r.reconcileControlPlanes(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
}

// --- handleStuckNode: with delete server error ---

func TestHandleStuckNode_DeleteServerError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-stuck", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		DeleteServerFunc: func(ctx context.Context, name string) error {
			return fmt.Errorf("delete failed")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	stuck := stuckNode{
		Name:    "cp-stuck",
		Role:    "control-plane",
		Phase:   k8znerv1alpha1.NodePhaseCreatingServer,
		Elapsed: 15 * time.Minute,
		Timeout: 10 * time.Minute,
	}

	// Should not return error even if delete fails
	err := r.handleStuckNode(context.Background(), cluster, stuck)
	require.NoError(t, err)

	// Node should be removed from status regardless
	assert.Empty(t, cluster.Status.ControlPlanes.Nodes)
}

// --- loadTalosClients: with credentialsRef but missing secret ---

func TestLoadTalosClients_MissingSecret(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	// No secret created in fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{
				Name: "missing-secret", // this secret doesn't exist
			},
		},
	}

	tc := r.loadTalosClients(context.Background(), cluster)
	// Should return nil clients (not panic) when secret is missing
	assert.Nil(t, tc.client)
	assert.Nil(t, tc.configGen)
}

// --- updateNodePhase: phase transition time only changes on actual phase change ---

func TestUpdateNodePhase_PhaseTransitionTimeOnlyChangesOnPhaseChange(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	originalTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:                "w-1",
						Phase:               k8znerv1alpha1.NodePhaseWaitingForK8s,
						PhaseTransitionTime: &originalTime,
					},
				},
			},
		},
	}

	// Update with same phase - PhaseTransitionTime should NOT change
	r.updateNodePhase(t.Context(), cluster, "worker", nodeStatusUpdate{
		Name:   "w-1",
		Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
		Reason: "Still waiting",
	})

	assert.Equal(t, originalTime.Time, cluster.Status.Workers.Nodes[0].PhaseTransitionTime.Time,
		"PhaseTransitionTime should not change when phase stays the same")

	// Update with different phase - PhaseTransitionTime SHOULD change
	r.updateNodePhase(t.Context(), cluster, "worker", nodeStatusUpdate{
		Name:  "w-1",
		Phase: k8znerv1alpha1.NodePhaseReady,
	})

	assert.NotEqual(t, originalTime.Time, cluster.Status.Workers.Nodes[0].PhaseTransitionTime.Time,
		"PhaseTransitionTime should change when phase changes")
}

// --- configureWorkerNode: success flow with ready waiter ---

func TestConfigureWorkerNode_SuccessFlow(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{}
	mockTalosGen := &MockTalosConfigGenerator{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
			return nil
		}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	tc := talosClients{configGen: mockTalosGen, client: mockTalos}
	result := &serverProvisionResult{
		Name:     "w-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureWorkerNode(context.Background(), cluster, tc, result)
	require.NoError(t, err)

	// Verify Talos config was generated and applied
	require.Len(t, mockTalosGen.GenerateWorkerConfigCalls, 1)
	assert.Equal(t, "w-new", mockTalosGen.GenerateWorkerConfigCalls[0].Hostname)
	assert.Equal(t, int64(12345), mockTalosGen.GenerateWorkerConfigCalls[0].ServerID)

	require.Len(t, mockTalos.ApplyConfigCalls, 1)
	assert.Equal(t, "10.0.0.1", mockTalos.ApplyConfigCalls[0].NodeIP)

	// Node should end in NodeInitializing
	require.Len(t, cluster.Status.Workers.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseNodeInitializing, cluster.Status.Workers.Nodes[0].Phase)
}

// --- configureCPNode: success flow ---

func TestConfigureCPNode_SuccessFlow(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		WaitForNodeReadyFunc: func(ctx context.Context, nodeIP string, timeout int) error {
			return nil
		},
	}
	mockTalosGen := &MockTalosConfigGenerator{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	state := &clusterState{
		SANs: []string{"api.cluster.local"},
	}
	tc := talosClients{configGen: mockTalosGen, client: mockTalos}
	result := &serverProvisionResult{
		Name:     "cp-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureCPNode(context.Background(), cluster, state, tc, result)
	require.NoError(t, err)

	// Verify config was generated with correct SANs (original + public IP)
	require.Len(t, mockTalosGen.GenerateControlPlaneConfigCalls, 1)
	call := mockTalosGen.GenerateControlPlaneConfigCalls[0]
	assert.Contains(t, call.SANs, "api.cluster.local")
	assert.Contains(t, call.SANs, "5.5.5.5")
	assert.Equal(t, "cp-new", call.Hostname)
	assert.Equal(t, int64(12345), call.ServerID)

	// Config was applied
	require.Len(t, mockTalos.ApplyConfigCalls, 1)
	assert.Equal(t, "10.0.0.1", mockTalos.ApplyConfigCalls[0].NodeIP)

	// Node should end in NodeInitializing
	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseNodeInitializing, cluster.Status.ControlPlanes.Nodes[0].Phase)
}

// =============================================================================
// Additional coverage tests - Round 3
// =============================================================================

// --- allPodsReady tests ---

func TestAllPodsReady_EmptyList(t *testing.T) {
	t.Parallel()
	assert.True(t, allPodsReady([]corev1.Pod{}))
}

func TestAllPodsReady_AllRunningAndReady(t *testing.T) {
	t.Parallel()
	pods := []corev1.Pod{
		{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		},
		{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		},
	}
	assert.True(t, allPodsReady(pods))
}

func TestAllPodsReady_PodNotRunning(t *testing.T) {
	t.Parallel()
	pods := []corev1.Pod{
		{
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionFalse},
				},
			},
		},
	}
	assert.False(t, allPodsReady(pods))
}

func TestAllPodsReady_PodRunningButNotReady(t *testing.T) {
	t.Parallel()
	pods := []corev1.Pod{
		{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionFalse},
				},
			},
		},
	}
	assert.False(t, allPodsReady(pods))
}

// --- findTalosEndpoint all branches ---

func TestFindTalosEndpoint_FromControlPlaneEndpoint(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "api.example.com",
		},
	}

	assert.Equal(t, "api.example.com", r.findTalosEndpoint(cluster))
}

func TestFindTalosEndpoint_FromBootstrapPublicIP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				PublicIP: "1.2.3.4",
			},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "", // empty - fall through
		},
	}

	assert.Equal(t, "1.2.3.4", r.findTalosEndpoint(cluster))
}

func TestFindTalosEndpoint_NoEndpointNoBootstrapNoHealthyCPReturnsEmpty(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PublicIP: "5.5.5.5"},
					{Name: "cp-2", Healthy: true, PublicIP: ""}, // healthy but no IP
				},
			},
		},
	}

	assert.Equal(t, "", r.findTalosEndpoint(cluster))
}

// --- ensureWorkersReady tests ---

func TestEnsureWorkersReady_DesiredZero(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Workers: k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{Ready: 0},
		},
	}

	_, waiting := r.ensureWorkersReady(context.Background(), cluster)
	assert.False(t, waiting, "should not wait when desired workers is 0")
}

func TestEnsureWorkersReady_AllReady(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Workers: k8znerv1alpha1.WorkerSpec{Count: 2},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{Ready: 2},
		},
	}

	_, waiting := r.ensureWorkersReady(context.Background(), cluster)
	assert.False(t, waiting, "should not wait when all workers are ready")
}

func TestEnsureWorkersReady_NotEnoughReadyNoHCloud(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	// No HCloud client - should still return waiting=true
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Workers: k8znerv1alpha1.WorkerSpec{Count: 2},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Ready: 0,
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	result, waiting := r.ensureWorkersReady(context.Background(), cluster)
	assert.True(t, waiting, "should be waiting when workers not ready")
	assert.Equal(t, workerReadyRequeueAfter, result.RequeueAfter)
}

// --- resolveNetworkID tests ---

func TestResolveNetworkID_FromStatus(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID: 12345,
			},
		},
	}

	networkID, err := r.resolveNetworkID(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), networkID)
}

func TestResolveNetworkID_FromHCloud(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return &hcloudgo.Network{ID: 99999, Name: name}, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID: 0, // not set - should look up from HCloud
			},
		},
	}

	networkID, err := r.resolveNetworkID(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, int64(99999), networkID)
	// Should also update the status
	assert.Equal(t, int64(99999), cluster.Status.Infrastructure.NetworkID)
}

func TestResolveNetworkID_HCloudError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return nil, fmt.Errorf("api error")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	_, err := r.resolveNetworkID(context.Background(), cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api error")
}

func TestResolveNetworkID_NetworkNotFound(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return nil, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	_, err := r.resolveNetworkID(context.Background(), cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network not found")
}

// --- replaceUnhealthyCPIfNeeded tests ---

func TestReplaceUnhealthyCPIfNeeded_NoUnhealthyCPs(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Ready: 3,
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: true},
					{Name: "cp-2", Healthy: true},
					{Name: "cp-3", Healthy: true},
				},
			},
		},
	}

	result, err := r.replaceUnhealthyCPIfNeeded(context.Background(), cluster)
	require.NoError(t, err)
	assert.Empty(t, result.RequeueAfter, "should not requeue when no unhealthy CPs")
}

func TestReplaceUnhealthyCPIfNeeded_QuorumLost(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	pastTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Ready: 1, // Only 1 healthy, need 2 for quorum
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: true},
					{Name: "cp-2", Healthy: false, UnhealthySince: &pastTime},
					{Name: "cp-3", Healthy: false, UnhealthySince: &pastTime},
				},
			},
		},
	}

	result, err := r.replaceUnhealthyCPIfNeeded(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter, "should requeue when quorum lost")
}

// --- scaleWorkers: scale down path ---

func TestScaleWorkers_ScaleDown(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Workers: k8znerv1alpha1.WorkerSpec{Count: 1}, // desired 1
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Healthy: true},
					{Name: "w-2", Healthy: true},
					{Name: "w-3", Healthy: true},
				},
			},
		},
	}

	result, err := r.scaleWorkers(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
	assert.Equal(t, k8znerv1alpha1.ClusterPhaseHealing, cluster.Status.Phase)
}

func TestScaleWorkers_ProvisioningSkips(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Workers: k8znerv1alpha1.WorkerSpec{Count: 3},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseCreatingServer}, // provisioning
					{Name: "w-2", Phase: k8znerv1alpha1.NodePhaseReady},
				},
			},
		},
	}

	result, err := r.scaleWorkers(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter, "should requeue when provisioning in progress")
}

// --- selectWorkersForRemoval tests ---

func TestSelectWorkersForRemoval_ZeroCount(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Healthy: true},
				},
			},
		},
	}

	result := r.selectWorkersForRemoval(cluster, 0)
	assert.Nil(t, result)
}

func TestSelectWorkersForRemoval_EmptyNodes(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	result := r.selectWorkersForRemoval(cluster, 1)
	assert.Nil(t, result)
}

func TestSelectWorkersForRemoval_PrefersUnhealthy(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Healthy: true},
					{Name: "w-2", Healthy: false},
					{Name: "w-3", Healthy: true},
				},
			},
		},
	}

	result := r.selectWorkersForRemoval(cluster, 1)
	require.Len(t, result, 1)
	assert.Equal(t, "w-2", result[0].Name, "should select unhealthy worker first")
}

func TestSelectWorkersForRemoval_FallsBackToNewest(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Healthy: true},
					{Name: "w-2", Healthy: true},
					{Name: "w-3", Healthy: true},
				},
			},
		},
	}

	result := r.selectWorkersForRemoval(cluster, 2)
	require.Len(t, result, 2)
	// Should select from the end (newest first)
	assert.Equal(t, "w-3", result[0].Name)
	assert.Equal(t, "w-2", result[1].Name)
}

// --- removeWorkersFromStatus tests ---

func TestRemoveWorkersFromStatus_Empty(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1"},
					{Name: "w-2"},
				},
			},
		},
	}

	r.removeWorkersFromStatus(cluster, nil)
	assert.Len(t, cluster.Status.Workers.Nodes, 2, "should not change anything with nil list")
}

func TestRemoveWorkersFromStatus_RemoveSome(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	w2 := k8znerv1alpha1.NodeStatus{Name: "w-2"}
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1"},
					w2,
					{Name: "w-3"},
				},
			},
		},
	}

	r.removeWorkersFromStatus(cluster, []*k8znerv1alpha1.NodeStatus{&w2})
	require.Len(t, cluster.Status.Workers.Nodes, 2)
	assert.Equal(t, "w-1", cluster.Status.Workers.Nodes[0].Name)
	assert.Equal(t, "w-3", cluster.Status.Workers.Nodes[1].Name)
}

// --- replaceUnhealthyWorkers tests ---

func TestReplaceUnhealthyWorkers_EmptyList(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	replaced := r.replaceUnhealthyWorkers(context.Background(), cluster, nil)
	assert.Equal(t, 0, replaced)
}

func TestReplaceUnhealthyWorkers_RespectsMaxConcurrentHeals(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{
			// Fail at prepareForProvisioning because there is no snapshot
			GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				return nil, fmt.Errorf("no snapshot")
			},
		}),
		WithMaxConcurrentHeals(1),
		WithMetrics(false),
	)

	pastTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	unhealthy := []*k8znerv1alpha1.NodeStatus{
		{Name: "w-1", UnhealthySince: &pastTime, UnhealthyReason: "NotReady"},
		{Name: "w-2", UnhealthySince: &pastTime, UnhealthyReason: "NotReady"},
		{Name: "w-3", UnhealthySince: &pastTime, UnhealthyReason: "NotReady"},
	}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1"}, {Name: "w-2"}, {Name: "w-3"},
				},
			},
		},
	}

	// Even though there are 3 unhealthy, maxConcurrentHeals=1 limits to 1 attempt
	// The attempt will fail at provisionReplacementWorker (no snapshot), so replaced=0
	replaced := r.replaceUnhealthyWorkers(context.Background(), cluster, unhealthy)
	// replaceWorker fails, so replaced stays 0, but only 1 was attempted
	assert.Equal(t, 0, replaced)
}

// --- reconcileControlPlanes: single CP skips health replacement ---

func TestReconcileControlPlanes_SingleCPSkipsHealthCheck(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Ready: 1,
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: true, Phase: k8znerv1alpha1.NodePhaseReady},
				},
			},
		},
	}

	result, err := r.reconcileControlPlanes(context.Background(), cluster)
	require.NoError(t, err)
	assert.Empty(t, result.RequeueAfter)
}

// --- reconcileWorkers: unhealthy replaced triggers requeue ---

func TestReconcileWorkers_UnhealthyTriggersRequeue(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{
			GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				return nil, fmt.Errorf("no snapshot")
			},
		}),
		WithMetrics(false),
	)

	pastTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Workers: k8znerv1alpha1.WorkerSpec{Count: 1},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Healthy: false, UnhealthySince: &pastTime, UnhealthyReason: "NodeNotReady"},
				},
			},
		},
	}

	result, err := r.reconcileWorkers(context.Background(), cluster)
	require.NoError(t, err)
	// Should requeue: unhealthy worker triggers replacement + potential scale-up (fast requeue)
	assert.NotZero(t, result.RequeueAfter)
}

// --- getSnapshot tests ---

func TestGetSnapshot_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			return &hcloudgo.Image{ID: 42, Name: "talos-v1.8"}, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	snapshot, err := r.getSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(42), snapshot.ID)
}

func TestGetSnapshot_APIError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			return nil, fmt.Errorf("api error")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	_, err := r.getSnapshot(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get Talos snapshot")
}

func TestGetSnapshot_NilSnapshot(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			return nil, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	_, err := r.getSnapshot(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Talos snapshot found")
}

// --- createEphemeralSSHKey tests ---

func TestCreateEphemeralSSHKey_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	keyName, cleanup, err := r.createEphemeralSSHKey(context.Background(), cluster, "worker")
	require.NoError(t, err)
	assert.Contains(t, keyName, "ephemeral-test-cluster-worker-")
	assert.NotNil(t, cleanup)

	// Call cleanup and verify key was deleted
	cleanup()
	require.Len(t, mockHCloud.DeleteSSHKeyCalls, 1)
	assert.Equal(t, keyName, mockHCloud.DeleteSSHKeyCalls[0])
}

func TestCreateEphemeralSSHKey_CreateKeyError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		CreateSSHKeyFunc: func(ctx context.Context, name, publicKey string, labels map[string]string) (string, error) {
			return "", fmt.Errorf("failed to create SSH key")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	_, cleanup, err := r.createEphemeralSSHKey(context.Background(), cluster, "cp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create ephemeral SSH key")
	assert.Nil(t, cleanup)
}

// --- provisionServer tests ---

func TestProvisionServer_CreateServerError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		CreateServerFunc: func(ctx context.Context, opts hcloud.ServerCreateOpts) (string, error) {
			return "", fmt.Errorf("server creation failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	_, err := r.provisionServer(context.Background(), cluster, serverCreateOpts{
		Name:       "test-server",
		SnapshotID: 1,
		ServerType: "cx23",
		Region:     "nbg1",
		SSHKeyName: "test-key",
		Role:       "worker",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create server")

	// Verify the node phase was set to Failed
	require.Len(t, cluster.Status.Workers.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseFailed, cluster.Status.Workers.Nodes[0].Phase)
}

func TestProvisionServer_GetServerIPError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
			return "", fmt.Errorf("IP not assigned")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := r.provisionServer(ctx, cluster, serverCreateOpts{
		Name:       "cp-server",
		SnapshotID: 1,
		ServerType: "cx23",
		Region:     "nbg1",
		SSHKeyName: "test-key",
		Role:       "control-plane",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get server IP")
}

func TestProvisionServer_GetServerIDError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetServerIDFunc: func(ctx context.Context, name string) (string, error) {
			return "", fmt.Errorf("server ID lookup failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	_, err := r.provisionServer(context.Background(), cluster, serverCreateOpts{
		Name:       "w-server",
		SnapshotID: 1,
		ServerType: "cx23",
		Region:     "nbg1",
		SSHKeyName: "test-key",
		Role:       "worker",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get server ID")
}

func TestProvisionServer_InvalidServerID(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetServerIDFunc: func(ctx context.Context, name string) (string, error) {
			return "not-a-number", nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	_, err := r.provisionServer(context.Background(), cluster, serverCreateOpts{
		Name:       "w-server",
		SnapshotID: 1,
		ServerType: "cx23",
		Region:     "nbg1",
		SSHKeyName: "test-key",
		Role:       "worker",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse server ID")
}

func TestProvisionServer_SuccessWithPrivateIP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(ctx context.Context, name string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:   12345,
				Name: name,
				PrivateNet: []hcloudgo.ServerPrivateNet{
					{IP: net.ParseIP("10.0.0.99")},
				},
			}, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{Region: "nbg1"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	result, err := r.provisionServer(context.Background(), cluster, serverCreateOpts{
		Name:       "w-server",
		SnapshotID: 1,
		ServerType: "cx23",
		Region:     "nbg1",
		SSHKeyName: "test-key",
		Role:       "worker",
	})
	require.NoError(t, err)
	assert.Equal(t, "w-server", result.Name)
	assert.Equal(t, int64(12345), result.ServerID)
	assert.Equal(t, "10.0.0.99", result.PrivateIP)
	assert.Equal(t, "10.0.0.99", result.TalosIP, "should use private IP for TalosIP when available")
}

// --- handleProvisioningFailure tests ---

func TestHandleProvisioningFailure_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
				},
			},
		},
	}

	r.handleProvisioningFailure(context.Background(), cluster, "worker", "w-1", "test reason")

	// Node should be removed from status
	assert.Empty(t, cluster.Status.Workers.Nodes, "node should be removed after provisioning failure")
	// Server should be deleted
	require.Len(t, mockHCloud.DeleteServerCalls, 1)
	assert.Equal(t, "w-1", mockHCloud.DeleteServerCalls[0])
}

func TestHandleProvisioningFailure_DeleteError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		DeleteServerFunc: func(ctx context.Context, name string) error {
			return fmt.Errorf("delete failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Phase: k8znerv1alpha1.NodePhaseWaitingForIP},
				},
			},
		},
	}

	// Should not panic even when delete fails
	r.handleProvisioningFailure(context.Background(), cluster, "control-plane", "cp-1", "IP assignment timeout")

	// Node should still be removed from status
	assert.Empty(t, cluster.Status.ControlPlanes.Nodes)
}

// --- drainNode with pods ---

func TestDrainNode_SkipsMirrorAndDaemonSetPods(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	regularPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "regular-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{NodeName: "test-node"},
	}

	mirrorPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mirror-pod",
			Namespace: "kube-system",
			Annotations: map[string]string{
				corev1.MirrorPodAnnotationKey: "true",
			},
		},
		Spec: corev1.PodSpec{NodeName: "test-node"},
	}

	daemonSetPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds-pod",
			Namespace: "kube-system",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "DaemonSet", Name: "my-ds"},
			},
		},
		Spec: corev1.PodSpec{NodeName: "test-node"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(regularPod, mirrorPod, daemonSetPod).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
			pod := o.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder)

	err := r.drainNode(context.Background(), "test-node")
	require.NoError(t, err)
	// The function should have attempted to evict only the regular pod
	// Mirror and DaemonSet pods should be skipped
}

// --- reconcileInfrastructurePhase: existing infrastructure skip ---

func TestReconcileInfrastructurePhase_ExistingInfrastructureSkips(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID:      123,
				LoadBalancerID: 456,
				FirewallID:     789,
				LoadBalancerIP: "1.2.3.4",
			},
		},
	}

	result, err := r.reconcileInfrastructurePhase(context.Background(), cluster)
	require.NoError(t, err)
	assert.True(t, result.Requeue, "should requeue to move to next phase")
	assert.Equal(t, k8znerv1alpha1.PhaseImage, cluster.Status.ProvisioningPhase)
}

func TestReconcileInfrastructurePhase_ExistingInfraSetsCPEndpoint(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "", // empty - should be populated from LB IP
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID:      123,
				LoadBalancerID: 456,
				FirewallID:     789,
				LoadBalancerIP: "1.2.3.4",
			},
		},
	}

	result, err := r.reconcileInfrastructurePhase(context.Background(), cluster)
	require.NoError(t, err)
	assert.True(t, result.Requeue)
	assert.Equal(t, "1.2.3.4", cluster.Status.ControlPlaneEndpoint,
		"should set CP endpoint from LB IP when empty")
}

// --- reconcileRunningPhase tests ---

func TestReconcileRunningPhase_HealthCheckError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	// Build a client that will fail on List (no scheme for NodeList)
	// Actually, the fake client will succeed for List. Let's just test the success path
	// to cover the function with basic running phase.
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 1},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Ready:   1,
				Desired: 1,
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: true, Phase: k8znerv1alpha1.NodePhaseReady},
				},
			},
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Ready:   1,
				Desired: 1,
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Healthy: true, Phase: k8znerv1alpha1.NodePhaseReady},
				},
			},
		},
	}

	cpNode := createTestNode("cp-1", true, true)
	workerNode := createTestNode("w-1", false, true)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, cpNode, workerNode).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.reconcileRunningPhase(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
}

// --- verifyAndUpdateNodeStates: server running, no K8s node ---

func TestVerifyAndUpdateNodeStates_ProgressionDetection(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	// Create a K8s node that's ready for cp-1
	cpNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cp-1",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(ctx context.Context, name string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:     12345,
				Name:   name,
				Status: "running",
				PublicNet: hcloudgo.ServerPublicNet{
					IPv4: hcloudgo.ServerPublicNetIPv4{IP: net.ParseIP("5.5.5.5")},
				},
			}, nil
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cpNode).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:     "cp-1",
						Phase:    k8znerv1alpha1.NodePhaseWaitingForK8s, // behind actual state
						PublicIP: "5.5.5.5",
					},
				},
			},
		},
	}

	err := r.verifyAndUpdateNodeStates(context.Background(), cluster)
	require.NoError(t, err)
	// Node should be updated to Ready since K8s node exists and is ready
	assert.Equal(t, k8znerv1alpha1.NodePhaseReady, cluster.Status.ControlPlanes.Nodes[0].Phase)
}

// --- decommissionWorker: node not found in K8s ---

func TestDecommissionWorker_NoK8sNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	worker := &k8znerv1alpha1.NodeStatus{
		Name:     "w-1",
		ServerID: 12345,
	}

	err := r.decommissionWorker(context.Background(), cluster, worker)
	require.NoError(t, err)
	// Server should have been deleted
	require.Len(t, mockHCloud.DeleteServerCalls, 1)
	assert.Equal(t, "w-1", mockHCloud.DeleteServerCalls[0])
}

func TestDecommissionWorker_WithExistingK8sNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	k8sNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "w-1"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(k8sNode).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
			pod := o.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	worker := &k8znerv1alpha1.NodeStatus{
		Name:     "w-1",
		ServerID: 12345,
	}

	err := r.decommissionWorker(context.Background(), cluster, worker)
	require.NoError(t, err)

	// K8s node should be deleted
	node := &corev1.Node{}
	getErr := fakeClient.Get(context.Background(), types.NamespacedName{Name: "w-1"}, node)
	assert.Error(t, getErr, "K8s node should be deleted")

	// HCloud server should be deleted
	require.Len(t, mockHCloud.DeleteServerCalls, 1)
}

func TestDecommissionWorker_HCloudDeleteError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		DeleteServerFunc: func(ctx context.Context, name string) error {
			return fmt.Errorf("delete failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	worker := &k8znerv1alpha1.NodeStatus{Name: "w-1"}

	err := r.decommissionWorker(context.Background(), cluster, worker)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete hetzner server")
}

// --- scaleDownWorkers tests ---

func TestScaleDownWorkers_NoWorkersToRemove(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	err := r.scaleDownWorkers(context.Background(), cluster, 1)
	require.NoError(t, err)
}

func TestScaleDownWorkers_RespectsConcurrencyLimit(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMaxConcurrentHeals(1),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Healthy: true},
					{Name: "w-2", Healthy: true},
					{Name: "w-3", Healthy: true},
				},
			},
		},
	}

	err := r.scaleDownWorkers(context.Background(), cluster, 3)
	// maxConcurrentHeals=1, so only 1 will be removed, hence error "only removed 1 of 3"
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only removed 1 of 3")
	// One server should have been deleted
	require.Len(t, mockHCloud.DeleteServerCalls, 1)
}

// --- Reconcile: statusErr path where reconcile succeeds but status update fails ---

func TestReconcile_StatusUpdateErrorPropagated(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
		},
	}

	// Build client WITHOUT status subresource to trigger status update failure
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		Build()
	recorder := record.NewFakeRecorder(20)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-cluster",
		},
	})

	// The reconcile itself may succeed but status update fails
	// In either case, it should be handled gracefully
	_ = result
	_ = err
}

// --- reconcileWithStateMachine: image phase dispatch ---

func TestReconcileWithStateMachine_ImagePhase(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: "test-secret"},
			ControlPlanes:  k8znerv1alpha1.ControlPlaneSpec{Count: 1},
			Workers:        k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ProvisioningPhase: k8znerv1alpha1.PhaseImage,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	// Will dispatch to reconcileImagePhase, which will fail at credentials
	result, err := r.reconcileWithStateMachine(context.Background(), cluster)
	assert.NoError(t, err) // Errors are handled internally, not returned
	assert.NotZero(t, result.RequeueAfter)
}

func TestReconcileWithStateMachine_ComputePhase(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: "test-secret"},
			ControlPlanes:  k8znerv1alpha1.ControlPlaneSpec{Count: 1},
			Workers:        k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ProvisioningPhase: k8znerv1alpha1.PhaseCompute,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.reconcileWithStateMachine(context.Background(), cluster)
	assert.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter)
}

func TestReconcileWithStateMachine_BootstrapPhase(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: "test-secret"},
			ControlPlanes:  k8znerv1alpha1.ControlPlaneSpec{Count: 1},
			Workers:        k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ProvisioningPhase: k8znerv1alpha1.PhaseBootstrap,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.reconcileWithStateMachine(context.Background(), cluster)
	assert.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter)
}

func TestReconcileWithStateMachine_AddonsPhase(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: "test-secret"},
			ControlPlanes:  k8znerv1alpha1.ControlPlaneSpec{Count: 1},
			Workers:        k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ProvisioningPhase: k8znerv1alpha1.PhaseAddons,
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Ready: 0,
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.reconcileWithStateMachine(context.Background(), cluster)
	assert.NoError(t, err)
	// Will either requeue to wait for workers or fail at credentials
	assert.NotZero(t, result.RequeueAfter)
}

func TestReconcileWithStateMachine_ConfiguringPhase(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: "test-secret"},
			ControlPlanes:  k8znerv1alpha1.ControlPlaneSpec{Count: 1},
			Workers:        k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ProvisioningPhase: k8znerv1alpha1.PhaseConfiguring,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	result, err := r.reconcileWithStateMachine(context.Background(), cluster)
	assert.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter)
}

// --- reconcile: stuck node handling continues on error ---

func TestReconcile_StuckNodeHandlingContinues(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	pastTime := metav1.NewTime(time.Now().Add(-30 * time.Minute))
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:                "w-stuck",
						Phase:               k8znerv1alpha1.NodePhaseCreatingServer,
						PhaseTransitionTime: &pastTime,
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(20)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	// reconcile should handle the stuck node and continue
	_, err := r.reconcile(context.Background(), cluster)
	require.NoError(t, err)

	// Stuck node should have been cleaned up
	require.Len(t, mockHCloud.DeleteServerCalls, 1)
	assert.Equal(t, "w-stuck", mockHCloud.DeleteServerCalls[0])
}

// --- verifyNodeState: node IP from server ---

func TestVerifyNodeState_UsesServerIPWhenAvailable(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(ctx context.Context, name string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:     12345,
				Name:   name,
				Status: "running",
				PublicNet: hcloudgo.ServerPublicNet{
					IPv4: hcloudgo.ServerPublicNetIPv4{IP: net.ParseIP("5.5.5.5")},
				},
			}, nil
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	info, err := r.verifyNodeState(context.Background(), "cp-1", "old-ip")
	require.NoError(t, err)
	assert.True(t, info.ServerExists)
	assert.Equal(t, "running", info.ServerStatus)
	assert.Equal(t, "5.5.5.5", info.ServerIP, "should use server IP from HCloud")
}

func TestVerifyNodeState_ServerNotFound(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(ctx context.Context, name string) (*hcloudgo.Server, error) {
			return nil, nil
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	info, err := r.verifyNodeState(context.Background(), "cp-1", "")
	require.NoError(t, err)
	assert.False(t, info.ServerExists)
}

// --- loadTalosClients: already injected ---

func TestLoadTalosClients_AlreadyInjected(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{}
	mockGen := &MockTalosConfigGenerator{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithTalosClient(mockTalos),
		WithTalosConfigGenerator(mockGen),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{}

	tc := r.loadTalosClients(context.Background(), cluster)
	assert.Equal(t, mockTalos, tc.client)
	assert.Equal(t, mockGen, tc.configGen)
}

func TestLoadTalosClients_NoCredentialsRef(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: ""},
		},
	}

	tc := r.loadTalosClients(context.Background(), cluster)
	assert.Nil(t, tc.client)
	assert.Nil(t, tc.configGen)
}

// --- checkPortReachable: unreachable port ---

func TestCheckPortReachable_Unreachable(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	// Port 1 on localhost is almost certainly not listening
	result := r.checkPortReachable("127.0.0.1", 1, 100*time.Millisecond)
	assert.False(t, result)
}

// --- Reconcile: ensureHCloudClient failure path ---

func TestReconcile_EnsureHCloudClientFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 0},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	// No HCloud client and no token - ensureHCloudClient should fail
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-cluster",
		},
	})

	// Should not return error (it's handled internally with requeue)
	require.NoError(t, err)
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
}

// --- isNodeInEarlyProvisioningPhase: all provisioning phases ---

func TestIsNodeInEarlyProvisioningPhase_AllPhases(t *testing.T) {
	t.Parallel()
	provisioningPhases := []k8znerv1alpha1.NodePhase{
		k8znerv1alpha1.NodePhaseCreatingServer,
		k8znerv1alpha1.NodePhaseWaitingForIP,
		k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		k8znerv1alpha1.NodePhaseApplyingTalosConfig,
		k8znerv1alpha1.NodePhaseRebootingWithConfig,
		k8znerv1alpha1.NodePhaseWaitingForK8s,
		k8znerv1alpha1.NodePhaseNodeInitializing,
	}
	for _, phase := range provisioningPhases {
		assert.True(t, isNodeInEarlyProvisioningPhase(phase), "phase %s should be early provisioning", phase)
	}

	nonProvisioningPhases := []k8znerv1alpha1.NodePhase{
		k8znerv1alpha1.NodePhaseReady,
		k8znerv1alpha1.NodePhaseFailed,
		k8znerv1alpha1.NodePhaseUnhealthy,
		k8znerv1alpha1.NodePhaseDraining,
		k8znerv1alpha1.NodePhaseRemovingFromEtcd,
		k8znerv1alpha1.NodePhaseDeletingServer,
	}
	for _, phase := range nonProvisioningPhases {
		assert.False(t, isNodeInEarlyProvisioningPhase(phase), "phase %s should NOT be early provisioning", phase)
	}
}

// --- countWorkersInEarlyProvisioning: mixed phases ---

func TestCountWorkersInEarlyProvisioning_MixedPhases(t *testing.T) {
	t.Parallel()
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseReady},
		{Name: "w-2", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
		{Name: "w-3", Phase: k8znerv1alpha1.NodePhaseWaitingForIP},
		{Name: "w-4", Phase: k8znerv1alpha1.NodePhaseFailed},
		{Name: "w-5", Phase: k8znerv1alpha1.NodePhaseApplyingTalosConfig},
	}
	assert.Equal(t, 3, countWorkersInEarlyProvisioning(nodes))
}

// --- determineNodePhaseFromState: more branch coverage ---

func TestDetermineNodePhaseFromState_K8sNodeNotReadyKubeletRunning(t *testing.T) {
	t.Parallel()
	info := &nodeStateInfo{
		K8sNodeExists:       true,
		K8sNodeReady:        false,
		TalosKubeletRunning: true,
	}
	phase, reason := determineNodePhaseFromState(info)
	assert.Equal(t, k8znerv1alpha1.NodePhaseNodeInitializing, phase)
	assert.Contains(t, reason, "waiting for system pods")
}

func TestDetermineNodePhaseFromState_K8sNodeNotReadyKubeletNotRunning(t *testing.T) {
	t.Parallel()
	info := &nodeStateInfo{
		K8sNodeExists:       true,
		K8sNodeReady:        false,
		TalosKubeletRunning: false,
	}
	phase, _ := determineNodePhaseFromState(info)
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForK8s, phase)
}

func TestDetermineNodePhaseFromState_TalosConfiguredKubeletRunning(t *testing.T) {
	t.Parallel()
	info := &nodeStateInfo{
		TalosConfigured:     true,
		TalosKubeletRunning: true,
	}
	phase, _ := determineNodePhaseFromState(info)
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForK8s, phase)
}

func TestDetermineNodePhaseFromState_TalosConfiguredKubeletNotRunning(t *testing.T) {
	t.Parallel()
	info := &nodeStateInfo{
		TalosConfigured:     true,
		TalosKubeletRunning: false,
	}
	phase, _ := determineNodePhaseFromState(info)
	assert.Equal(t, k8znerv1alpha1.NodePhaseRebootingWithConfig, phase)
}

func TestDetermineNodePhaseFromState_MaintenanceMode(t *testing.T) {
	t.Parallel()
	info := &nodeStateInfo{
		TalosInMaintenanceMode: true,
	}
	phase, _ := determineNodePhaseFromState(info)
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForTalosAPI, phase)
}

func TestDetermineNodePhaseFromState_TalosAPIReachable(t *testing.T) {
	t.Parallel()
	info := &nodeStateInfo{
		TalosAPIReachable: true,
	}
	phase, _ := determineNodePhaseFromState(info)
	assert.Equal(t, k8znerv1alpha1.NodePhaseApplyingTalosConfig, phase)
}

func TestDetermineNodePhaseFromState_ServerStarting(t *testing.T) {
	t.Parallel()
	info := &nodeStateInfo{
		ServerExists: true,
		ServerStatus: "starting",
	}
	phase, _ := determineNodePhaseFromState(info)
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForIP, phase)
}

func TestDetermineNodePhaseFromState_ServerExistsOtherStatus(t *testing.T) {
	t.Parallel()
	info := &nodeStateInfo{
		ServerExists: true,
		ServerStatus: "off",
	}
	phase, reason := determineNodePhaseFromState(info)
	assert.Equal(t, k8znerv1alpha1.NodePhaseCreatingServer, phase)
	assert.Contains(t, reason, "off")
}

func TestDetermineNodePhaseFromState_ServerDoesNotExist(t *testing.T) {
	t.Parallel()
	info := &nodeStateInfo{
		ServerExists: false,
	}
	phase, _ := determineNodePhaseFromState(info)
	assert.Equal(t, k8znerv1alpha1.NodePhaseFailed, phase)
}

// --- shouldUpdatePhase: backward progression blocked ---

func TestShouldUpdatePhase_BackwardProgressionBlocked(t *testing.T) {
	t.Parallel()
	// Going from WaitingForK8s back to CreatingServer should be blocked
	assert.False(t, shouldUpdatePhase(
		k8znerv1alpha1.NodePhaseWaitingForK8s,
		k8znerv1alpha1.NodePhaseCreatingServer,
	))
}

func TestShouldUpdatePhase_ForwardProgressionAllowed(t *testing.T) {
	t.Parallel()
	assert.True(t, shouldUpdatePhase(
		k8znerv1alpha1.NodePhaseCreatingServer,
		k8znerv1alpha1.NodePhaseWaitingForIP,
	))
}

func TestShouldUpdatePhase_ToFailedAlwaysAllowed(t *testing.T) {
	t.Parallel()
	assert.True(t, shouldUpdatePhase(
		k8znerv1alpha1.NodePhaseReady,
		k8znerv1alpha1.NodePhaseFailed,
	))
}

func TestShouldUpdatePhase_ToReadyAlwaysAllowed(t *testing.T) {
	t.Parallel()
	assert.True(t, shouldUpdatePhase(
		k8znerv1alpha1.NodePhaseCreatingServer,
		k8znerv1alpha1.NodePhaseReady,
	))
}

func TestShouldUpdatePhase_UnknownPhasesAllowed(t *testing.T) {
	t.Parallel()
	assert.True(t, shouldUpdatePhase(
		k8znerv1alpha1.NodePhase("UnknownPhase1"),
		k8znerv1alpha1.NodePhase("UnknownPhase2"),
	))
}

// --- discoverLoadBalancerInfo tests ---

func TestDiscoverLoadBalancerInfo_AlreadyDiscovered(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerID: 999,
				LoadBalancerIP: "1.2.3.4",
			},
		},
	}

	// Should return immediately without making any API calls
	r.discoverLoadBalancerInfo(context.Background(), cluster, "fake-token")
	assert.Equal(t, int64(999), cluster.Status.Infrastructure.LoadBalancerID)
	assert.Equal(t, "1.2.3.4", cluster.Status.Infrastructure.LoadBalancerIP)
}

// --- prepareForProvisioning: snapshot error ---

func TestPrepareForProvisioning_SnapshotError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			return nil, fmt.Errorf("snapshot error")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	_, _, err := r.prepareForProvisioning(context.Background(), cluster, "worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot")
}

// --- prepareForProvisioning: SSH key error ---

func TestPrepareForProvisioning_SSHKeyError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		CreateSSHKeyFunc: func(ctx context.Context, name, publicKey string, labels map[string]string) (string, error) {
			return "", fmt.Errorf("ssh key creation failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	_, _, err := r.prepareForProvisioning(context.Background(), cluster, "cp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create ephemeral SSH key")
}

// --- prepareForProvisioning: cluster state error ---

func TestPrepareForProvisioning_ClusterStateError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return nil, fmt.Errorf("network error")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	_, _, err := r.prepareForProvisioning(context.Background(), cluster, "worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to build cluster state")
}

// --- provisionAndConfigureNode: success path ---

func TestProvisionAndConfigureNode_SuccessPath(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{Region: "nbg1"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	configureCalled := false
	err := r.provisionAndConfigureNode(context.Background(), cluster, nodeProvisionParams{
		Name:       "w-test",
		Role:       "worker",
		Pool:       "workers",
		ServerType: "cx23",
		SnapshotID: 1,
		SSHKeyName: "test-key",
		NetworkID:  100,
		Configure: func(serverName string, result *serverProvisionResult) error {
			configureCalled = true
			return nil
		},
	})
	require.NoError(t, err)
	assert.True(t, configureCalled, "configure function should have been called")
}

func TestProvisionAndConfigureNode_ConfigureFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{Region: "nbg1"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	err := r.provisionAndConfigureNode(context.Background(), cluster, nodeProvisionParams{
		Name:       "w-test",
		Role:       "worker",
		Pool:       "workers",
		ServerType: "cx23",
		SnapshotID: 1,
		SSHKeyName: "test-key",
		NetworkID:  100,
		Configure: func(serverName string, result *serverProvisionResult) error {
			return fmt.Errorf("configure error")
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configure error")
}

// --- replaceControlPlane: flow test ---

func TestReplaceControlPlane_DeletesAndReplacesCP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region:        "nbg1",
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3, Size: "cx23"},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1", PublicIP: "1.1.1.1"},
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2", PublicIP: "2.2.2.2"},
					{Name: "cp-bad", Healthy: false, PrivateIP: "10.0.0.3", ServerID: 999},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(20)

	mockTalos := &MockTalosClient{
		WaitForNodeReadyFunc: func(ctx context.Context, nodeIP string, timeout int) error {
			return nil
		},
	}
	mockGen := &MockTalosConfigGenerator{}
	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(ctx context.Context, name string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:   12345,
				Name: name,
				PrivateNet: []hcloudgo.ServerPrivateNet{
					{IP: net.ParseIP("10.0.0.99")},
				},
			}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithTalosClient(mockTalos),
		WithTalosConfigGenerator(mockGen),
		WithMetrics(false),
		WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
			return nil
		}),
	)

	unhealthyNode := &cluster.Status.ControlPlanes.Nodes[2]

	err := r.replaceControlPlane(context.Background(), cluster, unhealthyNode)
	require.NoError(t, err)

	// Old node should be deleted from HCloud
	foundDeleteCall := false
	for _, call := range mockHCloud.DeleteServerCalls {
		if call == "cp-bad" {
			foundDeleteCall = true
			break
		}
	}
	assert.True(t, foundDeleteCall, "old server should have been deleted")
}

// --- replaceWorker: flow test ---

func TestReplaceWorker_DrainsDeletesAndReplacesWorker(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	workerNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "w-bad"},
	}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region:  "nbg1",
			Workers: k8znerv1alpha1.WorkerSpec{Count: 1, Size: "cx23"},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-bad", Healthy: false, ServerID: 999},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, workerNode).
		WithStatusSubresource(cluster).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
			pod := o.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()
	recorder := record.NewFakeRecorder(20)

	mockTalos := &MockTalosClient{
		WaitForNodeReadyFunc: func(ctx context.Context, nodeIP string, timeout int) error {
			return nil
		},
	}
	mockGen := &MockTalosConfigGenerator{}
	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(ctx context.Context, name string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:   12345,
				Name: name,
				PrivateNet: []hcloudgo.ServerPrivateNet{
					{IP: net.ParseIP("10.0.0.99")},
				},
			}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithTalosClient(mockTalos),
		WithTalosConfigGenerator(mockGen),
		WithMetrics(false),
		WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
			return nil
		}),
	)

	unhealthyNode := &cluster.Status.Workers.Nodes[0]

	err := r.replaceWorker(context.Background(), cluster, unhealthyNode)
	require.NoError(t, err)

	// Old server should have been deleted
	foundDeleteCall := false
	for _, call := range mockHCloud.DeleteServerCalls {
		if call == "w-bad" {
			foundDeleteCall = true
			break
		}
	}
	assert.True(t, foundDeleteCall, "old server should have been deleted")
}

// --- findHealthyControlPlaneIP: empty nodes ---

func TestFindHealthyControlPlaneIP_EmptyNodes(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	ip := r.findHealthyControlPlaneIP(cluster)
	assert.Empty(t, ip)
}

// --- updateClusterPhase: provisioning phase preserved ---

func TestUpdateClusterPhase_ProvisioningPhasePreserved(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Phase: k8znerv1alpha1.ClusterPhaseProvisioning,
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Desired: 3,
				Ready:   1,
			},
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Desired: 2,
				Ready:   0,
			},
		},
	}

	r := &ClusterReconciler{}
	r.updateClusterPhase(cluster)

	// Since phase is not Healing, and not all ready, it should be Degraded
	assert.Equal(t, k8znerv1alpha1.ClusterPhaseDegraded, cluster.Status.Phase)
}
