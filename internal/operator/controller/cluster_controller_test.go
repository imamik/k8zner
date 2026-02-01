package controller

import (
	"context"
	"testing"
	"time"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, k8znerv1alpha1.AddToScheme(scheme))
	return scheme
}

func TestNewClusterReconciler(t *testing.T) {
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	t.Run("with default options", func(t *testing.T) {
		r := NewClusterReconciler(client, scheme, recorder)

		assert.NotNil(t, r)
		assert.Equal(t, client, r.Client)
		assert.Equal(t, scheme, r.Scheme)
		assert.Equal(t, recorder, r.Recorder)
		assert.True(t, r.enableMetrics)
		assert.Equal(t, 1, r.maxConcurrentHeals)
	})

	t.Run("with custom options", func(t *testing.T) {
		mockHCloud := &MockHCloudClient{}
		mockTalos := &MockTalosClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithTalosClient(mockTalos),
			WithMetrics(false),
			WithMaxConcurrentHeals(3),
		)

		assert.NotNil(t, r)
		assert.Equal(t, mockHCloud, r.hcloudClient)
		assert.Equal(t, mockTalos, r.talosClient)
		assert.False(t, r.enableMetrics)
		assert.Equal(t, 3, r.maxConcurrentHeals)
	})

	t.Run("with HCloud token for lazy initialization", func(t *testing.T) {
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudToken("test-token"),
		)

		assert.NotNil(t, r)
		assert.Equal(t, "test-token", r.hcloudToken)
		assert.Nil(t, r.hcloudClient) // Should be nil until ensureHCloudClient is called
	})
}

func TestClusterReconciler_Reconcile(t *testing.T) {
	scheme := setupTestScheme(t)

	t.Run("cluster not found returns no error", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(&MockHCloudClient{}),
		)

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      "nonexistent",
			},
		})

		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, result)
	})

	t.Run("paused cluster skips reconciliation", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Paused: true,
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

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      "test-cluster",
			},
		})

		assert.NoError(t, err)
		assert.Equal(t, defaultRequeueAfter, result.RequeueAfter)
	})

	t.Run("reconciles healthy cluster", func(t *testing.T) {
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
		}

		// Create healthy control plane nodes
		cpNode1 := createTestNode("cp-1", true, true)
		cpNode2 := createTestNode("cp-2", true, true)
		cpNode3 := createTestNode("cp-3", true, true)

		// Create healthy worker nodes
		worker1 := createTestNode("worker-1", false, true)
		worker2 := createTestNode("worker-2", false, true)

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, cpNode1, cpNode2, cpNode3, worker1, worker2).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
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

		// Verify cluster status was updated
		updatedCluster := &k8znerv1alpha1.K8znerCluster{}
		err = client.Get(context.Background(), types.NamespacedName{
			Namespace: "default",
			Name:      "test-cluster",
		}, updatedCluster)
		require.NoError(t, err)

		assert.Equal(t, 3, updatedCluster.Status.ControlPlanes.Ready)
		assert.Equal(t, 2, updatedCluster.Status.Workers.Ready)
		assert.Equal(t, k8znerv1alpha1.ClusterPhaseRunning, updatedCluster.Status.Phase)
	})
}

func TestClusterReconciler_reconcileHealthCheck(t *testing.T) {
	scheme := setupTestScheme(t)

	t.Run("correctly categorizes control plane and worker nodes", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3},
				Workers:       k8znerv1alpha1.WorkerSpec{Count: 2},
			},
		}

		cpNode := createTestNode("cp-1", true, true)
		workerNode := createTestNode("worker-1", false, true)

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, cpNode, workerNode).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(&MockHCloudClient{}),
			WithMetrics(false),
		)

		err := r.reconcileHealthCheck(context.Background(), cluster)
		require.NoError(t, err)

		assert.Equal(t, 1, cluster.Status.ControlPlanes.Ready)
		assert.Equal(t, 1, cluster.Status.Workers.Ready)
	})

	t.Run("detects unhealthy nodes", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3},
				Workers:       k8znerv1alpha1.WorkerSpec{Count: 2},
			},
		}

		healthyNode := createTestNode("cp-1", true, true)
		unhealthyNode := createTestNode("cp-2", true, false)

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, healthyNode, unhealthyNode).
			WithStatusSubresource(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(&MockHCloudClient{}),
			WithMetrics(false),
		)

		err := r.reconcileHealthCheck(context.Background(), cluster)
		require.NoError(t, err)

		assert.Equal(t, 1, cluster.Status.ControlPlanes.Ready)
		assert.Equal(t, 1, cluster.Status.ControlPlanes.Unhealthy)
	})
}

func TestClusterReconciler_reconcileControlPlanes(t *testing.T) {
	scheme := setupTestScheme(t)

	t.Run("skips replacement for single control plane", func(t *testing.T) {
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
	scheme := setupTestScheme(t)

	t.Run("replaces unhealthy workers", func(t *testing.T) {
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
}

func TestClusterReconciler_updateClusterPhase(t *testing.T) {
	t.Run("sets Running when all healthy", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Desired: 3,
					Ready:   3,
				},
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Desired: 2,
					Ready:   2,
				},
			},
		}

		r := &ClusterReconciler{}
		r.updateClusterPhase(cluster)

		assert.Equal(t, k8znerv1alpha1.ClusterPhaseRunning, cluster.Status.Phase)
	})

	t.Run("sets Degraded when unhealthy", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Desired: 3,
					Ready:   2,
				},
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Desired: 2,
					Ready:   2,
				},
			},
		}

		r := &ClusterReconciler{}
		r.updateClusterPhase(cluster)

		assert.Equal(t, k8znerv1alpha1.ClusterPhaseDegraded, cluster.Status.Phase)
	})

	t.Run("preserves Healing phase", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Phase: k8znerv1alpha1.ClusterPhaseHealing,
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Desired: 3,
					Ready:   2,
				},
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Desired: 2,
					Ready:   2,
				},
			},
		}

		r := &ClusterReconciler{}
		r.updateClusterPhase(cluster)

		assert.Equal(t, k8znerv1alpha1.ClusterPhaseHealing, cluster.Status.Phase)
	})
}

func TestHelperFunctions(t *testing.T) {
	t.Run("isNodeReady", func(t *testing.T) {
		readyNode := &corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		}
		assert.True(t, isNodeReady(readyNode))

		notReadyNode := &corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
				},
			},
		}
		assert.False(t, isNodeReady(notReadyNode))

		noConditionNode := &corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{},
			},
		}
		assert.False(t, isNodeReady(noConditionNode))
	})

	t.Run("getNodeUnhealthyReason", func(t *testing.T) {
		notReadyNode := &corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Message: "Kubelet stopped posting node status"},
				},
			},
		}
		assert.Contains(t, getNodeUnhealthyReason(notReadyNode), "NodeNotReady")

		memPressureNode := &corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
				},
			},
		}
		assert.Equal(t, "MemoryPressure", getNodeUnhealthyReason(memPressureNode))

		diskPressureNode := &corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue},
				},
			},
		}
		assert.Equal(t, "DiskPressure", getNodeUnhealthyReason(diskPressureNode))
	})

	t.Run("parseThreshold", func(t *testing.T) {
		// Nil health check uses defaults
		assert.Equal(t, defaultNodeNotReadyThreshold, parseThreshold(nil, "node"))
		assert.Equal(t, defaultEtcdUnhealthyThreshold, parseThreshold(nil, "etcd"))

		// Custom values
		healthCheck := &k8znerv1alpha1.HealthCheckSpec{
			NodeNotReadyThreshold:  "10m",
			EtcdUnhealthyThreshold: "3m",
		}
		assert.Equal(t, 10*time.Minute, parseThreshold(healthCheck, "node"))
		assert.Equal(t, 3*time.Minute, parseThreshold(healthCheck, "etcd"))

		// Invalid values fall back to defaults
		invalidHealthCheck := &k8znerv1alpha1.HealthCheckSpec{
			NodeNotReadyThreshold: "invalid",
		}
		assert.Equal(t, defaultNodeNotReadyThreshold, parseThreshold(invalidHealthCheck, "node"))
	})

	t.Run("conditionStatus", func(t *testing.T) {
		assert.Equal(t, metav1.ConditionTrue, conditionStatus(true))
		assert.Equal(t, metav1.ConditionFalse, conditionStatus(false))
	})

	t.Run("conditionReason", func(t *testing.T) {
		assert.Equal(t, "Ready", conditionReason(true, "Ready", "NotReady"))
		assert.Equal(t, "NotReady", conditionReason(false, "Ready", "NotReady"))
	})
}

// Helper function to create test nodes
func createTestNode(name string, isControlPlane, isReady bool) *corev1.Node {
	labels := map[string]string{}
	if isControlPlane {
		labels["node-role.kubernetes.io/control-plane"] = ""
	}

	readyStatus := corev1.ConditionFalse
	if isReady {
		readyStatus = corev1.ConditionTrue
	}

	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: corev1.NodeSpec{
			ProviderID: "hcloud://12345",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             readyStatus,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
				},
			},
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
				{Type: corev1.NodeExternalIP, Address: "1.2.3.4"},
			},
		},
	}
}

// TestSelfHealingControlPlaneReplacement tests the full control plane replacement cycle.
func TestSelfHealingControlPlaneReplacement(t *testing.T) {
	scheme := setupTestScheme(t)

	t.Run("successful control plane replacement with full cycle", func(t *testing.T) {
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
				return "10.0.0.4", nil // New server IP
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

		// Execute the replacement
		node := &k8znerv1alpha1.NodeStatus{
			Name:            "test-cluster-control-plane-3",
			PrivateIP:       "10.0.0.3",
			ServerID:        12345,
			UnhealthyReason: "NodeNotReady",
		}
		err := r.replaceControlPlane(context.Background(), cluster, node)
		require.NoError(t, err)

		// Verify server was deleted
		assert.Contains(t, mockHCloud.DeleteServerCalls, "test-cluster-control-plane-3")

		// Verify new server was created
		assert.Len(t, mockHCloud.CreateServerCalls, 1)
		createCall := mockHCloud.CreateServerCalls[0]
		assert.Equal(t, "cx22", createCall.ServerType)
		assert.Equal(t, "nbg1", createCall.Location)
		assert.Equal(t, "control-plane", createCall.Labels["role"])

		// Verify etcd member removal was attempted
		assert.Len(t, mockTalos.GetEtcdMembersCalls, 1)

		// Verify network was queried
		assert.Len(t, mockHCloud.GetNetworkCalls, 1)
		assert.Equal(t, "test-cluster-network", mockHCloud.GetNetworkCalls[0])

		// Verify snapshot was queried
		assert.Len(t, mockHCloud.GetSnapshotByLabelsCalls, 1)

		// Verify config was generated and applied
		assert.Len(t, mockTalosGen.GenerateControlPlaneConfigCalls, 1)
		assert.Len(t, mockTalos.ApplyConfigCalls, 1)
		assert.Equal(t, "10.0.0.4", mockTalos.ApplyConfigCalls[0].NodeIP)

		// Verify wait for node ready was called
		assert.Len(t, mockTalos.WaitForNodeReadyCalls, 1)
		assert.Equal(t, "10.0.0.4", mockTalos.WaitForNodeReadyCalls[0].NodeIP)
	})

	t.Run("control plane replacement fails on server creation", func(t *testing.T) {
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
			CreateServerFunc: func(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string, placementGroupID *int64, networkID int64, privateIP string, enablePublicIPv4, enablePublicIPv6 bool) (string, error) {
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
		assert.Contains(t, err.Error(), "failed to apply config")

		// Server should have been created even if config apply failed
		assert.Len(t, mockHCloud.CreateServerCalls, 1)
	})
}

// TestSelfHealingWorkerReplacement tests the full worker replacement cycle.
func TestSelfHealingWorkerReplacement(t *testing.T) {
	scheme := setupTestScheme(t)

	t.Run("successful worker replacement with full cycle", func(t *testing.T) {
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
				return "10.0.0.12", nil // New server IP
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
			Name:            "test-cluster-workers-2",
			PrivateIP:       "10.0.0.11",
			ServerID:        12346,
			UnhealthyReason: "NodeNotReady",
		}
		err := r.replaceWorker(context.Background(), cluster, node)
		require.NoError(t, err)

		// Verify server was deleted
		assert.Contains(t, mockHCloud.DeleteServerCalls, "test-cluster-workers-2")

		// Verify new server was created with correct type
		assert.Len(t, mockHCloud.CreateServerCalls, 1)
		createCall := mockHCloud.CreateServerCalls[0]
		assert.Equal(t, "cx32", createCall.ServerType) // Worker size
		assert.Equal(t, "nbg1", createCall.Location)
		assert.Equal(t, "worker", createCall.Labels["role"])
		assert.Equal(t, "workers", createCall.Labels["pool"])

		// Verify worker config was generated (not control plane)
		assert.Len(t, mockTalosGen.GenerateWorkerConfigCalls, 1)
		assert.Len(t, mockTalosGen.GenerateControlPlaneConfigCalls, 0)

		// Verify config was applied
		assert.Len(t, mockTalos.ApplyConfigCalls, 1)
		assert.Equal(t, "10.0.0.12", mockTalos.ApplyConfigCalls[0].NodeIP)

		// Verify wait for node ready was called
		assert.Len(t, mockTalos.WaitForNodeReadyCalls, 1)
	})

	t.Run("worker replacement continues without talos clients", func(t *testing.T) {
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

		// No talos clients configured
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

		// Server should still be created
		assert.Len(t, mockHCloud.CreateServerCalls, 1)
		// But no talos operations should occur
	})
}

// TestBuildClusterState tests the buildClusterState helper function.
func TestBuildClusterState(t *testing.T) {
	scheme := setupTestScheme(t)

	t.Run("builds state with SSH keys from annotations", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cluster",
				Namespace: "default",
				Annotations: map[string]string{
					"k8zner.io/ssh-keys":               "key1,key2,key3",
					"k8zner.io/control-plane-endpoint": "1.2.3.4",
				},
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "fsn1",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1", PublicIP: "1.2.3.1"},
						{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2", PublicIP: "1.2.3.2"},
					},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		state, err := r.buildClusterState(context.Background(), cluster)
		require.NoError(t, err)

		assert.Equal(t, "my-cluster", state.Name)
		assert.Equal(t, "fsn1", state.Region)
		assert.Equal(t, []string{"key1", "key2", "key3"}, state.SSHKeyIDs)
		assert.Equal(t, "1.2.3.4", state.ControlPlaneIP)
		assert.Contains(t, state.SANs, "10.0.0.1")
		assert.Contains(t, state.SANs, "10.0.0.2")
		assert.Contains(t, state.SANs, "1.2.3.1")
		assert.Contains(t, state.SANs, "1.2.3.2")
		assert.Equal(t, int64(1), state.NetworkID)
	})

	t.Run("uses default SSH key naming when no annotation", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cluster",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			Build()
		recorder := record.NewFakeRecorder(10)

		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		state, err := r.buildClusterState(context.Background(), cluster)
		require.NoError(t, err)

		assert.Equal(t, []string{"my-cluster-key"}, state.SSHKeyIDs)
	})
}

// TestGenerateReplacementServerName tests server name generation.
func TestGenerateReplacementServerName(t *testing.T) {
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-cluster",
		},
	}

	t.Run("preserves pool and index from old name", func(t *testing.T) {
		name := r.generateReplacementServerName(cluster, "control-plane", "my-cluster-control-plane-3")
		assert.Equal(t, "my-cluster-control-plane-3", name)
	})

	t.Run("preserves worker pool name", func(t *testing.T) {
		name := r.generateReplacementServerName(cluster, "worker", "my-cluster-workers-2")
		assert.Equal(t, "my-cluster-workers-2", name)
	})

	t.Run("generates new name with timestamp for invalid old name", func(t *testing.T) {
		name := r.generateReplacementServerName(cluster, "worker", "invalid-name")
		assert.Contains(t, name, "my-cluster-worker-")
	})
}

// TestScaleUpWorkers tests the worker scale up functionality.
func TestScaleUpWorkers(t *testing.T) {
	scheme := setupTestScheme(t)

	t.Run("creates new workers successfully", func(t *testing.T) {
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

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithTalosClient(mockTalos),
			WithTalosConfigGenerator(mockTalosGen),
			WithMaxConcurrentHeals(5),
			WithMetrics(false),
		)

		err := r.scaleUpWorkers(context.Background(), cluster, 2)
		require.NoError(t, err)

		// Verify two servers were created
		assert.Len(t, mockHCloud.CreateServerCalls, 2)

		// Verify first server has correct naming (index 2, since 1 exists)
		assert.Equal(t, "test-cluster-workers-2", mockHCloud.CreateServerCalls[0].Name)
		assert.Equal(t, "cx22", mockHCloud.CreateServerCalls[0].ServerType)
		assert.Equal(t, "nbg1", mockHCloud.CreateServerCalls[0].Location)
		assert.Equal(t, "worker", mockHCloud.CreateServerCalls[0].Labels["role"])

		// Verify second server has correct naming (index 3)
		assert.Equal(t, "test-cluster-workers-3", mockHCloud.CreateServerCalls[1].Name)

		// Verify talos configs were generated
		assert.Len(t, mockTalosGen.GenerateWorkerConfigCalls, 2)

		// Verify configs were applied
		assert.Len(t, mockTalos.ApplyConfigCalls, 2)

		// Verify wait for node ready was called
		assert.Len(t, mockTalos.WaitForNodeReadyCalls, 2)
	})

	t.Run("respects maxConcurrentHeals limit", func(t *testing.T) {
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

		// maxConcurrentHeals defaults to 1
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		// Request 5 workers but only 1 should be created due to maxConcurrentHeals
		err := r.scaleUpWorkers(context.Background(), cluster, 5)
		// Should return error since we created less than requested
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only created 1 of 5")

		// Only one server should be created
		assert.Len(t, mockHCloud.CreateServerCalls, 1)
	})

	t.Run("handles missing snapshot", func(t *testing.T) {
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
				return nil, nil // No snapshot found
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		err := r.scaleUpWorkers(context.Background(), cluster, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no Talos snapshot found")

		// No servers should be created
		assert.Len(t, mockHCloud.CreateServerCalls, 0)
	})

	t.Run("works without talos clients", func(t *testing.T) {
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

		// No talos clients
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMaxConcurrentHeals(5),
			WithMetrics(false),
		)

		err := r.scaleUpWorkers(context.Background(), cluster, 2)
		require.NoError(t, err)

		// Servers should still be created
		assert.Len(t, mockHCloud.CreateServerCalls, 2)
	})
}

// TestFindNextWorkerIndex tests the worker index finding logic.
func TestFindNextWorkerIndex(t *testing.T) {
	scheme := setupTestScheme(t)

	t.Run("returns 1 for empty worker list", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := NewClusterReconciler(client, scheme, nil)

		idx := r.findNextWorkerIndex(cluster)
		assert.Equal(t, 1, idx)
	})

	t.Run("finds next index after existing workers", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cluster-workers-1"},
						{Name: "cluster-workers-2"},
						{Name: "cluster-workers-3"},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := NewClusterReconciler(client, scheme, nil)

		idx := r.findNextWorkerIndex(cluster)
		assert.Equal(t, 4, idx)
	})

	t.Run("handles gaps in worker indices", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cluster-workers-1"},
						{Name: "cluster-workers-5"}, // Gap: 2, 3, 4 missing
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := NewClusterReconciler(client, scheme, nil)

		idx := r.findNextWorkerIndex(cluster)
		assert.Equal(t, 6, idx) // Should be max+1
	})

	t.Run("handles non-numeric suffix", func(t *testing.T) {
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cluster-workers-abc"}, // Non-numeric
						{Name: "cluster-workers-2"},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := NewClusterReconciler(client, scheme, nil)

		idx := r.findNextWorkerIndex(cluster)
		assert.Equal(t, 3, idx)
	})
}
