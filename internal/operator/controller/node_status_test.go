package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
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

func TestUpdateNodePhase_UpdateExistingWithIPs(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:  "cp-1",
						Phase: k8znerv1alpha1.NodePhaseCreatingServer,
					},
				},
			},
		},
	}

	// Update with ServerID, PublicIP, and PrivateIP
	r.updateNodePhase(t.Context(), cluster, "control-plane", nodeStatusUpdate{
		Name:      "cp-1",
		Phase:     k8znerv1alpha1.NodePhaseWaitingForIP,
		Reason:    "Server created, waiting for IP assignment",
		ServerID:  12345,
		PublicIP:  "1.2.3.4",
		PrivateIP: "10.0.0.5",
	})

	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	node := cluster.Status.ControlPlanes.Nodes[0]
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForIP, node.Phase)
	assert.Equal(t, int64(12345), node.ServerID)
	assert.Equal(t, "1.2.3.4", node.PublicIP)
	assert.Equal(t, "10.0.0.5", node.PrivateIP)
	assert.NotNil(t, node.PhaseTransitionTime, "transition time should be set on phase change")
}

func TestUpdateNodePhase_AddNewNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{},
	}

	// Add a new worker node
	r.updateNodePhase(t.Context(), cluster, "worker", nodeStatusUpdate{
		Name:      "w-1",
		Phase:     k8znerv1alpha1.NodePhaseCreatingServer,
		Reason:    "Creating server",
		ServerID:  99999,
		PublicIP:  "5.6.7.8",
		PrivateIP: "10.0.1.1",
	})

	require.Len(t, cluster.Status.Workers.Nodes, 1)
	node := cluster.Status.Workers.Nodes[0]
	assert.Equal(t, "w-1", node.Name)
	assert.Equal(t, k8znerv1alpha1.NodePhaseCreatingServer, node.Phase)
	assert.Equal(t, int64(99999), node.ServerID)
	assert.Equal(t, "5.6.7.8", node.PublicIP)
	assert.Equal(t, "10.0.1.1", node.PrivateIP)
}

func TestUpdateNodePhase_SamePhaseNoTransitionTimeUpdate(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	pastTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:                "w-1",
						Phase:               k8znerv1alpha1.NodePhaseWaitingForK8s,
						PhaseTransitionTime: &pastTime,
					},
				},
			},
		},
	}

	// Update with same phase - should NOT change transition time
	r.updateNodePhase(t.Context(), cluster, "worker", nodeStatusUpdate{
		Name:   "w-1",
		Phase:  k8znerv1alpha1.NodePhaseWaitingForK8s,
		Reason: "Still waiting",
	})

	node := cluster.Status.Workers.Nodes[0]
	assert.Equal(t, pastTime.Time, node.PhaseTransitionTime.Time,
		"transition time should not change when phase stays the same")
}

func TestCheckStuckNodes_NilTransitionTime(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:                "w-1",
						Phase:               k8znerv1alpha1.NodePhaseCreatingServer,
						PhaseTransitionTime: nil, // nil transition time should be skipped
					},
				},
			},
		},
	}

	stuck := r.checkStuckNodes(t.Context(), cluster)
	assert.Empty(t, stuck, "nodes with nil PhaseTransitionTime should be skipped")
}

func TestCheckStuckNodes_SkipsUnhealthyAndFailed(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	pastTime := metav1.NewTime(time.Now().Add(-30 * time.Minute))
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:                "cp-1",
						Phase:               k8znerv1alpha1.NodePhaseUnhealthy,
						PhaseTransitionTime: &pastTime,
					},
					{
						Name:                "cp-2",
						Phase:               k8znerv1alpha1.NodePhaseFailed,
						PhaseTransitionTime: &pastTime,
					},
				},
			},
		},
	}

	stuck := r.checkStuckNodes(t.Context(), cluster)
	assert.Empty(t, stuck, "Unhealthy and Failed nodes should be skipped")
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

func TestHandleStuckNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("marks node as failed and deletes server", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Phase: k8znerv1alpha1.NodePhaseReady},
						{Name: "cp-2", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
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

		stuck := stuckNode{
			Name:    "cp-2",
			Role:    "control-plane",
			Phase:   k8znerv1alpha1.NodePhaseCreatingServer,
			Elapsed: 15 * time.Minute,
			Timeout: 10 * time.Minute,
		}

		err := r.handleStuckNode(context.Background(), cluster, stuck)
		require.NoError(t, err)

		// Server should be deleted
		assert.Contains(t, mockHCloud.DeleteServerCalls, "cp-2")

		// Node should be removed from status
		assert.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
		assert.Equal(t, "cp-1", cluster.Status.ControlPlanes.Nodes[0].Name)
	})

	t.Run("handles worker node stuck", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Workers: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseReady},
						{Name: "w-2", Phase: k8znerv1alpha1.NodePhaseWaitingForK8s},
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

		stuck := stuckNode{
			Name:    "w-2",
			Role:    "worker",
			Phase:   k8znerv1alpha1.NodePhaseWaitingForK8s,
			Elapsed: 12 * time.Minute,
			Timeout: 10 * time.Minute,
		}

		err := r.handleStuckNode(context.Background(), cluster, stuck)
		require.NoError(t, err)

		assert.Contains(t, mockHCloud.DeleteServerCalls, "w-2")
		assert.Len(t, cluster.Status.Workers.Nodes, 1)
		assert.Equal(t, "w-1", cluster.Status.Workers.Nodes[0].Name)
	})

	t.Run("continues even if server deletion fails", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
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
		mockHCloud := &MockHCloudClient{
			DeleteServerFunc: func(_ context.Context, _ string) error {
				return errors.New("hcloud API error")
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		stuck := stuckNode{
			Name:    "cp-1",
			Role:    "control-plane",
			Phase:   k8znerv1alpha1.NodePhaseCreatingServer,
			Elapsed: 15 * time.Minute,
			Timeout: 10 * time.Minute,
		}

		// Should NOT return error even when delete fails
		err := r.handleStuckNode(context.Background(), cluster, stuck)
		require.NoError(t, err)

		// Node should still be removed from status
		assert.Empty(t, cluster.Status.ControlPlanes.Nodes)
	})
}

func TestFindUnhealthyNodes_Empty(t *testing.T) {
	t.Parallel()
	result := findUnhealthyNodes(nil, 3*time.Minute)
	assert.Empty(t, result)
}

func TestFindUnhealthyNodes_AllHealthy(t *testing.T) {
	t.Parallel()
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: true},
		{Name: "w-2", Healthy: true},
	}
	result := findUnhealthyNodes(nodes, 3*time.Minute)
	assert.Empty(t, result)
}

func TestFindUnhealthyNodes_UnhealthyBelowThreshold(t *testing.T) {
	t.Parallel()
	recentTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: false, UnhealthySince: &recentTime},
	}
	// Threshold is 3 minutes, node has been unhealthy for only 1 minute
	result := findUnhealthyNodes(nodes, 3*time.Minute)
	assert.Empty(t, result, "node unhealthy for less than threshold should not be returned")
}

func TestFindUnhealthyNodes_UnhealthyAboveThreshold(t *testing.T) {
	t.Parallel()
	pastTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: false, UnhealthySince: &pastTime},
		{Name: "w-2", Healthy: true},
		{Name: "w-3", Healthy: false, UnhealthySince: &pastTime},
	}
	// Threshold is 3 minutes, nodes have been unhealthy for 5 minutes
	result := findUnhealthyNodes(nodes, 3*time.Minute)
	require.Len(t, result, 2)
	assert.Equal(t, "w-1", result[0].Name)
	assert.Equal(t, "w-3", result[1].Name)
}

func TestFindUnhealthyNodes_NilUnhealthySince(t *testing.T) {
	t.Parallel()
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: false, UnhealthySince: nil},
	}
	// Unhealthy but no UnhealthySince timestamp - should be skipped
	result := findUnhealthyNodes(nodes, 3*time.Minute)
	assert.Empty(t, result, "nodes with nil UnhealthySince should be skipped")
}

func TestFindUnhealthyNodes_MixedStates(t *testing.T) {
	t.Parallel()
	oldTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	recentTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: true},                               // healthy
		{Name: "w-2", Healthy: false, UnhealthySince: &oldTime},    // past threshold
		{Name: "w-3", Healthy: false, UnhealthySince: nil},         // no timestamp
		{Name: "w-4", Healthy: false, UnhealthySince: &recentTime}, // below threshold
		{Name: "w-5", Healthy: false, UnhealthySince: &oldTime},    // past threshold
	}
	result := findUnhealthyNodes(nodes, 3*time.Minute)
	require.Len(t, result, 2)
	assert.Equal(t, "w-2", result[0].Name)
	assert.Equal(t, "w-5", result[1].Name)
}

func TestCheckStuckNodes_PhaseWithoutTimeoutIsSkipped(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	pastTime := metav1.NewTime(time.Now().Add(-30 * time.Minute))
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						// Use a phase that is NOT in phaseTimeouts map
						Name:                "w-1",
						Phase:               k8znerv1alpha1.NodePhase("SomeCustomPhase"),
						PhaseTransitionTime: &pastTime,
					},
				},
			},
		},
	}

	stuck := r.checkStuckNodes(t.Context(), cluster)
	assert.Empty(t, stuck, "node in a phase without timeout config should be skipped")
}

func TestUpdateNodePhase_ReadyPhaseMarkHealthy(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseWaitingForK8s, Healthy: false},
				},
			},
		},
	}

	r.updateNodePhase(t.Context(), cluster, "worker", nodeStatusUpdate{
		Name:  "w-1",
		Phase: k8znerv1alpha1.NodePhaseReady,
	})

	assert.True(t, cluster.Status.Workers.Nodes[0].Healthy, "Ready phase should mark node as healthy")
}

func TestUpdateNodePhase_NonReadyPhaseMarkUnhealthy(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseReady, Healthy: true},
				},
			},
		},
	}

	r.updateNodePhase(t.Context(), cluster, "worker", nodeStatusUpdate{
		Name:  "w-1",
		Phase: k8znerv1alpha1.NodePhaseFailed,
	})

	assert.False(t, cluster.Status.Workers.Nodes[0].Healthy, "Failed phase should mark node as unhealthy")
}

func TestUpdateNodePhase_PreservesExistingFieldsOnZeroUpdate(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:      "cp-1",
						ServerID:  12345,
						PublicIP:  "1.2.3.4",
						PrivateIP: "10.0.0.1",
						Phase:     k8znerv1alpha1.NodePhaseWaitingForK8s,
					},
				},
			},
		},
	}

	// Update with zero ServerID, empty IPs - should preserve existing values
	r.updateNodePhase(t.Context(), cluster, "control-plane", nodeStatusUpdate{
		Name:   "cp-1",
		Phase:  k8znerv1alpha1.NodePhaseReady,
		Reason: "Node ready",
		// ServerID=0, PublicIP="", PrivateIP="" - all zero/empty
	})

	node := cluster.Status.ControlPlanes.Nodes[0]
	assert.Equal(t, int64(12345), node.ServerID, "existing ServerID preserved when update has 0")
	assert.Equal(t, "1.2.3.4", node.PublicIP, "existing PublicIP preserved when update is empty")
	assert.Equal(t, "10.0.0.1", node.PrivateIP, "existing PrivateIP preserved when update is empty")
}

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
