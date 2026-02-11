package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/platform/hcloud"
)

func TestSelfHealingControlPlaneReplacement(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("successful control plane replacement with full cycle", func(t *testing.T) {
		t.Parallel()
		unhealthySince := metav1.NewTime(time.Now().Add(-10 * time.Minute))
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
					Count: 3,
					Size:  "cx22",
				},
				Workers: k8znerv1alpha1.WorkerSpec{
					Count: 2,
					Size:  "cx22",
				},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Desired: 3,
					Ready:   2,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "test-cluster-control-plane-1", Healthy: true, PrivateIP: "10.0.0.1", PublicIP: "1.2.3.1"},
						{Name: "test-cluster-control-plane-2", Healthy: true, PrivateIP: "10.0.0.2", PublicIP: "1.2.3.2"},
						{Name: "test-cluster-control-plane-3", Healthy: false, PrivateIP: "10.0.0.3", PublicIP: "1.2.3.3", ServerID: 12345, UnhealthySince: &unhealthySince, UnhealthyReason: "NodeNotReady"},
					},
				},
			},
		}

		cpNode3 := createTestNode("test-cluster-control-plane-3", true, false)

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, cpNode3).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(20)

		mockHCloud := &MockHCloudClient{
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				return "10.0.0.4", nil
			},
		}
		mockTalos := &MockTalosClient{}
		mockTalosGen := &MockTalosConfigGenerator{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithTalosClient(mockTalos),
			WithTalosConfigGenerator(mockTalosGen),
			WithMetrics(false),
		)

		node := &k8znerv1alpha1.NodeStatus{
			Name:            "test-cluster-control-plane-3",
			PrivateIP:       "10.0.0.3",
			ServerID:        12345,
			UnhealthyReason: "NodeNotReady",
		}
		err := r.replaceControlPlane(context.Background(), cluster, node)
		require.NoError(t, err)

		assert.Contains(t, mockHCloud.DeleteServerCalls, "test-cluster-control-plane-3")

		assert.Len(t, mockHCloud.CreateServerCalls, 1)
		createCall := mockHCloud.CreateServerCalls[0]
		assert.Equal(t, "cx23", createCall.ServerType)
		assert.Equal(t, "nbg1", createCall.Location)
		assert.Equal(t, "control-plane", createCall.Labels["role"])

		assert.Len(t, mockTalos.GetEtcdMembersCalls, 1)

		assert.Len(t, mockHCloud.GetNetworkCalls, 1)
		assert.Equal(t, "test-cluster-network", mockHCloud.GetNetworkCalls[0])

		assert.Len(t, mockHCloud.GetSnapshotByLabelsCalls, 1)

		assert.Len(t, mockTalosGen.GenerateControlPlaneConfigCalls, 1)
		assert.Len(t, mockTalos.ApplyConfigCalls, 1)
		assert.Equal(t, "10.0.0.4", mockTalos.ApplyConfigCalls[0].NodeIP)

		assert.Len(t, mockTalos.WaitForNodeReadyCalls, 1)
		assert.Equal(t, "10.0.0.4", mockTalos.WaitForNodeReadyCalls[0].NodeIP)
	})

	t.Run("control plane replacement fails on server creation", func(t *testing.T) {
		t.Parallel()
		unhealthySince := metav1.NewTime(time.Now().Add(-10 * time.Minute))
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
					Count: 3,
					Size:  "cx22",
				},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Desired: 3,
					Ready:   2,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1"},
						{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
						{Name: "cp-3", Healthy: false, PrivateIP: "10.0.0.3", ServerID: 12345, UnhealthySince: &unhealthySince},
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
		recorder := record.NewFakeRecorder(20)

		mockHCloud := &MockHCloudClient{
			CreateServerFunc: func(ctx context.Context, opts hcloud.ServerCreateOpts) (string, error) {
				return "", assert.AnError
			},
		}
		mockTalos := &MockTalosClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithTalosClient(mockTalos),
			WithMetrics(false),
		)

		node := &k8znerv1alpha1.NodeStatus{
			Name:      "cp-3",
			PrivateIP: "10.0.0.3",
			ServerID:  12345,
		}
		err := r.replaceControlPlane(context.Background(), cluster, node)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create server")
	})

	t.Run("control plane replacement fails on config apply", func(t *testing.T) {
		t.Parallel()
		unhealthySince := metav1.NewTime(time.Now().Add(-10 * time.Minute))
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
					Count: 3,
					Size:  "cx22",
				},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Desired: 3,
					Ready:   2,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1"},
						{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
						{Name: "cp-3", Healthy: false, PrivateIP: "10.0.0.3", ServerID: 12345, UnhealthySince: &unhealthySince},
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
		recorder := record.NewFakeRecorder(20)

		mockHCloud := &MockHCloudClient{
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				return "10.0.0.4", nil
			},
		}
		mockTalos := &MockTalosClient{
			ApplyConfigFunc: func(ctx context.Context, nodeIP string, config []byte) error {
				return assert.AnError
			},
		}
		mockTalosGen := &MockTalosConfigGenerator{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithTalosClient(mockTalos),
			WithTalosConfigGenerator(mockTalosGen),
			WithMetrics(false),
		)

		node := &k8znerv1alpha1.NodeStatus{
			Name:      "cp-3",
			PrivateIP: "10.0.0.3",
			ServerID:  12345,
		}
		err := r.replaceControlPlane(context.Background(), cluster, node)
		assert.Error(t, err)

		assert.Len(t, mockHCloud.CreateServerCalls, 1)
	})
}

func TestSelfHealingWorkerReplacement(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("successful worker replacement with full cycle", func(t *testing.T) {
		t.Parallel()
		unhealthySince := metav1.NewTime(time.Now().Add(-10 * time.Minute))
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
					Count: 3,
					Size:  "cx22",
				},
				Workers: k8znerv1alpha1.WorkerSpec{
					Count: 2,
					Size:  "cx32",
				},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Desired: 3,
					Ready:   3,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1"},
						{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
						{Name: "cp-3", Healthy: true, PrivateIP: "10.0.0.3"},
					},
				},
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Desired: 2,
					Ready:   1,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "test-cluster-workers-1", Healthy: true, PrivateIP: "10.0.0.10"},
						{Name: "test-cluster-workers-2", Healthy: false, PrivateIP: "10.0.0.11", ServerID: 12346, UnhealthySince: &unhealthySince, UnhealthyReason: "NodeNotReady"},
					},
				},
			},
		}

		workerNode := createTestNode("test-cluster-workers-2", false, false)

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, workerNode).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(20)

		mockHCloud := &MockHCloudClient{
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				return "10.0.0.12", nil
			},
		}
		mockTalos := &MockTalosClient{}
		mockTalosGen := &MockTalosConfigGenerator{}

		var nodeReadyWaiterCalls []string
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithTalosClient(mockTalos),
			WithTalosConfigGenerator(mockTalosGen),
			WithMetrics(false),
			WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
				nodeReadyWaiterCalls = append(nodeReadyWaiterCalls, nodeName)
				return nil
			}),
		)

		node := &k8znerv1alpha1.NodeStatus{
			Name:            "test-cluster-workers-2",
			PrivateIP:       "10.0.0.11",
			ServerID:        12346,
			UnhealthyReason: "NodeNotReady",
		}
		err := r.replaceWorker(context.Background(), cluster, node)
		require.NoError(t, err)

		assert.Contains(t, mockHCloud.DeleteServerCalls, "test-cluster-workers-2")

		assert.Len(t, mockHCloud.CreateServerCalls, 1)
		createCall := mockHCloud.CreateServerCalls[0]
		assert.Equal(t, "cx33", createCall.ServerType)
		assert.Equal(t, "nbg1", createCall.Location)
		assert.Equal(t, "worker", createCall.Labels["role"])
		assert.Equal(t, "workers", createCall.Labels["pool"])

		assert.Len(t, mockTalosGen.GenerateWorkerConfigCalls, 1)
		assert.Len(t, mockTalosGen.GenerateControlPlaneConfigCalls, 0)

		assert.Len(t, mockTalos.ApplyConfigCalls, 1)
		assert.Equal(t, "10.0.0.12", mockTalos.ApplyConfigCalls[0].NodeIP)

		assert.Len(t, nodeReadyWaiterCalls, 1, "nodeReadyWaiter should be called for worker")
	})

	t.Run("worker replacement continues without talos clients", func(t *testing.T) {
		t.Parallel()
		unhealthySince := metav1.NewTime(time.Now().Add(-10 * time.Minute))
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
					Ready:   1,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Healthy: true},
						{Name: "worker-2", Healthy: false, ServerID: 12346, UnhealthySince: &unhealthySince},
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
		recorder := record.NewFakeRecorder(20)

		mockHCloud := &MockHCloudClient{
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				return "10.0.0.12", nil
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		node := &k8znerv1alpha1.NodeStatus{
			Name:     "worker-2",
			ServerID: 12346,
		}
		err := r.replaceWorker(context.Background(), cluster, node)
		require.NoError(t, err)

		assert.Len(t, mockHCloud.CreateServerCalls, 1)
	})
}
