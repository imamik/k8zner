package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/config"
)

func TestFindUnhealthyNodes(t *testing.T) {
	t.Parallel()
	threshold := 5 * time.Minute

	t.Run("returns nil for empty nodes", func(t *testing.T) {
		t.Parallel()
		result := findUnhealthyNodes(nil, threshold)
		assert.Nil(t, result)

		result = findUnhealthyNodes([]k8znerv1alpha1.NodeStatus{}, threshold)
		assert.Nil(t, result)
	})

	t.Run("returns nil when all nodes healthy", func(t *testing.T) {
		t.Parallel()
		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "worker-1", Healthy: true},
			{Name: "worker-2", Healthy: true},
		}
		result := findUnhealthyNodes(nodes, threshold)
		assert.Nil(t, result)
	})

	t.Run("skips unhealthy nodes without UnhealthySince", func(t *testing.T) {
		t.Parallel()
		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "worker-1", Healthy: false}, // No UnhealthySince
		}
		result := findUnhealthyNodes(nodes, threshold)
		assert.Nil(t, result)
	})

	t.Run("skips nodes not past threshold", func(t *testing.T) {
		t.Parallel()
		recentTime := metav1.NewTime(time.Now().Add(-1 * time.Minute)) // Only 1 min ago
		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "worker-1", Healthy: false, UnhealthySince: &recentTime},
		}
		result := findUnhealthyNodes(nodes, threshold)
		assert.Nil(t, result)
	})

	t.Run("returns nodes past threshold", func(t *testing.T) {
		t.Parallel()
		pastThreshold := metav1.NewTime(time.Now().Add(-10 * time.Minute))
		recentTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "worker-1", Healthy: true},
			{Name: "worker-2", Healthy: false, UnhealthySince: &pastThreshold},
			{Name: "worker-3", Healthy: false, UnhealthySince: &recentTime},
			{Name: "worker-4", Healthy: false, UnhealthySince: &pastThreshold},
		}
		result := findUnhealthyNodes(nodes, threshold)
		require.Len(t, result, 2)
		assert.Equal(t, "worker-2", result[0].Name)
		assert.Equal(t, "worker-4", result[1].Name)
	})

	t.Run("respects zero threshold", func(t *testing.T) {
		t.Parallel()
		recentTime := metav1.NewTime(time.Now().Add(-1 * time.Second))
		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "worker-1", Healthy: false, UnhealthySince: &recentTime},
		}
		result := findUnhealthyNodes(nodes, 0)
		require.Len(t, result, 1)
		assert.Equal(t, "worker-1", result[0].Name)
	})
}

func TestFindUnhealthyNode(t *testing.T) {
	t.Parallel()
	threshold := 5 * time.Minute

	t.Run("returns nil for empty nodes", func(t *testing.T) {
		t.Parallel()
		result := findUnhealthyNode(nil, threshold)
		assert.Nil(t, result)

		result = findUnhealthyNode([]k8znerv1alpha1.NodeStatus{}, threshold)
		assert.Nil(t, result)
	})

	t.Run("returns nil when all healthy", func(t *testing.T) {
		t.Parallel()
		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "cp-1", Healthy: true},
			{Name: "cp-2", Healthy: true},
			{Name: "cp-3", Healthy: true},
		}
		result := findUnhealthyNode(nodes, threshold)
		assert.Nil(t, result)
	})

	t.Run("returns nil when unhealthy but not past threshold", func(t *testing.T) {
		t.Parallel()
		recentTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "cp-1", Healthy: true},
			{Name: "cp-2", Healthy: false, UnhealthySince: &recentTime},
		}
		result := findUnhealthyNode(nodes, threshold)
		assert.Nil(t, result)
	})

	t.Run("returns first node past threshold", func(t *testing.T) {
		t.Parallel()
		pastThreshold := metav1.NewTime(time.Now().Add(-10 * time.Minute))
		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "cp-1", Healthy: true},
			{Name: "cp-2", Healthy: false, UnhealthySince: &pastThreshold},
			{Name: "cp-3", Healthy: false, UnhealthySince: &pastThreshold},
		}
		result := findUnhealthyNode(nodes, threshold)
		require.NotNil(t, result)
		assert.Equal(t, "cp-2", result.Name) // Returns first match
	})

	t.Run("skips unhealthy without UnhealthySince", func(t *testing.T) {
		t.Parallel()
		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "cp-1", Healthy: false}, // No timestamp
		}
		result := findUnhealthyNode(nodes, threshold)
		assert.Nil(t, result)
	})
}

func TestSelectWorkersForRemoval(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("returns nil for zero count", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Healthy: true},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		result := r.selectWorkersForRemoval(cluster, 0)
		assert.Nil(t, result)
	})

	t.Run("returns nil for empty nodes", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		result := r.selectWorkersForRemoval(cluster, 1)
		assert.Nil(t, result)
	})

	t.Run("prioritizes unhealthy workers", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Healthy: true},
						{Name: "worker-2", Healthy: false},
						{Name: "worker-3", Healthy: true},
						{Name: "worker-4", Healthy: false},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		result := r.selectWorkersForRemoval(cluster, 2)
		require.Len(t, result, 2)
		assert.Equal(t, "worker-2", result[0].Name)
		assert.Equal(t, "worker-4", result[1].Name)
	})

	t.Run("selects newest healthy when no unhealthy", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Healthy: true},
						{Name: "worker-2", Healthy: true},
						{Name: "worker-3", Healthy: true},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		result := r.selectWorkersForRemoval(cluster, 1)
		require.Len(t, result, 1)
		assert.Equal(t, "worker-3", result[0].Name) // Last in list = "newest"
	})

	t.Run("mixed unhealthy and healthy selection", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Healthy: true},
						{Name: "worker-2", Healthy: false},
						{Name: "worker-3", Healthy: true},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		// Requesting 2: should get 1 unhealthy + 1 newest healthy
		result := r.selectWorkersForRemoval(cluster, 2)
		require.Len(t, result, 2)
		assert.Equal(t, "worker-2", result[0].Name) // Unhealthy first
		assert.Equal(t, "worker-3", result[1].Name) // Then newest healthy
	})

	t.Run("count exceeds available nodes", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Healthy: true},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		result := r.selectWorkersForRemoval(cluster, 5)
		require.Len(t, result, 1)
		assert.Equal(t, "worker-1", result[0].Name)
	})
}

func TestRemoveWorkersFromStatus(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("no-op for empty removal list", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1"},
						{Name: "worker-2"},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		r.removeWorkersFromStatus(cluster, nil)
		assert.Len(t, cluster.Status.Workers.Nodes, 2)

		r.removeWorkersFromStatus(cluster, []*k8znerv1alpha1.NodeStatus{})
		assert.Len(t, cluster.Status.Workers.Nodes, 2)
	})

	t.Run("removes specified workers", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1"},
						{Name: "worker-2"},
						{Name: "worker-3"},
					},
				},
			},
		}

		toRemove := []*k8znerv1alpha1.NodeStatus{
			{Name: "worker-2"},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		r.removeWorkersFromStatus(cluster, toRemove)
		require.Len(t, cluster.Status.Workers.Nodes, 2)
		assert.Equal(t, "worker-1", cluster.Status.Workers.Nodes[0].Name)
		assert.Equal(t, "worker-3", cluster.Status.Workers.Nodes[1].Name)
	})

	t.Run("removes multiple workers", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1"},
						{Name: "worker-2"},
						{Name: "worker-3"},
						{Name: "worker-4"},
					},
				},
			},
		}

		toRemove := []*k8znerv1alpha1.NodeStatus{
			{Name: "worker-1"},
			{Name: "worker-3"},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		r.removeWorkersFromStatus(cluster, toRemove)
		require.Len(t, cluster.Status.Workers.Nodes, 2)
		assert.Equal(t, "worker-2", cluster.Status.Workers.Nodes[0].Name)
		assert.Equal(t, "worker-4", cluster.Status.Workers.Nodes[1].Name)
	})

	t.Run("removes all workers", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1"},
						{Name: "worker-2"},
					},
				},
			},
		}

		toRemove := []*k8znerv1alpha1.NodeStatus{
			{Name: "worker-1"},
			{Name: "worker-2"},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		r.removeWorkersFromStatus(cluster, toRemove)
		assert.Empty(t, cluster.Status.Workers.Nodes)
	})
}

func TestDecommissionWorker(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("cordons drains and deletes node and server", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}

		workerNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
			},
		}

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

		worker := &k8znerv1alpha1.NodeStatus{
			Name:     "worker-1",
			ServerID: 12345,
		}

		err := r.decommissionWorker(context.Background(), cluster, worker)
		require.NoError(t, err)

		// Verify server was deleted
		assert.Contains(t, mockHCloud.DeleteServerCalls, "worker-1")

		// Verify event was recorded
		select {
		case event := <-recorder.Events:
			assert.Contains(t, event, "Successfully removed worker worker-1")
		default:
			t.Error("expected scaling down event")
		}
	})

	t.Run("handles missing k8s node gracefully", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}

		// No K8s node object exists
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

		worker := &k8znerv1alpha1.NodeStatus{
			Name:     "worker-missing",
			ServerID: 12345,
		}

		err := r.decommissionWorker(context.Background(), cluster, worker)
		require.NoError(t, err)

		// Server should still be deleted even if K8s node doesn't exist
		assert.Contains(t, mockHCloud.DeleteServerCalls, "worker-missing")
	})

	t.Run("returns error on server deletion failure", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{
			DeleteServerFunc: func(ctx context.Context, name string) error {
				return assert.AnError
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		worker := &k8znerv1alpha1.NodeStatus{
			Name:     "worker-fail",
			ServerID: 12345,
		}

		err := r.decommissionWorker(context.Background(), cluster, worker)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete hetzner server")
	})
}

func TestScaleDownWorkers(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("no-op when no workers to remove", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{},
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

		err := r.scaleDownWorkers(context.Background(), cluster, 1)
		require.NoError(t, err)
		assert.Empty(t, mockHCloud.DeleteServerCalls)
	})

	t.Run("removes unhealthy workers first", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Healthy: true},
						{Name: "worker-2", Healthy: false},
						{Name: "worker-3", Healthy: true},
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

		err := r.scaleDownWorkers(context.Background(), cluster, 1)
		require.NoError(t, err)

		// Should have removed the unhealthy worker
		assert.Contains(t, mockHCloud.DeleteServerCalls, "worker-2")
		assert.Len(t, mockHCloud.DeleteServerCalls, 1)

		// Status should be updated
		assert.Len(t, cluster.Status.Workers.Nodes, 2)
	})

	t.Run("respects maxConcurrentHeals during scale-down", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Healthy: true},
						{Name: "worker-2", Healthy: true},
						{Name: "worker-3", Healthy: true},
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
			WithMaxConcurrentHeals(1),
			WithMetrics(false),
		)

		err := r.scaleDownWorkers(context.Background(), cluster, 3)
		// Should return partial error since only 1 out of 3 could be removed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only removed 1 of 3")
		assert.Len(t, mockHCloud.DeleteServerCalls, 1)
	})
}

func TestReconcileControlPlanes_ProvisioningSkip(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("skips scaling when CPs are provisioning", func(t *testing.T) {
		t.Parallel()
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
					Ready:   1,
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, Phase: k8znerv1alpha1.NodePhaseReady},
						{Name: "cp-2", Phase: k8znerv1alpha1.NodePhaseCreatingServer}, // Provisioning
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
		assert.Equal(t, 30*time.Second, result.RequeueAfter)
		// Should NOT have attempted any server creation
		assert.Empty(t, mockHCloud.CreateServerCalls)
	})
}

func TestHandleCPScaleUp(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("records event and requeues", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		// No hcloud client - should still succeed (just skips actual provisioning)
		r := NewClusterReconciler(client, scheme, recorder,
			WithMetrics(false),
		)

		result, err := r.handleCPScaleUp(context.Background(), cluster, 1, 3)

		assert.NoError(t, err)
		assert.Equal(t, 10*time.Second, result.RequeueAfter)

		// Verify event was recorded
		select {
		case event := <-recorder.Events:
			assert.Contains(t, event, "Scaling up control planes: 1 -> 3")
		default:
			t.Error("expected scaling event")
		}
	})

	t.Run("sets healing phase with hcloud client", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
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

		_, err := r.handleCPScaleUp(context.Background(), cluster, 1, 3)

		assert.NoError(t, err)
		assert.Equal(t, k8znerv1alpha1.ClusterPhaseHealing, cluster.Status.Phase)
	})
}

func TestNormalizeServerSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"cx22 normalizes to cx23", "cx22", "cx23"},
		{"cx21 passes through", "cx21", "cx21"},
		{"cx32 normalizes to cx33", "cx32", "cx33"},
		{"empty stays empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := string(config.ServerSize(tt.input).Normalize())
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReplaceUnhealthyWorkers(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("replaces up to maxConcurrentHeals", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}

		unhealthySince := metav1.NewTime(time.Now().Add(-10 * time.Minute))
		worker1 := createTestNode("worker-1", false, false)
		worker2 := createTestNode("worker-2", false, false)

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, worker1, worker2).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(20)

		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMaxConcurrentHeals(1),
			WithMetrics(false),
		)

		unhealthyWorkers := []*k8znerv1alpha1.NodeStatus{
			{Name: "worker-1", ServerID: 111, UnhealthySince: &unhealthySince},
			{Name: "worker-2", ServerID: 222, UnhealthySince: &unhealthySince},
		}

		replaced := r.replaceUnhealthyWorkers(context.Background(), cluster, unhealthyWorkers)

		assert.Equal(t, 1, replaced) // Limited by maxConcurrentHeals
		assert.Equal(t, k8znerv1alpha1.ClusterPhaseHealing, cluster.Status.Phase)
	})

	t.Run("returns zero for empty list", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(client, scheme, recorder,
			WithMetrics(false),
		)

		replaced := r.replaceUnhealthyWorkers(context.Background(), cluster, nil)
		assert.Equal(t, 0, replaced)
	})
}

func TestScaleWorkers(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("no-op when at desired count", func(t *testing.T) {
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
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Phase: k8znerv1alpha1.NodePhaseReady, Healthy: true},
						{Name: "worker-2", Phase: k8znerv1alpha1.NodePhaseReady, Healthy: true},
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

		r := NewClusterReconciler(client, scheme, recorder,
			WithMetrics(false),
		)

		result, err := r.scaleWorkers(context.Background(), cluster)
		assert.NoError(t, err)
		assert.Zero(t, result.RequeueAfter)
		assert.False(t, result.Requeue)
	})

	t.Run("triggers scale-down when over desired", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Workers: k8znerv1alpha1.WorkerSpec{Count: 1},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "worker-1", Phase: k8znerv1alpha1.NodePhaseReady, Healthy: true},
						{Name: "worker-2", Phase: k8znerv1alpha1.NodePhaseReady, Healthy: true},
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

		result, err := r.scaleWorkers(context.Background(), cluster)
		assert.NoError(t, err)
		assert.Equal(t, 30*time.Second, result.RequeueAfter)
		assert.Equal(t, k8znerv1alpha1.ClusterPhaseHealing, cluster.Status.Phase)

		// Verify a scale-down event was recorded
		select {
		case event := <-recorder.Events:
			assert.Contains(t, event, "Scaling down workers: 2 -> 1")
		default:
			t.Error("expected scaling down event")
		}
	})
}
