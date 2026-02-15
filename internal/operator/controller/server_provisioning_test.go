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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	corev1 "k8s.io/api/core/v1"
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

func TestWaitForServerIP_ErrorOnInitialCallThenRetrySucceeds(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	calls := 0
	mockHCloud := &MockHCloudClient{
		GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
			calls++
			if calls == 1 {
				return "", fmt.Errorf("temporary error")
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
}

func TestWaitForServerIP_ErrorOnInitialCallEmptyOnRetryThenSuccess(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	calls := 0
	mockHCloud := &MockHCloudClient{
		GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
			calls++
			if calls == 1 {
				return "", fmt.Errorf("initial error")
			}
			if calls == 2 {
				return "", nil // empty, not yet assigned
			}
			if calls == 3 {
				return "", fmt.Errorf("transient error in ticker")
			}
			return "10.0.0.9", nil
		},
	}
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	ip, err := r.waitForServerIP(context.Background(), "test-server", 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.9", ip)
}

func TestWaitForServerIP_ContextCancelled(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{
		GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
			return "", fmt.Errorf("not found")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := r.waitForServerIP(ctx, "test-server", 30*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

// --- NewClusterReconciler: default nodeReadyWaiter ---

func TestConfigureCPNode_NoCredentials(t *testing.T) {
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
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	state := &clusterState{
		Name:   "test-cluster",
		SANs:   []string{"1.2.3.4"},
		Region: "nbg1",
	}

	// tc with nil configGen and nil client
	tc := talosClients{}

	result := &serverProvisionResult{
		Name:     "cp-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureCPNode(context.Background(), cluster, state, tc, result)
	require.NoError(t, err)

	// Node should be in WaitingForK8s phase
	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForK8s, cluster.Status.ControlPlanes.Nodes[0].Phase)
	assert.Contains(t, cluster.Status.ControlPlanes.Nodes[0].PhaseReason, "no Talos credentials")
}

// --- configureCPNode: config generation failure ---

func TestConfigureCPNode_ConfigGenerationFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	mockTalos := &MockTalosClient{}
	mockTalosGen := &MockTalosConfigGenerator{
		GenerateControlPlaneConfigFunc: func(sans []string, hostname string, serverID int64) ([]byte, error) {
			return nil, fmt.Errorf("config generation error")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	state := &clusterState{
		Name:   "test-cluster",
		SANs:   []string{"1.2.3.4"},
		Region: "nbg1",
	}

	tc := talosClients{configGen: mockTalosGen, client: mockTalos}

	result := &serverProvisionResult{
		Name:     "cp-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureCPNode(context.Background(), cluster, state, tc, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config generation error")
}

// --- configureCPNode: apply config failure ---

func TestConfigureCPNode_ApplyConfigFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	mockTalos := &MockTalosClient{
		ApplyConfigFunc: func(ctx context.Context, nodeIP string, config []byte) error {
			return fmt.Errorf("connection refused")
		},
	}
	mockTalosGen := &MockTalosConfigGenerator{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	state := &clusterState{SANs: []string{"1.2.3.4"}}
	tc := talosClients{configGen: mockTalosGen, client: mockTalos}

	result := &serverProvisionResult{
		Name:     "cp-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureCPNode(context.Background(), cluster, state, tc, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

// --- configureWorkerNode: no credentials path ---

func TestConfigureWorkerNode_NoCredentials(t *testing.T) {
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
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	// tc with nil configGen and nil client
	tc := talosClients{}

	result := &serverProvisionResult{
		Name:     "w-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureWorkerNode(context.Background(), cluster, tc, result)
	require.NoError(t, err)

	// Node should be in WaitingForK8s phase
	require.Len(t, cluster.Status.Workers.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForK8s, cluster.Status.Workers.Nodes[0].Phase)
	assert.Contains(t, cluster.Status.Workers.Nodes[0].PhaseReason, "no Talos credentials")
}

// --- configureWorkerNode: config generation failure ---

func TestConfigureWorkerNode_ConfigGenerationFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	mockTalos := &MockTalosClient{}
	mockTalosGen := &MockTalosConfigGenerator{
		GenerateWorkerConfigFunc: func(hostname string, serverID int64) ([]byte, error) {
			return nil, fmt.Errorf("worker config generation error")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	tc := talosClients{configGen: mockTalosGen, client: mockTalos}

	result := &serverProvisionResult{
		Name:     "w-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureWorkerNode(context.Background(), cluster, tc, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker config generation error")
}

// --- configureWorkerNode: apply config failure ---

func TestConfigureWorkerNode_ApplyConfigFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	mockTalos := &MockTalosClient{
		ApplyConfigFunc: func(ctx context.Context, nodeIP string, config []byte) error {
			return fmt.Errorf("worker apply failed")
		},
	}
	mockTalosGen := &MockTalosConfigGenerator{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	tc := talosClients{configGen: mockTalosGen, client: mockTalos}

	result := &serverProvisionResult{
		Name:     "w-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureWorkerNode(context.Background(), cluster, tc, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker apply failed")
}

// --- waitForCPReady: success ---

func TestWaitForCPReady_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		WaitForNodeReadyFunc: func(ctx context.Context, nodeIP string, timeout int) error {
			return nil
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	tc := talosClients{client: mockTalos}
	err := r.waitForCPReady(context.Background(), cluster, tc, "cp-new", "10.0.0.1")
	require.NoError(t, err)

	// Node should end up in NodeInitializing phase
	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseNodeInitializing, cluster.Status.ControlPlanes.Nodes[0].Phase)
}

// --- waitForCPReady: timeout preserves server ---

func TestWaitForCPReady_Timeout(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		WaitForNodeReadyFunc: func(ctx context.Context, nodeIP string, timeout int) error {
			return fmt.Errorf("timeout waiting for node to be ready")
		},
	}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	tc := talosClients{client: mockTalos}
	err := r.waitForCPReady(context.Background(), cluster, tc, "cp-new", "10.0.0.1")
	require.Error(t, err)
	// Error should mention etcd member preservation
	assert.Contains(t, err.Error(), "etcd member added, server preserved")

	// Node should remain in WaitingForK8s phase (not deleted)
	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseWaitingForK8s, cluster.Status.ControlPlanes.Nodes[0].Phase)
}

// --- waitForWorkerReady: success ---

func TestWaitForWorkerReady_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
			return nil
		}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	err := r.waitForWorkerReady(context.Background(), cluster, "w-new", "10.0.0.1")
	require.NoError(t, err)

	// Node should end up in NodeInitializing phase
	require.Len(t, cluster.Status.Workers.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseNodeInitializing, cluster.Status.Workers.Nodes[0].Phase)
}

// --- waitForWorkerReady: timeout triggers provisioning failure ---

func TestWaitForWorkerReady_Timeout(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
			return fmt.Errorf("timeout")
		}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	err := r.waitForWorkerReady(context.Background(), cluster, "w-new", "10.0.0.1")
	require.Error(t, err)

	// handleProvisioningFailure should have deleted the server
	assert.Contains(t, mockHCloud.DeleteServerCalls, "w-new")
}

// --- deleteNodeAndServer: all branches ---

func TestLoadTalosClients_MissingSecret(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	// No secret created in fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{
				Name: "missing-secret", // this secret doesn't exist
			},
		},
	}

	tc := r.loadTalosClients(context.Background(), cluster)
	// Should return nil clients (not panic) when secret is missing
	assert.Nil(t, tc.client)
	assert.Nil(t, tc.configGen)
}

// --- updateNodePhase: phase transition time only changes on actual phase change ---

func TestConfigureWorkerNode_SuccessFlow(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{}
	mockTalosGen := &MockTalosConfigGenerator{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
		WithNodeReadyWaiter(func(ctx context.Context, nodeName string, timeout time.Duration) error {
			return nil
		}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	tc := talosClients{configGen: mockTalosGen, client: mockTalos}
	result := &serverProvisionResult{
		Name:     "w-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureWorkerNode(context.Background(), cluster, tc, result)
	require.NoError(t, err)

	// Verify Talos config was generated and applied
	require.Len(t, mockTalosGen.GenerateWorkerConfigCalls, 1)
	assert.Equal(t, "w-new", mockTalosGen.GenerateWorkerConfigCalls[0].Hostname)
	assert.Equal(t, int64(12345), mockTalosGen.GenerateWorkerConfigCalls[0].ServerID)

	require.Len(t, mockTalos.ApplyConfigCalls, 1)
	assert.Equal(t, "10.0.0.1", mockTalos.ApplyConfigCalls[0].NodeIP)

	// Node should end in NodeInitializing
	require.Len(t, cluster.Status.Workers.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseNodeInitializing, cluster.Status.Workers.Nodes[0].Phase)
}

// --- configureCPNode: success flow ---

func TestConfigureCPNode_SuccessFlow(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{
		WaitForNodeReadyFunc: func(ctx context.Context, nodeIP string, timeout int) error {
			return nil
		},
	}
	mockTalosGen := &MockTalosConfigGenerator{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{},
			},
		},
	}

	state := &clusterState{
		SANs: []string{"api.cluster.local"},
	}
	tc := talosClients{configGen: mockTalosGen, client: mockTalos}
	result := &serverProvisionResult{
		Name:     "cp-new",
		ServerID: 12345,
		PublicIP: "5.5.5.5",
		TalosIP:  "10.0.0.1",
	}

	err := r.configureCPNode(context.Background(), cluster, state, tc, result)
	require.NoError(t, err)

	// Verify config was generated with correct SANs (original + public IP)
	require.Len(t, mockTalosGen.GenerateControlPlaneConfigCalls, 1)
	call := mockTalosGen.GenerateControlPlaneConfigCalls[0]
	assert.Contains(t, call.SANs, "api.cluster.local")
	assert.Contains(t, call.SANs, "5.5.5.5")
	assert.Equal(t, "cp-new", call.Hostname)
	assert.Equal(t, int64(12345), call.ServerID)

	// Config was applied
	require.Len(t, mockTalos.ApplyConfigCalls, 1)
	assert.Equal(t, "10.0.0.1", mockTalos.ApplyConfigCalls[0].NodeIP)

	// Node should end in NodeInitializing
	require.Len(t, cluster.Status.ControlPlanes.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseNodeInitializing, cluster.Status.ControlPlanes.Nodes[0].Phase)
}

// =============================================================================
// Additional coverage tests - Round 3
// =============================================================================

// --- allPodsReady tests ---

func TestAllPodsReady_EmptyList(t *testing.T) {
	t.Parallel()
	assert.True(t, allPodsReady([]corev1.Pod{}))
}

func TestAllPodsReady_AllRunningAndReady(t *testing.T) {
	t.Parallel()
	pods := []corev1.Pod{
		{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		},
		{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		},
	}
	assert.True(t, allPodsReady(pods))
}

func TestAllPodsReady_PodNotRunning(t *testing.T) {
	t.Parallel()
	pods := []corev1.Pod{
		{
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionFalse},
				},
			},
		},
	}
	assert.False(t, allPodsReady(pods))
}

func TestAllPodsReady_PodRunningButNotReady(t *testing.T) {
	t.Parallel()
	pods := []corev1.Pod{
		{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionFalse},
				},
			},
		},
	}
	assert.False(t, allPodsReady(pods))
}

// --- findTalosEndpoint all branches ---

func TestGetSnapshot_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			return &hcloudgo.Image{ID: 42, Name: "talos-v1.8"}, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	snapshot, err := r.getSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(42), snapshot.ID)
}

func TestGetSnapshot_APIError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			return nil, fmt.Errorf("api error")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	_, err := r.getSnapshot(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get Talos snapshot")
}

func TestGetSnapshot_NilSnapshot(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			return nil, nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	_, err := r.getSnapshot(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Talos snapshot found")
}

// --- createEphemeralSSHKey tests ---

func TestCreateEphemeralSSHKey_Success(t *testing.T) {
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
	}

	keyName, cleanup, err := r.createEphemeralSSHKey(context.Background(), cluster, "worker")
	require.NoError(t, err)
	assert.Contains(t, keyName, "ephemeral-test-cluster-worker-")
	assert.NotNil(t, cleanup)

	// Call cleanup and verify key was deleted
	cleanup()
	require.Len(t, mockHCloud.DeleteSSHKeyCalls, 1)
	assert.Equal(t, keyName, mockHCloud.DeleteSSHKeyCalls[0])
}

func TestCreateEphemeralSSHKey_CreateKeyError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		CreateSSHKeyFunc: func(ctx context.Context, name, publicKey string, labels map[string]string) (string, error) {
			return "", fmt.Errorf("failed to create SSH key")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	_, cleanup, err := r.createEphemeralSSHKey(context.Background(), cluster, "cp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create ephemeral SSH key")
	assert.Nil(t, cleanup)
}

// --- provisionServer tests ---

func TestProvisionServer_CreateServerError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		CreateServerFunc: func(ctx context.Context, opts hcloud.ServerCreateOpts) (string, error) {
			return "", fmt.Errorf("server creation failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	_, err := r.provisionServer(context.Background(), cluster, serverCreateOpts{
		Name:       "test-server",
		SnapshotID: 1,
		ServerType: "cx23",
		Region:     "nbg1",
		SSHKeyName: "test-key",
		Role:       "worker",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create server")

	// Verify the node phase was set to Failed
	require.Len(t, cluster.Status.Workers.Nodes, 1)
	assert.Equal(t, k8znerv1alpha1.NodePhaseFailed, cluster.Status.Workers.Nodes[0].Phase)
}

func TestProvisionServer_GetServerIPError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
			return "", fmt.Errorf("IP not assigned")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := r.provisionServer(ctx, cluster, serverCreateOpts{
		Name:       "cp-server",
		SnapshotID: 1,
		ServerType: "cx23",
		Region:     "nbg1",
		SSHKeyName: "test-key",
		Role:       "control-plane",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get server IP")
}

func TestProvisionServer_GetServerIDError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetServerIDFunc: func(ctx context.Context, name string) (string, error) {
			return "", fmt.Errorf("server ID lookup failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	_, err := r.provisionServer(context.Background(), cluster, serverCreateOpts{
		Name:       "w-server",
		SnapshotID: 1,
		ServerType: "cx23",
		Region:     "nbg1",
		SSHKeyName: "test-key",
		Role:       "worker",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get server ID")
}

func TestProvisionServer_InvalidServerID(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetServerIDFunc: func(ctx context.Context, name string) (string, error) {
			return "not-a-number", nil
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	_, err := r.provisionServer(context.Background(), cluster, serverCreateOpts{
		Name:       "w-server",
		SnapshotID: 1,
		ServerType: "cx23",
		Region:     "nbg1",
		SSHKeyName: "test-key",
		Role:       "worker",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse server ID")
}

func TestProvisionServer_SuccessWithPrivateIP(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
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
		WithMetrics(false),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{Region: "nbg1"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
		},
	}

	result, err := r.provisionServer(context.Background(), cluster, serverCreateOpts{
		Name:       "w-server",
		SnapshotID: 1,
		ServerType: "cx23",
		Region:     "nbg1",
		SSHKeyName: "test-key",
		Role:       "worker",
	})
	require.NoError(t, err)
	assert.Equal(t, "w-server", result.Name)
	assert.Equal(t, int64(12345), result.ServerID)
	assert.Equal(t, "10.0.0.99", result.PrivateIP)
	assert.Equal(t, "10.0.0.99", result.TalosIP, "should use private IP for TalosIP when available")
}

// --- handleProvisioningFailure tests ---

func TestHandleProvisioningFailure_Success(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseCreatingServer},
				},
			},
		},
	}

	r.handleProvisioningFailure(context.Background(), cluster, "worker", "w-1", "test reason")

	// Node should be removed from status
	assert.Empty(t, cluster.Status.Workers.Nodes, "node should be removed after provisioning failure")
	// Server should be deleted
	require.Len(t, mockHCloud.DeleteServerCalls, 1)
	assert.Equal(t, "w-1", mockHCloud.DeleteServerCalls[0])
}

func TestHandleProvisioningFailure_DeleteError(t *testing.T) {
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
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "cp-1", Phase: k8znerv1alpha1.NodePhaseWaitingForIP},
				},
			},
		},
	}

	// Should not panic even when delete fails
	r.handleProvisioningFailure(context.Background(), cluster, "control-plane", "cp-1", "IP assignment timeout")

	// Node should still be removed from status
	assert.Empty(t, cluster.Status.ControlPlanes.Nodes)
}

// --- drainNode with pods ---

func TestLoadTalosClients_AlreadyInjected(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockTalos := &MockTalosClient{}
	mockGen := &MockTalosConfigGenerator{}

	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithTalosClient(mockTalos),
		WithTalosConfigGenerator(mockGen),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{}

	tc := r.loadTalosClients(context.Background(), cluster)
	assert.Equal(t, mockTalos, tc.client)
	assert.Equal(t, mockGen, tc.configGen)
}

func TestLoadTalosClients_NoCredentialsRef(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(fakeClient, scheme, recorder)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			CredentialsRef: corev1.LocalObjectReference{Name: ""},
		},
	}

	tc := r.loadTalosClients(context.Background(), cluster)
	assert.Nil(t, tc.client)
	assert.Nil(t, tc.configGen)
}

// --- checkPortReachable: unreachable port ---

func TestCheckPortReachable_Unreachable(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(fakeClient, scheme, recorder)

	// Port 1 on localhost is almost certainly not listening
	result := r.checkPortReachable("127.0.0.1", 1, 100*time.Millisecond)
	assert.False(t, result)
}

// --- Reconcile: ensureHCloudClient failure path ---

func TestPrepareForProvisioning_SnapshotError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		GetSnapshotByLabelsFunc: func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			return nil, fmt.Errorf("snapshot error")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	_, _, err := r.prepareForProvisioning(context.Background(), cluster, "worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot")
}

// --- prepareForProvisioning: SSH key error ---

func TestPrepareForProvisioning_SSHKeyError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{
		CreateSSHKeyFunc: func(ctx context.Context, name, publicKey string, labels map[string]string) (string, error) {
			return "", fmt.Errorf("ssh key creation failed")
		},
	}
	r := NewClusterReconciler(fakeClient, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
	}

	_, _, err := r.prepareForProvisioning(context.Background(), cluster, "cp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create ephemeral SSH key")
}

// --- prepareForProvisioning: cluster state error ---

func TestPrepareForProvisioning_ClusterStateError(t *testing.T) {
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
	}

	_, _, err := r.prepareForProvisioning(context.Background(), cluster, "worker")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to build cluster state")
}

// --- provisionAndConfigureNode: success path ---

func TestProvisionAndConfigureNode_SuccessPath(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{Region: "nbg1"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
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

	configureCalled := false
	err := r.provisionAndConfigureNode(context.Background(), cluster, nodeProvisionParams{
		Name:       "w-test",
		Role:       "worker",
		Pool:       "workers",
		ServerType: "cx23",
		SnapshotID: 1,
		SSHKeyName: "test-key",
		NetworkID:  100,
		Configure: func(serverName string, result *serverProvisionResult) error {
			configureCalled = true
			return nil
		},
	})
	require.NoError(t, err)
	assert.True(t, configureCalled, "configure function should have been called")
}

func TestProvisionAndConfigureNode_ConfigureFailure(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{Region: "nbg1"},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{},
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

	err := r.provisionAndConfigureNode(context.Background(), cluster, nodeProvisionParams{
		Name:       "w-test",
		Role:       "worker",
		Pool:       "workers",
		ServerType: "cx23",
		SnapshotID: 1,
		SSHKeyName: "test-key",
		NetworkID:  100,
		Configure: func(serverName string, result *serverProvisionResult) error {
			return fmt.Errorf("configure error")
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configure error")
}

// --- replaceControlPlane: flow test ---
