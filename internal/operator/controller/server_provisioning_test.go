package controller

import (
	"context"
	"fmt"
	"net"
	"testing"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/platform/hcloud"
)

func TestLoadTalosClients(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	t.Run("returns injected mocks directly", func(t *testing.T) {
		t.Parallel()
		mockTalos := &MockTalosClient{}
		mockGen := &MockTalosConfigGenerator{}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(&MockHCloudClient{}),
			WithTalosClient(mockTalos),
			WithTalosConfigGenerator(mockGen),
			WithMetrics(false),
		)

		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		}

		tc := r.loadTalosClients(context.Background(), cluster)
		assert.Equal(t, mockTalos, tc.client)
		assert.Equal(t, mockGen, tc.configGen)
	})

	t.Run("returns nil clients when no credentials ref", func(t *testing.T) {
		t.Parallel()
		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(&MockHCloudClient{}),
			WithMetrics(false),
		)

		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		}

		tc := r.loadTalosClients(context.Background(), cluster)
		assert.Nil(t, tc.client)
		assert.Nil(t, tc.configGen)
	})
}

func TestGetSnapshot(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	t.Run("returns snapshot on success", func(t *testing.T) {
		t.Parallel()
		expectedSnapshot := &hcloudgo.Image{ID: 42, Name: "talos-v1.9"}
		mockHCloud := &MockHCloudClient{
			GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				assert.Equal(t, map[string]string{"os": "talos"}, labels)
				return expectedSnapshot, nil
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		snapshot, err := r.getSnapshot(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(42), snapshot.ID)
	})

	t.Run("returns error when no snapshot found", func(t *testing.T) {
		t.Parallel()
		mockHCloud := &MockHCloudClient{
			GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				return nil, nil
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		_, err := r.getSnapshot(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no Talos snapshot found")
	})

	t.Run("returns error on API failure", func(t *testing.T) {
		t.Parallel()
		mockHCloud := &MockHCloudClient{
			GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				return nil, fmt.Errorf("API error")
			},
		}

		r := NewClusterReconciler(client, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		_, err := r.getSnapshot(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get Talos snapshot")
	})
}

func TestCreateEphemeralSSHKey(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cluster"},
	}

	t.Run("creates key and returns cleanup function", func(t *testing.T) {
		t.Parallel()
		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(k8sClient, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		keyName, cleanup, err := r.createEphemeralSSHKey(context.Background(), cluster, "control-plane")
		require.NoError(t, err)
		assert.Contains(t, keyName, "ephemeral-my-cluster-control-plane-")
		assert.NotNil(t, cleanup)

		// Verify SSH key was created
		assert.Len(t, mockHCloud.CreateSSHKeyCalls, 1)
		assert.Equal(t, keyName, mockHCloud.CreateSSHKeyCalls[0].Name)
		assert.Equal(t, "my-cluster", mockHCloud.CreateSSHKeyCalls[0].Labels["cluster"])
		assert.Equal(t, "ephemeral-control-plane", mockHCloud.CreateSSHKeyCalls[0].Labels["type"])

		// Call cleanup and verify key deletion
		cleanup()
		assert.Len(t, mockHCloud.DeleteSSHKeyCalls, 1)
		assert.Equal(t, keyName, mockHCloud.DeleteSSHKeyCalls[0])
	})

	t.Run("returns error on SSH key creation failure", func(t *testing.T) {
		t.Parallel()
		mockHCloud := &MockHCloudClient{
			CreateSSHKeyFunc: func(ctx context.Context, name, publicKey string, labels map[string]string) (string, error) {
				return "", fmt.Errorf("SSH key limit reached")
			},
		}

		r := NewClusterReconciler(k8sClient, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		_, cleanup, err := r.createEphemeralSSHKey(context.Background(), cluster, "worker")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create ephemeral SSH key")
		assert.Nil(t, cleanup)
	})
}

func TestProvisionServer(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	recorder := record.NewFakeRecorder(10)

	t.Run("successful provisioning with private IP", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()

		mockHCloud := &MockHCloudClient{
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				return "1.2.3.4", nil
			},
			GetServerIDFunc: func(ctx context.Context, name string) (string, error) {
				return "99999", nil
			},
			GetServerByNameFunc: func(ctx context.Context, name string) (*hcloudgo.Server, error) {
				return &hcloudgo.Server{
					ID:   99999,
					Name: name,
					PrivateNet: []hcloudgo.ServerPrivateNet{
						{IP: net.ParseIP("10.0.0.5")},
					},
				}, nil
			},
		}

		r := NewClusterReconciler(k8sClient, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		result, err := r.provisionServer(context.Background(), cluster, serverCreateOpts{
			Name:       "test-server",
			SnapshotID: 42,
			ServerType: "cx22",
			Region:     "nbg1",
			SSHKeyName: "ephemeral-key",
			Labels:     map[string]string{"role": "control-plane"},
			NetworkID:  1,
			Role:       "control-plane",
		})
		require.NoError(t, err)

		assert.Equal(t, "test-server", result.Name)
		assert.Equal(t, int64(99999), result.ServerID)
		assert.Equal(t, "1.2.3.4", result.PublicIP)
		assert.Equal(t, "10.0.0.5", result.PrivateIP)
		assert.Equal(t, "10.0.0.5", result.TalosIP) // Uses private IP when available

		// Verify server was created
		assert.Len(t, mockHCloud.CreateServerCalls, 1)
		assert.Equal(t, "test-server", mockHCloud.CreateServerCalls[0].Name)
		assert.Equal(t, "cx22", mockHCloud.CreateServerCalls[0].ServerType)
	})

	t.Run("server creation failure", func(t *testing.T) {
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

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()

		mockHCloud := &MockHCloudClient{
			CreateServerFunc: func(ctx context.Context, opts hcloud.ServerCreateOpts) (string, error) {
				return "", fmt.Errorf("resource limit exceeded")
			},
		}

		r := NewClusterReconciler(k8sClient, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		_, err := r.provisionServer(context.Background(), cluster, serverCreateOpts{
			Name:       "test-worker",
			SnapshotID: 42,
			ServerType: "cx22",
			Region:     "nbg1",
			SSHKeyName: "key",
			Labels:     map[string]string{"role": "worker"},
			Role:       "worker",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create server")
	})

	t.Run("falls back to public IP when no private IP", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()

		mockHCloud := &MockHCloudClient{
			GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
				return "5.6.7.8", nil
			},
			GetServerIDFunc: func(ctx context.Context, name string) (string, error) {
				return "11111", nil
			},
			GetServerByNameFunc: func(ctx context.Context, name string) (*hcloudgo.Server, error) {
				// No private network
				return nil, nil
			},
		}

		r := NewClusterReconciler(k8sClient, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		result, err := r.provisionServer(context.Background(), cluster, serverCreateOpts{
			Name:       "test-cp",
			SnapshotID: 42,
			ServerType: "cx22",
			Region:     "nbg1",
			SSHKeyName: "key",
			Labels:     map[string]string{"role": "control-plane"},
			Role:       "control-plane",
		})
		require.NoError(t, err)

		assert.Equal(t, "5.6.7.8", result.PublicIP)
		assert.Equal(t, "", result.PrivateIP)
		assert.Equal(t, "5.6.7.8", result.TalosIP) // Falls back to public IP
	})
}

func TestHandleProvisioningFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	recorder := record.NewFakeRecorder(10)

	t.Run("marks node failed, deletes server, removes from status", func(t *testing.T) {
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
					},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()

		mockHCloud := &MockHCloudClient{}

		r := NewClusterReconciler(k8sClient, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		r.handleProvisioningFailure(context.Background(), cluster, "worker", "worker-2", "IP timeout")

		// Verify server was deleted
		assert.Contains(t, mockHCloud.DeleteServerCalls, "worker-2")

		// Verify node was removed from status
		assert.Len(t, cluster.Status.Workers.Nodes, 1)
		assert.Equal(t, "worker-1", cluster.Status.Workers.Nodes[0].Name)
	})

	t.Run("handles delete server failure gracefully", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
					Nodes: []k8znerv1alpha1.NodeStatus{
						{Name: "cp-1", Healthy: true},
					},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster).
			WithStatusSubresource(cluster).
			Build()

		mockHCloud := &MockHCloudClient{
			DeleteServerFunc: func(ctx context.Context, name string) error {
				return fmt.Errorf("server not found")
			},
		}

		r := NewClusterReconciler(k8sClient, scheme, recorder,
			WithHCloudClient(mockHCloud),
			WithMetrics(false),
		)

		// Should not panic even if delete fails
		r.handleProvisioningFailure(context.Background(), cluster, "control-plane", "cp-1", "creation failed")

		assert.Contains(t, mockHCloud.DeleteServerCalls, "cp-1")
	})
}

func TestDiscoverLoadBalancerInfo(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	t.Run("skips when LB info already present", func(t *testing.T) {
		t.Parallel()
		cluster := &k8znerv1alpha1.K8znerCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
			Status: k8znerv1alpha1.K8znerClusterStatus{
				Infrastructure: k8znerv1alpha1.InfrastructureStatus{
					LoadBalancerID: 123,
					LoadBalancerIP: "1.2.3.4",
				},
			},
		}

		r := NewClusterReconciler(k8sClient, scheme, recorder,
			WithHCloudClient(&MockHCloudClient{}),
			WithMetrics(false),
		)

		r.discoverLoadBalancerInfo(context.Background(), cluster, "test-token")

		// Should not have changed
		assert.Equal(t, int64(123), cluster.Status.Infrastructure.LoadBalancerID)
		assert.Equal(t, "1.2.3.4", cluster.Status.Infrastructure.LoadBalancerIP)
	})
}
