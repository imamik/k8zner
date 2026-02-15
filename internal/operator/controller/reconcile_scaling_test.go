package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

func TestClusterReconciler_reconcileControlPlanes(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("skips replacement for single control plane", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(&MockHCloudClient{}),
		)

		result, err := r.reconcileControlPlanes(context.Background(), cluster)

		assert.NoError(t, err)
		assert.False(t, result.Requeue)
	})

	t.Run("replaces unhealthy control plane with quorum", func(t *testing.T) {
		t.Parallel()
		unhealthySince := metav1.NewTime(time.Now().Add(-10 * time.Minute))
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Desired: 3,
					Ready:   2,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1"},
						{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
						{Name: "cp-3", Healthy: false, PrivateIP: "10.0.0.3", ServerID: 12345, UnhealthySince: &unhealthySince, UnhealthyReason: "NodeNotReady"},
					},
				},
			},
		}

		cpNode3 := createTestNode("cp-3", true, false)

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, cpNode3).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}
		mockTalos := &MockTalosClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithTalosClient(mockTalos),
			WithMetrics(false),
		)

		result, err := r.reconcileControlPlanes(context.Background(), cluster)

		assert.NoError(t, err)
		assert.True(t, result.RequeueAfter > 0)
		assert.Equal(t, k8znerv1alpha1.ClusterPhaseHealing, cluster.Status.Phase)

		// Verify etcd member was attempted to be removed
		assert.Len(t, mockTalos.GetEtcdMembersCalls, 1)

		// Verify server was deleted
		assert.Contains(t, mockHCloud.DeleteServerCalls, "cp-3")
	})

	t.Run("refuses replacement without quorum", func(t *testing.T) {
		t.Parallel()
		unhealthySince := metav1.NewTime(time.Now().Add(-10 * time.Minute))
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Desired: 3,
					Ready:   1, // Only 1 healthy - can't maintain quorum
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1"},
						{Name: "cp-2", Healthy: false, UnhealthySince: &unhealthySince},
						{Name: "cp-3", Healthy: false, UnhealthySince: &unhealthySince},
					},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		result, err := r.reconcileControlPlanes(context.Background(), cluster)

		assert.NoError(t, err)
		assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)

		// Verify no server was deleted
		assert.Len(t, mockHCloud.DeleteServerCalls, 0)

		// Verify condition was set
		found := false
		for _, cond := range cluster.Status.Conditions {
			if cond.Type == k8znerv1alpha1.ConditionControlPlaneReady && cond.Reason == "QuorumLost" {
				found = true
				break
			}
		}
		assert.True(t, found, "QuorumLost condition should be set")
	})
}

func TestClusterReconciler_reconcileWorkers(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("replaces unhealthy workers", func(t *testing.T) {
		t.Parallel()
		unhealthySince := metav1.NewTime(time.Now().Add(-10 * time.Minute))
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 2},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Desired: 2,
					Ready:   1,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Healthy: true},
						{Name: "worker-2", Healthy: false, ServerID: 12345, UnhealthySince: &unhealthySince, UnhealthyReason: "NodeNotReady"},
					},
				},
			},
		}

		workerNode := createTestNode("worker-2", false, false)

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, workerNode).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		result, err := r.reconcileWorkers(context.Background(), cluster)

		assert.NoError(t, err)
		assert.True(t, result.RequeueAfter > 0)

		// Verify server was deleted
		assert.Contains(t, mockHCloud.DeleteServerCalls, "worker-2")
	})

	t.Run("respects maxConcurrentHeals limit", func(t *testing.T) {
		t.Parallel()
		unhealthySince := metav1.NewTime(time.Now().Add(-10 * time.Minute))
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 3},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Desired: 3,
					Ready:   1,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Healthy: true},
						{Name: "worker-2", Healthy: false, ServerID: 12345, UnhealthySince: &unhealthySince},
						{Name: "worker-3", Healthy: false, ServerID: 12346, UnhealthySince: &unhealthySince},
					},
				},
			},
		}

		worker2 := createTestNode("worker-2", false, false)
		worker3 := createTestNode("worker-3", false, false)

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, worker2, worker3).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMaxConcurrentHeals(1), // Only 1 at a time
			WithMetrics(false),
		)

		_, err := r.reconcileWorkers(context.Background(), cluster)
		require.NoError(t, err)

		// Only 1 server should be deleted due to maxConcurrentHeals
		assert.Len(t, mockHCloud.DeleteServerCalls, 1)
	})

	t.Run("skips scaling when workers are provisioning", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 2},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Desired: 2,
					Ready:   0,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Phase: k8znerv1alpha1.NodePhaseCreatingServer, Healthy: false},
					},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		result, err := r.reconcileWorkers(context.Background(), cluster)

		assert.NoError(t, err)
		assert.True(t, result.RequeueAfter > 0)
		assert.Len(t, mockHCloud.CreateServerCalls, 0)
	})

	t.Run("scales up when no workers are provisioning", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 2, Size: "cx21"},
				Region:  "fsn1",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Desired: 2,
					Ready:   1,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Phase: k8znerv1alpha1.NodePhaseReady, Healthy: true},
					},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		result, err := r.reconcileWorkers(context.Background(), cluster)

		assert.NoError(t, err)
		assert.True(t, result.RequeueAfter > 0)
		assert.Equal(t, k8znerv1alpha1.ClusterPhaseHealing, cluster.Status.Phase)
	})
}

func TestProvisioningDetectionHelpers(t *testing.T) {
	t.Parallel()
	t.Run("isNodeInEarlyProvisioningPhase", func(t *testing.T) {
		t.Parallel()
		earlyPhases := []k8znerv1alpha1.NodePhase{
			k8znerv1alpha1.NodePhaseCreatingServer,
			k8znerv1alpha1.NodePhaseWaitingForIP,
			k8znerv1alpha1.NodePhaseWaitingForTalosAPI,
			k8znerv1alpha1.NodePhaseApplyingTalosConfig,
			k8znerv1alpha1.NodePhaseRebootingWithConfig,
			k8znerv1alpha1.NodePhaseWaitingForK8s,
			k8znerv1alpha1.NodePhaseNodeInitializing,
		}
		for _, phase := range earlyPhases {
			assert.True(t, isNodeInEarlyProvisioningPhase(phase), "expected %s to be early provisioning phase", phase)
		}

		nonProvisioningPhases := []k8znerv1alpha1.NodePhase{
			k8znerv1alpha1.NodePhaseReady,
			k8znerv1alpha1.NodePhaseUnhealthy,
			k8znerv1alpha1.NodePhaseFailed,
			k8znerv1alpha1.NodePhaseDraining,
			k8znerv1alpha1.NodePhaseRemovingFromEtcd,
			k8znerv1alpha1.NodePhaseDeletingServer,
		}
		for _, phase := range nonProvisioningPhases {
			assert.False(t, isNodeInEarlyProvisioningPhase(phase), "expected %s to NOT be early provisioning phase", phase)
		}

		assert.False(t, isNodeInEarlyProvisioningPhase(""))
	})

	t.Run("countWorkersInEarlyProvisioning", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 0, countWorkersInEarlyProvisioning(nil))
		assert.Equal(t, 0, countWorkersInEarlyProvisioning([]k8znerv1alpha1.NodeStatus{}))

		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "worker-1", Phase: k8znerv1alpha1.NodePhaseReady},
			{Name: "worker-2", Phase: k8znerv1alpha1.NodePhaseReady},
		}
		assert.Equal(t, 0, countWorkersInEarlyProvisioning(nodes))

		nodes = []k8znerv1alpha1.NodeStatus{
			{Name: "worker-1", Phase: k8znerv1alpha1.NodePhaseReady},
			{Name: "worker-2", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
			{Name: "worker-3", Phase: k8znerv1alpha1.NodePhaseWaitingForIP},
		}
		assert.Equal(t, 2, countWorkersInEarlyProvisioning(nodes))

		nodes = []k8znerv1alpha1.NodeStatus{
			{Name: "worker-1", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
			{Name: "worker-2", Phase: k8znerv1alpha1.NodePhaseWaitingForTalosAPI},
			{Name: "worker-3", Phase: k8znerv1alpha1.NodePhaseApplyingTalosConfig},
		}
		assert.Equal(t, 3, countWorkersInEarlyProvisioning(nodes))

		nodes = []k8znerv1alpha1.NodeStatus{
			{Name: "worker-1", Phase: k8znerv1alpha1.NodePhaseReady},
			{Name: "worker-2", Phase: k8znerv1alpha1.NodePhaseUnhealthy},
			{Name: "worker-3", Phase: k8znerv1alpha1.NodePhaseFailed},
			{Name: "worker-4", Phase: k8znerv1alpha1.NodePhaseNodeInitializing},
		}
		assert.Equal(t, 1, countWorkersInEarlyProvisioning(nodes))
	})
}

func TestScaleUpWorkers(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("creates new workers successfully", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
				Workers: k8znerv1alpha1.WorkerSpec{
					Count: 3,
					Size:  "cx22",
				},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Desired: 3,
					Ready:   1,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "test-cluster-workers-1", Healthy: true},
					},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(20)

		mockHCloud := &MockHCloudClient{
			GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
				return &hcloudgo.Network{ID: 123}, nil
			},
			GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				return &hcloudgo.Image{ID: 456}, nil
			},
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				return "10.0.0.12", nil
			},
			GetServerIDFunc: func(ctx context.Context, name string) (string, error) {
				return "12345", nil
			},
		}

		mockTalos := &MockTalosClient{}
		mockTalosGen := &MockTalosConfigGenerator{}

		var nodeReadyWaiterCalls []string
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithTalosClient(mockTalos),
			WithTalosConfigGenerator(mockTalosGen),
			WithMaxConcurrentHeals(5),
			WithMetrics(false),
			WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
				nodeReadyWaiterCalls = append(nodeReadyWaiterCalls, nodeName)
				return nil
			}),
		)

		err := r.scaleUpWorkers(context.Background(), cluster, 2)
		require.NoError(t, err)

		assert.Len(t, mockHCloud.CreateServerCalls, 2)

		assert.True(t, strings.HasPrefix(mockHCloud.CreateServerCalls[0].Name, "test-cluster-w-"),
			"expected name to start with test-cluster-w-, got %s", mockHCloud.CreateServerCalls[0].Name)
		assert.Equal(t, "cx23", mockHCloud.CreateServerCalls[0].ServerType)
		assert.Equal(t, "nbg1", mockHCloud.CreateServerCalls[0].Location)
		assert.Equal(t, "worker", mockHCloud.CreateServerCalls[0].Labels["role"])

		assert.True(t, strings.HasPrefix(mockHCloud.CreateServerCalls[1].Name, "test-cluster-w-"),
			"expected name to start with test-cluster-w-, got %s", mockHCloud.CreateServerCalls[1].Name)
		assert.NotEqual(t, mockHCloud.CreateServerCalls[0].Name, mockHCloud.CreateServerCalls[1].Name)

		assert.Len(t, mockTalosGen.GenerateWorkerConfigCalls, 2)
		assert.Len(t, mockTalos.ApplyConfigCalls, 2)
		assert.Len(t, nodeReadyWaiterCalls, 2, "node ready waiter should be called for each worker")
	})

	t.Run("respects maxConcurrentHeals limit", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
				Workers: k8znerv1alpha1.WorkerSpec{
					Count: 5,
					Size:  "cx22",
				},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Desired: 5,
					Ready:   0,
					Nodes:   []k8znerv1alpha1.NodeStatus{},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(20)

		mockHCloud := &MockHCloudClient{
			GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
				return &hcloudgo.Network{ID: 123}, nil
			},
			GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				return &hcloudgo.Image{ID: 456}, nil
			},
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				return "10.0.0.12", nil
			},
			GetServerIDFunc: func(ctx context.Context, name string) (string, error) {
				return "12345", nil
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		err := r.scaleUpWorkers(context.Background(), cluster, 5)
		// All 5 servers are created in parallel (no maxConcurrentHeals limit for scale-up)
		require.NoError(t, err)

		assert.Len(t, mockHCloud.CreateServerCalls, 5)
	})

	t.Run("handles missing snapshot", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
				Workers: k8znerv1alpha1.WorkerSpec{
					Count: 2,
					Size:  "cx22",
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(20)

		mockHCloud := &MockHCloudClient{
			GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
				return &hcloudgo.Network{ID: 123}, nil
			},
			GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				return nil, nil
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		err := r.scaleUpWorkers(context.Background(), cluster, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no Talos snapshot found")

		assert.Len(t, mockHCloud.CreateServerCalls, 0)
	})

	t.Run("works without talos clients", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
				Workers: k8znerv1alpha1.WorkerSpec{
					Count: 2,
					Size:  "cx22",
				},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Desired: 2,
					Ready:   0,
					Nodes:   []k8znerv1alpha1.NodeStatus{},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(20)

		mockHCloud := &MockHCloudClient{
			GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
				return &hcloudgo.Network{ID: 123}, nil
			},
			GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				return &hcloudgo.Image{ID: 456}, nil
			},
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				return "10.0.0.12", nil
			},
			GetServerIDFunc: func(ctx context.Context, name string) (string, error) {
				return "12345", nil
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMaxConcurrentHeals(5),
			WithMetrics(false),
		)

		err := r.scaleUpWorkers(context.Background(), cluster, 2)
		require.NoError(t, err)

		assert.Len(t, mockHCloud.CreateServerCalls, 2)
	})
}

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
