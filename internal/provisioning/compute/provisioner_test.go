package compute

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTalosProducer implements TalosConfigProducer for testing.
type mockTalosProducer struct {
	endpoint string
}

func (m *mockTalosProducer) GenerateControlPlaneConfig(_ []string, _ string, _ int64) ([]byte, error) {
	return []byte("control-plane-config"), nil
}

func (m *mockTalosProducer) GenerateWorkerConfig(_ string, _ int64) ([]byte, error) {
	return []byte("worker-config"), nil
}

func (m *mockTalosProducer) GetClientConfig() ([]byte, error) {
	return []byte("client-config"), nil
}

func (m *mockTalosProducer) SetEndpoint(endpoint string) {
	m.endpoint = endpoint
}

func (m *mockTalosProducer) GetNodeVersion(_ context.Context, _ string) (string, error) {
	return "v1.8.2", nil
}

func (m *mockTalosProducer) UpgradeNode(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
	return nil
}

func (m *mockTalosProducer) UpgradeKubernetes(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockTalosProducer) WaitForNodeReady(_ context.Context, _ string, _ time.Duration) error {
	return nil
}

func (m *mockTalosProducer) HealthCheck(_ context.Context, _ string) error {
	return nil
}

func (m *mockTalosProducer) SetMachineConfigOptions(_ interface{}) {
}

func createTestContext(t *testing.T, mockInfra *hcloud_internal.MockClient, cfg *config.Config) *provisioning.Context {
	t.Helper()

	_, ipNet, err := net.ParseCIDR("10.0.0.0/16")
	require.NoError(t, err)

	ctx := provisioning.NewContext(
		context.Background(),
		cfg,
		mockInfra,
		&mockTalosProducer{}, // Provide mock Talos producer
	)

	// Initialize state with network
	ctx.State = &provisioning.State{
		Network: &hcloud.Network{
			ID:      1,
			IPRange: ipNet,
		},
		ControlPlaneIPs:       make(map[string]string),
		WorkerIPs:             make(map[string]string),
		ControlPlaneServerIDs: make(map[string]int64),
		WorkerServerIDs:       make(map[string]int64),
	}

	return ctx
}

func TestProvisioner_Provision_EmptyConfig(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{},
		},
		Workers: []config.WorkerNodePool{},
	}

	// LoadBalancer returns nil (no LB configured)
	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return nil, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	err := Provision(ctx)
	assert.NoError(t, err)
}

func TestProvisionControlPlane_SingleNode(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					ServerType: "cx21",
					Count:      1,
					Location:   "nbg1",
				},
			},
		},
	}
	// Calculate subnets for IP allocation
	require.NoError(t, cfg.CalculateSubnets())

	// Setup mock expectations
	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID: 1,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("5.6.7.8"),
				},
			},
		}, nil
	}

	mockInfra.EnsurePlacementGroupFunc = func(_ context.Context, name, _ string, _ map[string]string) (*hcloud.PlacementGroup, error) {
		assert.Contains(t, name, "test-cluster")
		return &hcloud.PlacementGroup{ID: 1}, nil
	}

	// Track which servers have been created
	createdServerIDs := make(map[string]string)
	var serverMu sync.Mutex

	mockInfra.GetServerIDFunc = func(_ context.Context, name string) (string, error) {
		serverMu.Lock()
		defer serverMu.Unlock()
		if id, exists := createdServerIDs[name]; exists {
			return id, nil
		}
		// Server doesn't exist yet
		return "", nil
	}

	serverCreated := false
	mockInfra.CreateServerFunc = func(_ context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		serverCreated = true
		assert.Contains(t, opts.Name, "test-cluster")
		assert.Contains(t, opts.Name, "-cp-") // New naming: cluster-cp-1
		assert.Equal(t, "cx21", opts.ServerType)
		assert.Equal(t, "nbg1", opts.Location)
		assert.NotNil(t, opts.PlacementGroupID)
		assert.NotEmpty(t, opts.PrivateIP)
		assert.Equal(t, "control-plane", opts.Labels["role"])
		// Store the server ID for later lookup
		serverMu.Lock()
		createdServerIDs[opts.Name] = "12345"
		serverMu.Unlock()
		return "12345", nil
	}

	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.1.1", nil
	}

	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 456, Description: "talos-snapshot"}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	err := ProvisionControlPlane(ctx)
	assert.NoError(t, err)
	assert.True(t, serverCreated, "server should have been created")
	assert.NotEmpty(t, ctx.State.ControlPlaneIPs, "control plane IPs should be populated")
}

func TestProvisionWorkers_MultipleNodes(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "worker-pool",
				ServerType: "cx31",
				Count:      2,
				Location:   "nbg1",
			},
		},
	}
	// Calculate subnets for IP allocation
	require.NoError(t, cfg.CalculateSubnets())

	// Track which servers have been created with their IDs
	createdServerIDs := make(map[string]string)
	var mu sync.Mutex
	serverCounter := int64(10000)

	mockInfra.GetServerIDFunc = func(_ context.Context, name string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if id, exists := createdServerIDs[name]; exists {
			return id, nil
		}
		return "", nil
	}

	createdServers := make(map[string]bool)
	mockInfra.CreateServerFunc = func(_ context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		mu.Lock()
		serverCounter++
		idStr := fmt.Sprintf("%d", serverCounter)
		createdServerIDs[opts.Name] = idStr
		createdServers[opts.Name] = true
		mu.Unlock()
		assert.Equal(t, "cx31", opts.ServerType)
		assert.Equal(t, "worker", opts.Labels["role"])
		return idStr, nil
	}

	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.2.1", nil
	}

	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 456}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	err := ProvisionWorkers(ctx)
	assert.NoError(t, err)
	assert.Len(t, createdServers, 2, "two worker servers should have been created")
}

// TestProvision_WithBothCPAndWorkers exercises provisionAllServers via Provision()
// which creates both CP and worker servers in parallel. This covers the 11.8% provisionAllServers path.
func TestProvision_WithBothCPAndWorkers(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					ServerType: "cx21",
					Count:      1,
					Location:   "nbg1",
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "worker-pool",
				ServerType: "cx31",
				Count:      2,
				Location:   "nbg1",
			},
		},
	}
	require.NoError(t, cfg.CalculateSubnets())

	// Mock GetLoadBalancer for prepareControlPlaneEndpoint
	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID: 1,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("5.6.7.8"),
				},
			},
		}, nil
	}

	// Mock EnsurePlacementGroup for CP pools
	mockInfra.EnsurePlacementGroupFunc = func(_ context.Context, _, _ string, _ map[string]string) (*hcloud.PlacementGroup, error) {
		return &hcloud.PlacementGroup{ID: 1}, nil
	}

	// Track servers
	createdServerIDs := make(map[string]string)
	var mu sync.Mutex
	serverCounter := int64(1000)

	mockInfra.GetServerIDFunc = func(_ context.Context, name string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		return createdServerIDs[name], nil
	}

	mockInfra.CreateServerFunc = func(_ context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		mu.Lock()
		serverCounter++
		idStr := fmt.Sprintf("%d", serverCounter)
		createdServerIDs[opts.Name] = idStr
		mu.Unlock()
		return idStr, nil
	}

	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.1.1", nil
	}

	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 456}, nil
	}

	// CreateSSHKey and DeleteSSHKey for ephemeral key
	sshKeyCreated := false
	sshKeyDeleted := false
	mockInfra.CreateSSHKeyFunc = func(_ context.Context, _, _ string, _ map[string]string) (string, error) {
		sshKeyCreated = true
		return "ssh-key-1", nil
	}
	mockInfra.DeleteSSHKeyFunc = func(_ context.Context, _ string) error {
		sshKeyDeleted = true
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	err := Provision(ctx)
	assert.NoError(t, err)
	assert.True(t, sshKeyCreated, "SSH key should have been created")
	assert.True(t, sshKeyDeleted, "SSH key should have been deleted")
	assert.NotEmpty(t, ctx.State.ControlPlaneIPs, "control plane IPs should be populated")
	assert.NotEmpty(t, ctx.State.WorkerIPs, "worker IPs should be populated")
	// 1 CP + 2 workers = 3 total servers
	assert.Len(t, ctx.State.ControlPlaneIPs, 1, "should have 1 control plane IP")
	assert.Len(t, ctx.State.WorkerIPs, 2, "should have 2 worker IPs")
}

// TestProvision_CreateSSHKeyFailure tests Provision() when CreateSSHKey fails.
func TestProvision_CreateSSHKeyFailure(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp", ServerType: "cx21", Count: 1, Location: "nbg1"},
			},
		},
	}
	require.NoError(t, cfg.CalculateSubnets())

	mockInfra.CreateSSHKeyFunc = func(_ context.Context, _, _ string, _ map[string]string) (string, error) {
		return "", fmt.Errorf("SSH key quota exceeded")
	}

	ctx := createTestContext(t, mockInfra, cfg)

	err := Provision(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create ephemeral SSH key")
}

// TestProvision_GetLoadBalancerFailure tests Provision() when GetLoadBalancer fails.
func TestProvision_GetLoadBalancerFailure(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp", ServerType: "cx21", Count: 1, Location: "nbg1"},
			},
		},
	}
	require.NoError(t, cfg.CalculateSubnets())

	sshKeyDeleted := false
	mockInfra.CreateSSHKeyFunc = func(_ context.Context, _, _ string, _ map[string]string) (string, error) {
		return "key-1", nil
	}
	mockInfra.DeleteSSHKeyFunc = func(_ context.Context, _ string) error {
		sshKeyDeleted = true
		return nil
	}
	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return nil, fmt.Errorf("API connection timeout")
	}

	ctx := createTestContext(t, mockInfra, cfg)

	err := Provision(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get load balancer")
	// SSH key should still be cleaned up even on error
	assert.True(t, sshKeyDeleted, "SSH key should be cleaned up on error")
}

// TestProvision_EnsurePlacementGroupFailure tests Provision() when EnsurePlacementGroup fails.
func TestProvision_EnsurePlacementGroupFailure(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp", ServerType: "cx21", Count: 1, Location: "nbg1"},
			},
		},
	}
	require.NoError(t, cfg.CalculateSubnets())

	mockInfra.CreateSSHKeyFunc = func(_ context.Context, _, _ string, _ map[string]string) (string, error) {
		return "key-1", nil
	}
	mockInfra.DeleteSSHKeyFunc = func(_ context.Context, _ string) error {
		return nil
	}
	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return nil, nil // No LB
	}
	mockInfra.EnsurePlacementGroupFunc = func(_ context.Context, _, _ string, _ map[string]string) (*hcloud.PlacementGroup, error) {
		return nil, fmt.Errorf("placement group API unavailable")
	}

	ctx := createTestContext(t, mockInfra, cfg)

	err := Provision(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to provision servers")
}

// TestProvision_DeleteSSHKeyFailure tests that SSH key cleanup failure is logged but doesn't block.
func TestProvision_DeleteSSHKeyFailure(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{},
		},
		Workers: []config.WorkerNodePool{},
	}

	mockInfra.CreateSSHKeyFunc = func(_ context.Context, _, _ string, _ map[string]string) (string, error) {
		return "key-1", nil
	}
	mockInfra.DeleteSSHKeyFunc = func(_ context.Context, _ string) error {
		return fmt.Errorf("delete SSH key failed")
	}
	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return nil, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	// Provision with empty config should succeed even if SSH key delete fails
	// (delete failure is only logged)
	err := Provision(ctx)
	assert.NoError(t, err)
}

func TestProvisionControlPlane_ExistingServer(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					ServerType: "cx21",
					Count:      1,
					Location:   "nbg1",
				},
			},
		},
	}
	// Calculate subnets for IP allocation
	require.NoError(t, cfg.CalculateSubnets())

	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return nil, nil
	}

	mockInfra.EnsurePlacementGroupFunc = func(_ context.Context, _, _ string, _ map[string]string) (*hcloud.PlacementGroup, error) {
		return &hcloud.PlacementGroup{ID: 1}, nil
	}

	// Server already exists
	mockInfra.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "12345", nil
	}

	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.1.1", nil
	}

	// CreateServer should NOT be called since server exists
	mockInfra.CreateServerFunc = func(_ context.Context, _ hcloud_internal.ServerCreateOpts) (string, error) {
		t.Fatal("CreateServer should not be called for existing server")
		return "", nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	err := ProvisionControlPlane(ctx)
	assert.NoError(t, err)
	assert.NotEmpty(t, ctx.State.ControlPlaneIPs)
}
