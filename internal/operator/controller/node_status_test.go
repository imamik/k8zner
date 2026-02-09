package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

func TestShouldUpdatePhase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		current  k8znerv1alpha1.NodePhase
		newPhase k8znerv1alpha1.NodePhase
		expected bool
	}{
		{
			name:     "allow transition to Ready",
			current:  k8znerv1alpha1.NodePhaseWaitingForK8s,
			newPhase: k8znerv1alpha1.NodePhaseReady,
			expected: true,
		},
		{
			name:     "allow transition to Failed",
			current:  k8znerv1alpha1.NodePhaseCreatingServer,
			newPhase: k8znerv1alpha1.NodePhaseFailed,
			expected: true,
		},
		{
			name:     "allow forward progression",
			current:  k8znerv1alpha1.NodePhaseCreatingServer,
			newPhase: k8znerv1alpha1.NodePhaseWaitingForIP,
			expected: true,
		},
		{
			name:     "deny backward progression",
			current:  k8znerv1alpha1.NodePhaseWaitingForK8s,
			newPhase: k8znerv1alpha1.NodePhaseCreatingServer,
			expected: false,
		},
		{
			name:     "allow same phase (no-op handled elsewhere)",
			current:  k8znerv1alpha1.NodePhaseWaitingForIP,
			newPhase: k8znerv1alpha1.NodePhaseWaitingForIP,
			expected: false,
		},
		{
			name:     "allow unknown current phase",
			current:  k8znerv1alpha1.NodePhase("Unknown"),
			newPhase: k8znerv1alpha1.NodePhaseWaitingForIP,
			expected: true,
		},
		{
			name:     "allow unknown new phase",
			current:  k8znerv1alpha1.NodePhaseWaitingForIP,
			newPhase: k8znerv1alpha1.NodePhase("CustomPhase"),
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

func TestIsNodeInEarlyProvisioningPhase(t *testing.T) {
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
		assert.True(t, isNodeInEarlyProvisioningPhase(phase), "phase %s should be early provisioning", phase)
	}

	nonEarlyPhases := []k8znerv1alpha1.NodePhase{
		k8znerv1alpha1.NodePhaseReady,
		k8znerv1alpha1.NodePhaseUnhealthy,
		k8znerv1alpha1.NodePhaseFailed,
		k8znerv1alpha1.NodePhaseDraining,
		k8znerv1alpha1.NodePhaseRemovingFromEtcd,
		k8znerv1alpha1.NodePhaseDeletingServer,
	}

	for _, phase := range nonEarlyPhases {
		assert.False(t, isNodeInEarlyProvisioningPhase(phase), "phase %s should NOT be early provisioning", phase)
	}
}

func TestCountWorkersInEarlyProvisioning(t *testing.T) {
	t.Parallel()
	t.Run("no workers", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 0, countWorkersInEarlyProvisioning(nil))
	})

	t.Run("all ready", func(t *testing.T) {
		t.Parallel()
		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseReady},
			{Name: "w-2", Phase: k8znerv1alpha1.NodePhaseReady},
		}
		assert.Equal(t, 0, countWorkersInEarlyProvisioning(nodes))
	})

	t.Run("some provisioning", func(t *testing.T) {
		t.Parallel()
		nodes := []k8znerv1alpha1.NodeStatus{
			{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseReady},
			{Name: "w-2", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
			{Name: "w-3", Phase: k8znerv1alpha1.NodePhaseWaitingForK8s},
			{Name: "w-4", Phase: k8znerv1alpha1.NodePhaseFailed},
		}
		assert.Equal(t, 2, countWorkersInEarlyProvisioning(nodes))
	})
}

func TestCheckStuckNodes(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	t.Run("no stuck nodes", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{
							Name:  "cp-1",
							Phase: k8znerv1alpha1.NodePhaseReady, // Terminal phase - skipped
						},
					},
				},
			},
		}

		stuck := r.checkStuckNodes(t.Context(), cluster)
		assert.Empty(t, stuck)
	})

	t.Run("node stuck in creating server", func(t *testing.T) {
		t.Parallel()
		pastTime := metav1.NewTime(time.Now().Add(-15 * time.Minute))
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{
							Name:                "cp-1",
							Phase:               k8znerv1alpha1.NodePhaseCreatingServer,
							PhaseTransitionTime: &pastTime, // 15 min ago, timeout is 10 min
						},
					},
				},
			},
		}

		stuck := r.checkStuckNodes(t.Context(), cluster)
		require.Len(t, stuck, 1)
		assert.Equal(t, "cp-1", stuck[0].Name)
		assert.Equal(t, "control-plane", stuck[0].Role)
		assert.Equal(t, k8znerv1alpha1.NodePhaseCreatingServer, stuck[0].Phase)
	})

	t.Run("node not yet timed out", func(t *testing.T) {
		t.Parallel()
		recentTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{
							Name:                "w-1",
							Phase:               k8znerv1alpha1.NodePhaseCreatingServer,
							PhaseTransitionTime: &recentTime, // 1 min ago, timeout is 10 min
						},
					},
				},
			},
		}

		stuck := r.checkStuckNodes(t.Context(), cluster)
		assert.Empty(t, stuck)
	})
}

func TestRemoveNodeFromStatus(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	t.Run("remove control plane node", func(t *testing.T) {
		t.Parallel()
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

		r.removeNodeFromStatus(cluster, "control-plane", "cp-2")
		assert.Len(t, cluster.Status.ControlPlanes.Nodes, 2)

		names := make([]string, len(cluster.Status.ControlPlanes.Nodes))
		for i, n := range cluster.Status.ControlPlanes.Nodes {
			names[i] = n.Name
		}
		assert.NotContains(t, names, "cp-2")
	})

	t.Run("remove worker node", func(t *testing.T) {
		t.Parallel()
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

		r.removeNodeFromStatus(cluster, "worker", "w-1")
		require.Len(t, cluster.Status.Workers.Nodes, 1)
		assert.Equal(t, "w-2", cluster.Status.Workers.Nodes[0].Name)
	})

	t.Run("remove nonexistent node is no-op", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1"},
					},
				},
			},
		}

		r.removeNodeFromStatus(cluster, "control-plane", "cp-nonexistent")
		assert.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	})
}
