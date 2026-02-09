package controller

import (
	"context"
	"fmt"
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
