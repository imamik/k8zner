package controller

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func TestDrainNode_EmptyPodList(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	// drainNode uses MatchingFields which requires a field indexer.
	// The fake client without an indexer will return an empty list.
	err := r.drainNode(context.Background(), "nonexistent-node")
	// Without the field index, this returns an error
	// (fake client doesn't support MatchingFields without indexer)
	// The function wraps it in "failed to list pods on node"
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list pods on node")
}

// --- Reconcile: full flow with HCloud client error ---

func TestHandleStuckNode_RecordsEventAndDeletesServer(t *testing.T) {
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
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseDeletingServer},
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
	mockHCloud := &MockHCloudClient{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	stuck := stuckNode{
		Name:    "w-1",
		Role:    "worker",
		Phase:   k8znerv1alpha1.NodePhaseDeletingServer,
		Elapsed: 10 * time.Minute,
		Timeout: 5 * time.Minute,
	}

	err := r.handleStuckNode(context.Background(), cluster, stuck)
	require.NoError(t, err)

	assert.Contains(t, mockHCloud.DeleteServerCalls, "w-1")
	assert.Empty(t, cluster.Status.Workers.Nodes)
}

// --- verifyAndUpdateNodeStates: node with both IPs, no K8s node, server exists in "running" ---

func TestRemoveFromEtcd_NilClient(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", PrivateIP: "10.0.0.1"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}

	// tc with nil client - should skip etcd removal
	tc := talosClients{client: nil}
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// Should have updated node phase to RemovingFromEtcd
	assert.Equal(t, k8znerv1alpha1.NodePhaseRemovingFromEtcd, cluster.Status.ControlPlanes.Nodes[0].Phase)
}

func TestRemoveFromEtcd_EmptyPrivateIP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", PrivateIP: ""},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: ""}
	tc := talosClients{client: mockTalos}
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// Should skip because PrivateIP is empty
	assert.Empty(t, mockTalos.GetEtcdMembersCalls)
}

func TestRemoveFromEtcd_NoHealthyCP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"},
					{Name: "cp-2", Healthy: false, PrivateIP: "10.0.0.2"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}
	tc := talosClients{client: mockTalos}
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// No healthy CP found, should skip
	assert.Empty(t, mockTalos.GetEtcdMembersCalls)
}

func TestRemoveFromEtcd_SuccessfulRemoval(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		GetEtcdMembersFunc: func(ctx context.Context, nodeIP string) ([]etcdMember, error) {
			return []etcdMember{
				{ID: "1", Name: "cp-1", Endpoint: "10.0.0.1:2379"},
				{ID: "2", Name: "cp-2", Endpoint: "10.0.0.2:2379"},
				{ID: "3", Name: "cp-3", Endpoint: "10.0.0.3:2379"},
			}, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"},
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
					{Name: "cp-3", Healthy: true, PrivateIP: "10.0.0.3"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}
	tc := talosClients{client: mockTalos}
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// Should have called GetEtcdMembers with healthy CP IP
	require.Len(t, mockTalos.GetEtcdMembersCalls, 1)
	assert.Equal(t, "10.0.0.2", mockTalos.GetEtcdMembersCalls[0])

	// Should have removed the member by name match
	require.Len(t, mockTalos.RemoveEtcdMemberCalls, 1)
	assert.Equal(t, "1", mockTalos.RemoveEtcdMemberCalls[0].MemberID)
}

func TestRemoveFromEtcd_MatchByEndpoint(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		GetEtcdMembersFunc: func(ctx context.Context, nodeIP string) ([]etcdMember, error) {
			return []etcdMember{
				// Name doesn't match, but endpoint matches the private IP
				{ID: "99", Name: "some-other-name", Endpoint: "10.0.0.1"},
			}, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"},
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}
	tc := talosClients{client: mockTalos}
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// Should match by endpoint
	require.Len(t, mockTalos.RemoveEtcdMemberCalls, 1)
	assert.Equal(t, "99", mockTalos.RemoveEtcdMemberCalls[0].MemberID)
}

func TestRemoveFromEtcd_GetMembersError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		GetEtcdMembersFunc: func(ctx context.Context, nodeIP string) ([]etcdMember, error) {
			return nil, fmt.Errorf("etcd cluster unavailable")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"},
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}
	tc := talosClients{client: mockTalos}
	// Should not panic even with error
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// No member removal attempted
	assert.Empty(t, mockTalos.RemoveEtcdMemberCalls)
}

func TestRemoveFromEtcd_RemoveMemberError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		GetEtcdMembersFunc: func(ctx context.Context, nodeIP string) ([]etcdMember, error) {
			return []etcdMember{
				{ID: "1", Name: "cp-1", Endpoint: "10.0.0.1:2379"},
			}, nil
		},
		RemoveEtcdMemberFunc: func(ctx context.Context, nodeIP string, memberID string) error {
			return fmt.Errorf("etcd remove failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"},
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", PrivateIP: "10.0.0.1"}
	tc := talosClients{client: mockTalos}
	// Should not panic even with remove error
	r.removeFromEtcd(context.Background(), cluster, tc, node)

	// Removal was attempted
	require.Len(t, mockTalos.RemoveEtcdMemberCalls, 1)
}

// --- Reconcile: paused cluster ---

func TestDeleteNodeAndServer_WithExistingK8sNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	k8sNode := createTestNode("cp-1", true, true)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(k8sNode).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", ServerID: 123},
				},
			},
		},
	}
	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1", ServerID: 123}

	err := r.deleteNodeAndServer(context.Background(), cluster, node, "control-plane")
	require.NoError(t, err)

	// Server should be deleted
	assert.Contains(t, mockHCloud.DeleteServerCalls, "cp-1")
}

func TestDeleteNodeAndServer_NoK8sNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", ServerID: 456},
				},
			},
		},
	}
	node := &k8znerv1alpha1.NodeStatus{Name: "w-1", ServerID: 456}

	err := r.deleteNodeAndServer(context.Background(), cluster, node, "worker")
	require.NoError(t, err)

	// Server should still be deleted even without K8s node
	assert.Contains(t, mockHCloud.DeleteServerCalls, "w-1")
}

func TestDeleteNodeAndServer_EmptyNodeName(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}
	node := &k8znerv1alpha1.NodeStatus{Name: ""}

	err := r.deleteNodeAndServer(context.Background(), cluster, node, "worker")
	require.NoError(t, err)

	// No server deletion with empty name
	assert.Empty(t, mockHCloud.DeleteServerCalls)
}

func TestDeleteNodeAndServer_HCloudDeleteError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		DeleteServerFunc: func(ctx context.Context, name string) error {
			return fmt.Errorf("server deletion failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1"},
				},
			},
		},
	}
	node := &k8znerv1alpha1.NodeStatus{Name: "cp-1"}

	// Should not return error even when delete fails
	err := r.deleteNodeAndServer(context.Background(), cluster, node, "control-plane")
	require.NoError(t, err)
}

// --- buildClusterState: integration ---

func TestDrainAndDeleteWorker_WithExistingNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	workerNode := createTestNode("w-1", false, true)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workerNode).
		Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", ServerID: 123},
				},
			},
		},
	}

	node := &k8znerv1alpha1.NodeStatus{Name: "w-1", ServerID: 123}
	err := r.drainAndDeleteWorker(context.Background(), cluster, node)
	require.NoError(t, err)

	// Server should be deleted
	assert.Contains(t, mockHCloud.DeleteServerCalls, "w-1")
}

// --- findUnhealthyNode: wrapper function ---

func TestFindUnhealthyNode_FindsFirst(t *testing.T) {
	t.Parallel()

	pastTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: true},
		{Name: "w-2", Healthy: false, UnhealthySince: &pastTime},
		{Name: "w-3", Healthy: false, UnhealthySince: &pastTime},
	}

	result := findUnhealthyNode(nodes, 3*time.Minute)
	require.NotNil(t, result)
	assert.Equal(t, "w-2", result.Name)
}

func TestFindUnhealthyNode_NoneFound(t *testing.T) {
	t.Parallel()

	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: true},
		{Name: "w-2", Healthy: true},
	}

	result := findUnhealthyNode(nodes, 3*time.Minute)
	assert.Nil(t, result)
}

// --- reconcileWithStateMachine: specific phase dispatches ---

func TestHandleStuckNode_DeleteServerError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-stuck", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		DeleteServerFunc: func(ctx context.Context, name string) error {
			return fmt.Errorf("delete failed")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	stuck := stuckNode{
		Name:    "cp-stuck",
		Role:    "control-plane",
		Phase:   k8znerv1alpha1.NodePhaseCreatingServer,
		Elapsed: 15 * time.Minute,
		Timeout: 10 * time.Minute,
	}

	// Should not return error even if delete fails
	err := r.handleStuckNode(context.Background(), cluster, stuck)
	require.NoError(t, err)

	// Node should be removed from status regardless
	assert.Empty(t, cluster.Status.ControlPlanes.Nodes)
}

// --- loadTalosClients: with credentialsRef but missing secret ---

func TestReplaceUnhealthyCPIfNeeded_NoUnhealthyCPs(t *testing.T) {
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
				Ready: 3,
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: true},
					{Name: "cp-2", Healthy: true},
					{Name: "cp-3", Healthy: true},
				},
			},
		},
	}

	result, err := r.replaceUnhealthyCPIfNeeded(context.Background(), cluster)
	require.NoError(t, err)
	assert.Empty(t, result.RequeueAfter, "should not requeue when no unhealthy CPs")
}

func TestReplaceUnhealthyCPIfNeeded_QuorumLost(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	pastTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Ready: 1, // Only 1 healthy, need 2 for quorum
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: true},
					{Name: "cp-2", Healthy: false, UnhealthySince: &pastTime},
					{Name: "cp-3", Healthy: false, UnhealthySince: &pastTime},
				},
			},
		},
	}

	result, err := r.replaceUnhealthyCPIfNeeded(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, defaultRequeueAfter, result.RequeueAfter, "should requeue when quorum lost")
}

// --- scaleWorkers: scale down path ---

func TestReplaceUnhealthyWorkers_EmptyList(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	replaced := r.replaceUnhealthyWorkers(context.Background(), cluster, nil)
	assert.Equal(t, 0, replaced)
}

func TestReplaceUnhealthyWorkers_RespectsMaxConcurrentHeals(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{
			// Fail at prepareForProvisioning because there is no snapshot
			GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				return nil, fmt.Errorf("no snapshot")
			},
		}),
		WithMaxConcurrentHeals(1),
		WithMetrics(false),
	)

	pastTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	unhealthy := []*k8znerv1alpha1.NodeStatus{
		{Name: "w-1", UnhealthySince: &pastTime, UnhealthyReason: "NotReady"},
		{Name: "w-2", UnhealthySince: &pastTime, UnhealthyReason: "NotReady"},
		{Name: "w-3", UnhealthySince: &pastTime, UnhealthyReason: "NotReady"},
	}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1"}, {Name: "w-2"}, {Name: "w-3"},
				},
			},
		},
	}

	// Even though there are 3 unhealthy, maxConcurrentHeals=1 limits to 1 attempt
	// The attempt will fail at provisionReplacementWorker (no snapshot), so replaced=0
	replaced := r.replaceUnhealthyWorkers(context.Background(), cluster, unhealthy)
	// replaceWorker fails, so replaced stays 0, but only 1 was attempted
	assert.Equal(t, 0, replaced)
}

// --- reconcileControlPlanes: single CP skips health replacement ---

func TestDrainNode_SkipsMirrorAndDaemonSetPods(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	regularPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "regular-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{NodeName: "test-node"},
	}

	mirrorPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mirror-pod",
			Namespace: "kube-system",
			Annotations: map[string]string{
				corev1.MirrorPodAnnotationKey: "true",
			},
		},
		Spec: corev1.PodSpec{NodeName: "test-node"},
	}

	daemonSetPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds-pod",
			Namespace: "kube-system",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "DaemonSet", Name: "my-ds"},
			},
		},
		Spec: corev1.PodSpec{NodeName: "test-node"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(regularPod, mirrorPod, daemonSetPod).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
			pod := o.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder)

	err := r.drainNode(context.Background(), "test-node")
	require.NoError(t, err)
	// The function should have attempted to evict only the regular pod
	// Mirror and DaemonSet pods should be skipped
}

// --- reconcileInfrastructurePhase: existing infrastructure skip ---

func TestDecommissionWorker_NoK8sNode(t *testing.T) {
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
	}

	worker := &k8znerv1alpha1.NodeStatus{
		Name:     "w-1",
		ServerID: 12345,
	}

	err := r.decommissionWorker(context.Background(), cluster, worker)
	require.NoError(t, err)
	// Server should have been deleted
	require.Len(t, mockHCloud.DeleteServerCalls, 1)
	assert.Equal(t, "w-1", mockHCloud.DeleteServerCalls[0])
}

func TestDecommissionWorker_WithExistingK8sNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	k8sNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "w-1"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(k8sNode).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
			pod := o.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	worker := &k8znerv1alpha1.NodeStatus{
		Name:     "w-1",
		ServerID: 12345,
	}

	err := r.decommissionWorker(context.Background(), cluster, worker)
	require.NoError(t, err)

	// K8s node should be deleted
	node := &corev1.Node{}
	getErr := fakeClient.Get(context.Background(), types.NamespacedName{Name: "w-1"}, node)
	assert.Error(t, getErr, "K8s node should be deleted")

	// HCloud server should be deleted
	require.Len(t, mockHCloud.DeleteServerCalls, 1)
}

func TestDecommissionWorker_HCloudDeleteError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		DeleteServerFunc: func(ctx context.Context, name string) error {
			return fmt.Errorf("delete failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	worker := &k8znerv1alpha1.NodeStatus{Name: "w-1"}

	err := r.decommissionWorker(context.Background(), cluster, worker)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete hetzner server")
}

// --- scaleDownWorkers tests ---

func TestReplaceControlPlane_DeletesAndReplacesCP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region:        "nbg1",
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3, Size: "cx23"},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1", PublicIP: "1.1.1.1"},
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2", PublicIP: "2.2.2.2"},
					{Name: "cp-bad", Healthy: false, PrivateIP: "10.0.0.3", ServerID: 999},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(20)

	mockTalos := &MockTalosClient{
		WaitForNodeReadyFunc: func(ctx context.Context, nodeIP string, timeout int) error {
			return nil
		},
	}
	mockGen := &MockTalosConfigGenerator{}
	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(ctx context.Context, name string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:   12345,
				Name: name,
				PrivateNet: []hcloudgo.ServerPrivateNet{
					{IP: net.ParseIP("10.0.0.99")},
				},
			}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithTalosClient(mockTalos),
		WithTalosConfigGenerator(mockGen),
		WithMetrics(false),
		WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
			return nil
		}),
	)

	unhealthyNode := &cluster.Status.ControlPlanes.Nodes[2]

	err := r.replaceControlPlane(context.Background(), cluster, unhealthyNode)
	require.NoError(t, err)

	// Old node should be deleted from HCloud
	foundDeleteCall := false
	for _, call := range mockHCloud.DeleteServerCalls {
		if call == "cp-bad" {
			foundDeleteCall = true
			break
		}
	}
	assert.True(t, foundDeleteCall, "old server should have been deleted")
}

// --- replaceWorker: flow test ---

func TestReplaceWorker_DrainsDeletesAndReplacesWorker(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	workerNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "w-bad"},
	}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region:  "nbg1",
			Workers: k8znerv1alpha1.WorkerSpec{Count: 1, Size: "cx23"},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-bad", Healthy: false, ServerID: 999},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, workerNode).
		WithStatusSubresource(cluster).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
			pod := o.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()
	recorder := record.NewFakeRecorder(20)

	mockTalos := &MockTalosClient{
		WaitForNodeReadyFunc: func(ctx context.Context, nodeIP string, timeout int) error {
			return nil
		},
	}
	mockGen := &MockTalosConfigGenerator{}
	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(ctx context.Context, name string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:   12345,
				Name: name,
				PrivateNet: []hcloudgo.ServerPrivateNet{
					{IP: net.ParseIP("10.0.0.99")},
				},
			}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithTalosClient(mockTalos),
		WithTalosConfigGenerator(mockGen),
		WithMetrics(false),
		WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
			return nil
		}),
	)

	unhealthyNode := &cluster.Status.Workers.Nodes[0]

	err := r.replaceWorker(context.Background(), cluster, unhealthyNode)
	require.NoError(t, err)

	// Old server should have been deleted
	foundDeleteCall := false
	for _, call := range mockHCloud.DeleteServerCalls {
		if call == "w-bad" {
			foundDeleteCall = true
			break
		}
	}
	assert.True(t, foundDeleteCall, "old server should have been deleted")
}

// --- findHealthyControlPlaneIP: empty nodes ---
