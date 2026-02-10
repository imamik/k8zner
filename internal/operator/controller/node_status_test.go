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
		PublicIP:   "1.2.3.4",
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
		PublicIP:   "5.6.7.8",
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
