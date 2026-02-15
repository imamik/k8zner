package controller

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDetermineNodePhaseFromState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		info          *nodeStateInfo
		expectedPhase k8znerv1alpha1.NodePhase
	}{
		{
			name: "K8s node exists and ready",
			info: &nodeStateInfo{
				ServerExists:  true,
				ServerStatus:  "running",
				K8sNodeExists: true,
				K8sNodeReady:  true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseReady,
		},
		{
			name: "K8s node exists, not ready, kubelet running",
			info: &nodeStateInfo{
				ServerExists:        true,
				ServerStatus:        "running",
				K8sNodeExists:       true,
				K8sNodeReady:        false,
				TalosKubeletRunning: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseNodeInitializing,
		},
		{
			name: "K8s node exists, not ready, kubelet not running",
			info: &nodeStateInfo{
				ServerExists:        true,
				ServerStatus:        "running",
				K8sNodeExists:       true,
				K8sNodeReady:        false,
				TalosKubeletRunning: false,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForK8s,
		},
		{
			name: "Talos configured, kubelet running, no K8s node",
			info: &nodeStateInfo{
				ServerExists:        true,
				ServerStatus:        "running",
				TalosConfigured:     true,
				TalosKubeletRunning: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForK8s,
		},
		{
			name: "Talos configured, kubelet not running",
			info: &nodeStateInfo{
				ServerExists:        true,
				ServerStatus:        "running",
				TalosConfigured:     true,
				TalosKubeletRunning: false,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseRebootingWithConfig,
		},
		{
			name: "Talos in maintenance mode",
			info: &nodeStateInfo{
				ServerExists:           true,
				ServerStatus:           "running",
				TalosAPIReachable:      true,
				TalosInMaintenanceMode: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		},
		{
			name: "Talos API reachable but not in maintenance",
			info: &nodeStateInfo{
				ServerExists:      true,
				ServerStatus:      "running",
				TalosAPIReachable: true,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseApplyingTalosConfig,
		},
		{
			name: "Server running, Talos not reachable",
			info: &nodeStateInfo{
				ServerExists: true,
				ServerStatus: "running",
				ServerIP:     "10.0.0.1",
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
		},
		{
			name: "Server starting",
			info: &nodeStateInfo{
				ServerExists: true,
				ServerStatus: "starting",
			},
			expectedPhase: k8znerv1alpha1.NodePhaseWaitingForIP,
		},
		{
			name: "Server exists in unknown state",
			info: &nodeStateInfo{
				ServerExists: true,
				ServerStatus: "initializing",
			},
			expectedPhase: k8znerv1alpha1.NodePhaseCreatingServer,
		},
		{
			name: "Server does not exist",
			info: &nodeStateInfo{
				ServerExists: false,
			},
			expectedPhase: k8znerv1alpha1.NodePhaseFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			phase, reason := determineNodePhaseFromState(tt.info)
			assert.Equal(t, tt.expectedPhase, phase)
			assert.NotEmpty(t, reason, "reason should always be provided")
		})
	}
}

func TestDetermineNodePhaseFromState_ReasonContent(t *testing.T) {
	t.Parallel()
	t.Run("ready node has descriptive reason", func(t *testing.T) {
		t.Parallel()
		info := &nodeStateInfo{
			K8sNodeExists: true,
			K8sNodeReady:  true,
		}
		_, reason := determineNodePhaseFromState(info)
		assert.Contains(t, reason, "ready")
	})

	t.Run("failed node mentions HCloud", func(t *testing.T) {
		t.Parallel()
		info := &nodeStateInfo{
			ServerExists: false,
		}
		_, reason := determineNodePhaseFromState(info)
		assert.Contains(t, reason, "HCloud")
	})

	t.Run("waiting for talos includes IP in reason", func(t *testing.T) {
		t.Parallel()
		info := &nodeStateInfo{
			ServerExists: true,
			ServerStatus: "running",
			ServerIP:     "10.0.0.42",
		}
		_, reason := determineNodePhaseFromState(info)
		assert.Contains(t, reason, "10.0.0.42")
	})
}

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
