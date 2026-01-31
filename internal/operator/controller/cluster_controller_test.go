package controller

import (
	"context"
	"testing"
	"time"

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
