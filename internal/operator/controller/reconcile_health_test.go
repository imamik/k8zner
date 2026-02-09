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
)

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
