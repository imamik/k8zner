package controller

import (
	"context"
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
