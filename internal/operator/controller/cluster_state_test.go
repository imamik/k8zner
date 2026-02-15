package controller

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

func TestFindHealthyControlPlaneIP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("returns empty for no nodes", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: nil,
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		ip := r.findHealthyControlPlaneIP(cluster)
		assert.Equal(t, "", ip)
	})

	t.Run("returns empty when all unhealthy", func(t *testing.T) {
		t.Parallel()
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

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		ip := r.findHealthyControlPlaneIP(cluster)
		assert.Equal(t, "", ip)
	})

	t.Run("returns first healthy CP IP", func(t *testing.T) {
		t.Parallel()
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

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		ip := r.findHealthyControlPlaneIP(cluster)
		assert.Equal(t, "10.0.0.2", ip)
	})

	t.Run("skips healthy node without private IP", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, PrivateIP: ""}, // No private IP
						{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		ip := r.findHealthyControlPlaneIP(cluster)
		assert.Equal(t, "10.0.0.2", ip)
	})
}

func TestBuildClusterSANs(t *testing.T) {
	t.Parallel()

	t.Run("uses ControlPlaneEndpoint first", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"k8zner.io/control-plane-endpoint": "10.0.0.99",
				},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlaneEndpoint: "1.2.3.4",
				Infrastructure: k8znerv1alpha1.InfrastructureStatus{
					LoadBalancerIP: "5.6.7.8",
				},
			},
		}

		sans := buildClusterSANs(cluster)
		require.Len(t, sans, 1)
		assert.Equal(t, "1.2.3.4", sans[0]) // Endpoint takes priority
	})

	t.Run("falls back to LB IP when no endpoint", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Infrastructure: k8znerv1alpha1.InfrastructureStatus{
					LoadBalancerIP: "5.6.7.8",
				},
			},
		}

		sans := buildClusterSANs(cluster)
		require.Len(t, sans, 1)
		assert.Equal(t, "5.6.7.8", sans[0])
	})

	t.Run("falls back to annotation when no endpoint or LB", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"k8zner.io/control-plane-endpoint": "10.0.0.99",
				},
			},
		}

		sans := buildClusterSANs(cluster)
		require.Len(t, sans, 1)
		assert.Equal(t, "10.0.0.99", sans[0])
	})

	t.Run("includes CP node IPs", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlaneEndpoint: "1.2.3.4",
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", PrivateIP: "10.0.0.1", PublicIP: "203.0.113.1"},
						{Name: "cp-2", PrivateIP: "10.0.0.2", PublicIP: "203.0.113.2"},
					},
				},
			},
		}

		sans := buildClusterSANs(cluster)
		assert.Len(t, sans, 5) // endpoint + 2*private + 2*public
		assert.Contains(t, sans, "1.2.3.4")
		assert.Contains(t, sans, "10.0.0.1")
		assert.Contains(t, sans, "203.0.113.1")
		assert.Contains(t, sans, "10.0.0.2")
		assert.Contains(t, sans, "203.0.113.2")
	})

	t.Run("skips empty node IPs", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", PrivateIP: "10.0.0.1", PublicIP: ""},
						{Name: "cp-2", PrivateIP: "", PublicIP: ""},
					},
				},
			},
		}

		sans := buildClusterSANs(cluster)
		require.Len(t, sans, 1)
		assert.Equal(t, "10.0.0.1", sans[0])
	})

	t.Run("returns empty for minimal cluster", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{}

		sans := buildClusterSANs(cluster)
		assert.Empty(t, sans)
	})
}

func TestResolveSSHKeyIDs(t *testing.T) {
	t.Parallel()

	t.Run("returns annotation keys split by comma", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"k8zner.io/ssh-keys": "key-a,key-b,key-c",
				},
			},
		}

		keys := resolveSSHKeyIDs(cluster)
		assert.Equal(t, []string{"key-a", "key-b", "key-c"}, keys)
	})

	t.Run("returns single annotation key", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"k8zner.io/ssh-keys": "my-key",
				},
			},
		}

		keys := resolveSSHKeyIDs(cluster)
		assert.Equal(t, []string{"my-key"}, keys)
	})

	t.Run("returns default key when no annotation", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "prod-cluster",
			},
		}

		keys := resolveSSHKeyIDs(cluster)
		assert.Equal(t, []string{"prod-cluster-key"}, keys)
	})

	t.Run("returns default key when annotations exist but no ssh-keys", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster",
				Annotations: map[string]string{
					"some-other-annotation": "value",
				},
			},
		}

		keys := resolveSSHKeyIDs(cluster)
		assert.Equal(t, []string{"test-cluster-key"}, keys)
	})
}

func TestResolveControlPlaneIP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("returns status endpoint first", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"k8zner.io/control-plane-endpoint": "10.0.0.99",
				},
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlaneEndpoint: "1.2.3.4",
				Infrastructure: k8znerv1alpha1.InfrastructureStatus{
					LoadBalancerIP: "5.6.7.8",
				},
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1"},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		ip := r.resolveControlPlaneIP(cluster)
		assert.Equal(t, "1.2.3.4", ip)
	})

	t.Run("returns LB IP when no endpoint", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Infrastructure: k8znerv1alpha1.InfrastructureStatus{
					LoadBalancerIP: "5.6.7.8",
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		ip := r.resolveControlPlaneIP(cluster)
		assert.Equal(t, "5.6.7.8", ip)
	})

	t.Run("returns annotation when no endpoint or LB", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"k8zner.io/control-plane-endpoint": "10.0.0.99",
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		ip := r.resolveControlPlaneIP(cluster)
		assert.Equal(t, "10.0.0.99", ip)
	})

	t.Run("falls back to healthy CP IP", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1"},
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		ip := r.resolveControlPlaneIP(cluster)
		assert.Equal(t, "10.0.0.1", ip)
	})

	t.Run("returns empty when no sources available", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		ip := r.resolveControlPlaneIP(cluster)
		assert.Equal(t, "", ip)
	})
}

func TestResolveNetworkIDForState(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("returns cached network ID from status", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Infrastructure: k8znerv1alpha1.InfrastructureStatus{
					NetworkID: 12345,
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{}
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		id, err := r.resolveNetworkIDForState(context.Background(), cluster)
		require.NoError(t, err)
		assert.Equal(t, int64(12345), id)
		// Should not call HCloud API since cached
		assert.Empty(t, mockHCloud.GetNetworkCalls)
	})

	t.Run("queries HCloud when not cached", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
				assert.Equal(t, "test-cluster-network", name)
				return &hcloudgo.Network{ID: 999}, nil
			},
		}
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		id, err := r.resolveNetworkIDForState(context.Background(), cluster)
		require.NoError(t, err)
		assert.Equal(t, int64(999), id)
		assert.Len(t, mockHCloud.GetNetworkCalls, 1)
	})

	t.Run("returns zero when network not found", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
				return nil, nil
			},
		}
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		id, err := r.resolveNetworkIDForState(context.Background(), cluster)
		require.NoError(t, err)
		assert.Equal(t, int64(0), id)
	})

	t.Run("returns error on HCloud failure", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
				return nil, fmt.Errorf("api error")
			},
		}
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		_, err := r.resolveNetworkIDForState(context.Background(), cluster)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get network")
	})
}

func TestBuildClusterState_FullIntegration(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("builds complete state", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prod",
				Namespace: "default",
			},
			Spec: k8znerv1alpha1.K8znerClusterSpec{
				Region: "nbg1",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlaneEndpoint: "1.2.3.4",
				Infrastructure: k8znerv1alpha1.InfrastructureStatus{
					NetworkID: 100,
				},
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true, PrivateIP: "10.0.0.1", PublicIP: "203.0.113.1"},
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
		)

		state, err := r.buildClusterState(context.Background(), cluster)
		require.NoError(t, err)

		assert.Equal(t, "prod", state.Name)
		assert.Equal(t, "nbg1", state.Region)
		assert.Equal(t, int64(100), state.NetworkID)
		assert.Contains(t, state.SANs, "1.2.3.4")
		assert.Contains(t, state.SANs, "10.0.0.1")
		assert.Contains(t, state.SSHKeyIDs, "prod-key")
		assert.Equal(t, "1.2.3.4", state.ControlPlaneIP)
	})

	t.Run("returns error when network lookup fails", func(t *testing.T) {
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
			GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
				return nil, fmt.Errorf("network error")
			},
		}
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		_, err := r.buildClusterState(context.Background(), cluster)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get network")
	})
}

func TestWaitForServerIP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("returns immediately when IP already assigned", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				return "10.0.0.5", nil
			},
		}
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		ip, err := r.waitForServerIP(context.Background(), "test-server", 10*time.Second)
		require.NoError(t, err)
		assert.Equal(t, "10.0.0.5", ip)
	})

	t.Run("returns IP after retry", func(t *testing.T) {
		t.Parallel()
		calls := 0
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				calls++
				if calls <= 2 {
					return "", nil // Not assigned yet
				}
				return "10.0.0.5", nil
			},
		}
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		ip, err := r.waitForServerIP(context.Background(), "test-server", 30*time.Second)
		require.NoError(t, err)
		assert.Equal(t, "10.0.0.5", ip)
	})

	t.Run("returns error on timeout", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		mockHCloud := &MockHCloudClient{
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				return "", nil // Never assigned
			},
		}
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
		)

		_, err := r.waitForServerIP(context.Background(), "test-server", 100*time.Millisecond)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timeout")
	})
}

func TestWaitForK8sNodeReady(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	t.Run("returns error on timeout when node not found", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		// Use very short timeout - the function has a 10s initial sleep
		// so we use context cancellation instead
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := r.waitForK8sNodeReady(ctx, "nonexistent-node", 100*time.Millisecond)
		require.Error(t, err)
	})

	t.Run("returns nil when node becomes ready", func(t *testing.T) {
		t.Parallel()
		readyNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ready-node",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		}

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(readyNode).
			Build()
		recorder := record.NewFakeRecorder(10)
		r := NewClusterReconciler(client, scheme, recorder)

		// This test will take ~15s due to the 10s sleep + 5s poll interval
		// Use a short context to avoid the 10s initial sleep blocking
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		err := r.waitForK8sNodeReady(ctx, "ready-node", 20*time.Second)
		require.NoError(t, err)
	})
}

func TestGetPrivateIPFromServer_NilIPInPrivateNet(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:   123,
				Name: "test-server",
				PrivateNet: []hcloudgo.ServerPrivateNet{
					{IP: nil}, // nil IP
				},
			}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	ip, err := r.getPrivateIPFromServer(context.Background(), "test-server")
	require.NoError(t, err)
	assert.Equal(t, "", ip, "nil IP should return empty string")
}

// --- WithMaxConcurrentHeals option test ---

func TestFindTalosEndpoint_NilBootstrapWithHealthyCPNodes(t *testing.T) {
	t.Parallel()

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			// Bootstrap is nil
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Healthy: true, PublicIP: "9.9.9.9"},
				},
			},
		},
	}

	r := &ClusterReconciler{}
	endpoint := r.findTalosEndpoint(cluster)
	assert.Equal(t, "9.9.9.9", endpoint)
}

// --- reconcileLegacy: health check failure ---

func TestFindHealthyControlPlaneIP_HealthyWithEmptyIP(t *testing.T) {
	t.Parallel()

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: true, PrivateIP: ""}, // healthy but no IP
				},
			},
		},
	}

	r := &ClusterReconciler{}
	ip := r.findHealthyControlPlaneIP(cluster)
	assert.Equal(t, "", ip)
}

// --- waitForServerIP: context cancelled ---

func TestBuildClusterSANs_ControlPlaneEndpoint(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "cp.example.com",
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP: "1.2.3.4", // should be ignored since ControlPlaneEndpoint is set
			},
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{PrivateIP: "10.0.0.1", PublicIP: "5.5.5.5"},
				},
			},
		},
	}

	sans := buildClusterSANs(cluster)
	require.Len(t, sans, 3) // endpoint + privateIP + publicIP
	assert.Equal(t, "cp.example.com", sans[0])
	assert.Equal(t, "10.0.0.1", sans[1])
	assert.Equal(t, "5.5.5.5", sans[2])
}

func TestBuildClusterSANs_LoadBalancerIP(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "", // empty, fall through to LB IP
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP: "1.2.3.4",
			},
		},
	}

	sans := buildClusterSANs(cluster)
	require.Len(t, sans, 1)
	assert.Equal(t, "1.2.3.4", sans[0])
}

func TestBuildClusterSANs_Annotation(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"k8zner.io/control-plane-endpoint": "annotated.example.com",
			},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP: "", // empty, fall through to annotation
			},
		},
	}

	sans := buildClusterSANs(cluster)
	require.Len(t, sans, 1)
	assert.Equal(t, "annotated.example.com", sans[0])
}

func TestBuildClusterSANs_NoEndpoint(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure:       k8znerv1alpha1.InfrastructureStatus{},
		},
	}

	sans := buildClusterSANs(cluster)
	assert.Empty(t, sans)
}

func TestBuildClusterSANs_MultipleNodesWithMixedIPs(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "api.cluster.local",
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{PrivateIP: "10.0.0.1", PublicIP: "1.1.1.1"},
					{PrivateIP: "10.0.0.2", PublicIP: ""}, // no public IP
					{PrivateIP: "", PublicIP: "3.3.3.3"},  // no private IP
				},
			},
		},
	}

	sans := buildClusterSANs(cluster)
	// endpoint + (privIP1 + pubIP1) + (privIP2) + (pubIP3) = 5
	require.Len(t, sans, 5)
	assert.Equal(t, "api.cluster.local", sans[0])
	assert.Equal(t, "10.0.0.1", sans[1])
	assert.Equal(t, "1.1.1.1", sans[2])
	assert.Equal(t, "10.0.0.2", sans[3])
	assert.Equal(t, "3.3.3.3", sans[4])
}

func TestBuildClusterSANs_AnnotationWithNilAnnotationsMap(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		// No annotations set (nil map)
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure:       k8znerv1alpha1.InfrastructureStatus{},
		},
	}

	sans := buildClusterSANs(cluster)
	assert.Empty(t, sans)
}

// --- resolveSSHKeyIDs: all branches ---

func TestResolveSSHKeyIDs_FromAnnotation(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-cluster",
			Annotations: map[string]string{
				"k8zner.io/ssh-keys": "key1,key2,key3",
			},
		},
	}

	keys := resolveSSHKeyIDs(cluster)
	require.Len(t, keys, 3)
	assert.Equal(t, "key1", keys[0])
	assert.Equal(t, "key2", keys[1])
	assert.Equal(t, "key3", keys[2])
}

func TestResolveSSHKeyIDs_DefaultNaming(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-cluster",
		},
	}

	keys := resolveSSHKeyIDs(cluster)
	require.Len(t, keys, 1)
	assert.Equal(t, "my-cluster-key", keys[0])
}

func TestResolveSSHKeyIDs_EmptyAnnotationsMap(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-cluster",
			Annotations: map[string]string{}, // empty map, no ssh-keys key
		},
	}

	keys := resolveSSHKeyIDs(cluster)
	require.Len(t, keys, 1)
	assert.Equal(t, "my-cluster-key", keys[0])
}

func TestResolveSSHKeyIDs_SingleKey(t *testing.T) {
	t.Parallel()
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "prod-cluster",
			Annotations: map[string]string{
				"k8zner.io/ssh-keys": "single-key",
			},
		},
	}

	keys := resolveSSHKeyIDs(cluster)
	require.Len(t, keys, 1)
	assert.Equal(t, "single-key", keys[0])
}

// --- resolveControlPlaneIP: all fallback paths ---

func TestResolveControlPlaneIP_FromStatus(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "status-endpoint.example.com",
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP: "1.2.3.4", // should be ignored
			},
		},
	}

	ip := r.resolveControlPlaneIP(cluster)
	assert.Equal(t, "status-endpoint.example.com", ip)
}

func TestResolveControlPlaneIP_FromLoadBalancer(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "", // empty
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerIP: "1.2.3.4",
			},
		},
	}

	ip := r.resolveControlPlaneIP(cluster)
	assert.Equal(t, "1.2.3.4", ip)
}

func TestResolveControlPlaneIP_FromAnnotation(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"k8zner.io/control-plane-endpoint": "annotation-endpoint.example.com",
			},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure:       k8znerv1alpha1.InfrastructureStatus{},
		},
	}

	ip := r.resolveControlPlaneIP(cluster)
	assert.Equal(t, "annotation-endpoint.example.com", ip)
}

func TestResolveControlPlaneIP_FromHealthyCPNode(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure:       k8znerv1alpha1.InfrastructureStatus{},
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PrivateIP: "10.0.0.1"}, // unhealthy
					{Name: "cp-2", Healthy: true, PrivateIP: "10.0.0.2"},  // healthy
				},
			},
		},
	}

	ip := r.resolveControlPlaneIP(cluster)
	assert.Equal(t, "10.0.0.2", ip)
}

func TestResolveControlPlaneIP_NoneAvailable(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "",
			Infrastructure:       k8znerv1alpha1.InfrastructureStatus{},
		},
	}

	ip := r.resolveControlPlaneIP(cluster)
	assert.Equal(t, "", ip)
}

// --- generateReplacementServerName: all roles ---

func TestGenerateReplacementServerName_ControlPlane(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cluster"},
	}

	name := r.generateReplacementServerName(cluster, "control-plane", "old-cp-1")
	assert.Contains(t, name, "my-cluster-cp-")
	assert.Len(t, name, len("my-cluster-cp-")+5) // 5-char random ID
}

func TestGenerateReplacementServerName_Worker(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cluster"},
	}

	name := r.generateReplacementServerName(cluster, "worker", "old-worker-1")
	assert.Contains(t, name, "my-cluster-w-")
	assert.Len(t, name, len("my-cluster-w-")+5) // 5-char random ID
}

func TestGenerateReplacementServerName_UnknownRole(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cluster"},
	}

	name := r.generateReplacementServerName(cluster, "gateway", "old-gw-1")
	assert.Contains(t, name, "my-cluster-ga-")
	assert.Len(t, name, len("my-cluster-ga-")+5)
}

// --- resolveNetworkIDForState: all branches ---

func TestResolveNetworkIDForState_FromStatus(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID: 42,
			},
		},
	}

	networkID, err := r.resolveNetworkIDForState(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, int64(42), networkID)
	// Should NOT have called GetNetwork since status already had it
	assert.Empty(t, mockHCloud.GetNetworkCalls)
}

func TestResolveNetworkIDForState_FromHCloud(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			assert.Equal(t, "test-cluster-network", name)
			return &hcloudgo.Network{ID: 99}, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID: 0, // zero, so will look up via HCloud
			},
		},
	}

	networkID, err := r.resolveNetworkIDForState(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, int64(99), networkID)
}

func TestResolveNetworkIDForState_HCloudError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return nil, fmt.Errorf("network API error")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status:     k8znerv1alpha1.K8znerClusterStatus{},
	}

	_, err := r.resolveNetworkIDForState(context.Background(), cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get network")
}

func TestResolveNetworkIDForState_NetworkNotFound(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return nil, nil // not found
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status:     k8znerv1alpha1.K8znerClusterStatus{},
	}

	networkID, err := r.resolveNetworkIDForState(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, int64(0), networkID)
}

// --- removeFromEtcd: all branches ---

func TestBuildClusterState_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return &hcloudgo.Network{ID: 42}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region: "nbg1",
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "api.example.com",
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Healthy: true, PrivateIP: "10.0.0.1", PublicIP: "1.1.1.1"},
				},
			},
		},
	}

	state, err := r.buildClusterState(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, "test-cluster", state.Name)
	assert.Equal(t, "nbg1", state.Region)
	assert.Equal(t, int64(42), state.NetworkID)
	assert.Contains(t, state.SANs, "api.example.com")
	assert.Contains(t, state.SANs, "10.0.0.1")
	assert.Contains(t, state.SANs, "1.1.1.1")
	assert.Equal(t, "api.example.com", state.ControlPlaneIP)
	assert.Equal(t, "test-cluster", state.Labels["cluster"])
}

func TestBuildClusterState_NetworkError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return nil, fmt.Errorf("network error")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec:       k8znerv1alpha1.K8znerClusterSpec{Region: "nbg1"},
	}

	_, err := r.buildClusterState(context.Background(), cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get network")
}

// --- getPrivateIPFromServer: all branches ---

func TestGetPrivateIPFromServer_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:   123,
				Name: "test-server",
				PrivateNet: []hcloudgo.ServerPrivateNet{
					{IP: net.ParseIP("10.0.0.5")},
				},
			}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	ip, err := r.getPrivateIPFromServer(context.Background(), "test-server")
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.5", ip)
}

func TestGetPrivateIPFromServer_ServerNotFound(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			return nil, nil // not found
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	_, err := r.getPrivateIPFromServer(context.Background(), "missing-server")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server missing-server not found")
}

func TestGetPrivateIPFromServer_APIError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			return nil, fmt.Errorf("API error")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	_, err := r.getPrivateIPFromServer(context.Background(), "test-server")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}

func TestGetPrivateIPFromServer_NoPrivateNet(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerByNameFunc: func(_ context.Context, _ string) (*hcloudgo.Server, error) {
			return &hcloudgo.Server{
				ID:         123,
				Name:       "test-server",
				PrivateNet: []hcloudgo.ServerPrivateNet{}, // empty
			}, nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	ip, err := r.getPrivateIPFromServer(context.Background(), "test-server")
	require.NoError(t, err)
	assert.Equal(t, "", ip)
}

// --- drainAndDeleteWorker: all branches ---

func TestFindTalosEndpoint_FromControlPlaneEndpoint(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "api.example.com",
		},
	}

	assert.Equal(t, "api.example.com", r.findTalosEndpoint(cluster))
}

func TestFindTalosEndpoint_FromBootstrapPublicIP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				PublicIP: "1.2.3.4",
			},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlaneEndpoint: "", // empty - fall through
		},
	}

	assert.Equal(t, "1.2.3.4", r.findTalosEndpoint(cluster))
}

func TestFindTalosEndpoint_NoEndpointNoBootstrapNoHealthyCPReturnsEmpty(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Healthy: false, PublicIP: "5.5.5.5"},
					{Name: "cp-2", Healthy: true, PublicIP: ""}, // healthy but no IP
				},
			},
		},
	}

	assert.Equal(t, "", r.findTalosEndpoint(cluster))
}

// --- ensureWorkersReady tests ---

func TestResolveNetworkID_FromStatus(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID: 12345,
			},
		},
	}

	networkID, err := r.resolveNetworkID(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), networkID)
}

func TestResolveNetworkID_FromHCloud(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return &hcloudgo.Network{ID: 99999, Name: name}, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID: 0, // not set - should look up from HCloud
			},
		},
	}

	networkID, err := r.resolveNetworkID(context.Background(), cluster)
	require.NoError(t, err)
	assert.Equal(t, int64(99999), networkID)
	// Should also update the status
	assert.Equal(t, int64(99999), cluster.Status.Infrastructure.NetworkID)
}

func TestResolveNetworkID_HCloudError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return nil, fmt.Errorf("api error")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	_, err := r.resolveNetworkID(context.Background(), cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api error")
}

func TestResolveNetworkID_NetworkNotFound(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetNetworkFunc: func(ctx context.Context, name string) (*hcloudgo.Network, error) {
			return nil, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	_, err := r.resolveNetworkID(context.Background(), cluster)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network not found")
}

// --- replaceUnhealthyCPIfNeeded tests ---

func TestDiscoverLoadBalancerInfo_AlreadyDiscovered(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				LoadBalancerID: 999,
				LoadBalancerIP: "1.2.3.4",
			},
		},
	}

	// Should return immediately without making any API calls
	r.discoverLoadBalancerInfo(context.Background(), cluster, "fake-token")
	assert.Equal(t, int64(999), cluster.Status.Infrastructure.LoadBalancerID)
	assert.Equal(t, "1.2.3.4", cluster.Status.Infrastructure.LoadBalancerIP)
}

// --- prepareForProvisioning: snapshot error ---

func TestFindHealthyControlPlaneIP_EmptyNodes(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	ip := r.findHealthyControlPlaneIP(cluster)
	assert.Empty(t, ip)
}

// --- updateClusterPhase: provisioning phase preserved ---
