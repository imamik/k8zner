package compute

import (
	"context"
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

func (m *mockTalosProducer) GenerateControlPlaneConfig(_ []string, _ string) ([]byte, error) {
	return []byte("control-plane-config"), nil
}

func (m *mockTalosProducer) GenerateWorkerConfig(_ string) ([]byte, error) {
	return []byte("worker-config"), nil
}

func (m *mockTalosProducer) GetClientConfig() ([]byte, error) {
	return []byte("client-config"), nil
}

func (m *mockTalosProducer) SetEndpoint(endpoint string) {
	m.endpoint = endpoint
}

func (m *mockTalosProducer) GenerateAutoscalerConfig(_ string, _ map[string]string, _ []string) ([]byte, error) {
	return []byte("autoscaler-config"), nil
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
		ControlPlaneIPs: make(map[string]string),
		WorkerIPs:       make(map[string]string),
	}

	return ctx
}

func TestProvisioner_Name(t *testing.T) {
	p := NewProvisioner()
	assert.Equal(t, "compute", p.Name())
}

func TestProvisioner_Provision_EmptyConfig(t *testing.T) {
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
	p := NewProvisioner()

	err := p.Provision(ctx)
	assert.NoError(t, err)
}

func TestProvisionControlPlane_SingleNode(t *testing.T) {
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

	mockInfra.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		// Server doesn't exist yet
		return "", nil
	}

	serverCreated := false
	mockInfra.CreateServerFunc = func(_ context.Context, name, _, serverType, location string, _ []string, labels map[string]string, _ string, pgID *int64, _ int64, privateIP string) (string, error) {
		serverCreated = true
		assert.Contains(t, name, "test-cluster")
		assert.Contains(t, name, "control-plane")
		assert.Equal(t, "cx21", serverType)
		assert.Equal(t, "nbg1", location)
		assert.NotNil(t, pgID)
		assert.NotEmpty(t, privateIP)
		assert.Equal(t, "control-plane", labels["role"])
		return "server-123", nil
	}

	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.1.1", nil
	}

	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 456, Description: "talos-snapshot"}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionControlPlane(ctx)
	assert.NoError(t, err)
	assert.True(t, serverCreated, "server should have been created")
	assert.NotEmpty(t, ctx.State.ControlPlaneIPs, "control plane IPs should be populated")
}

func TestProvisionWorkers_MultipleNodes(t *testing.T) {
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

	mockInfra.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "", nil
	}

	createdServers := make(map[string]bool)
	var mu sync.Mutex
	mockInfra.CreateServerFunc = func(_ context.Context, name, _, serverType, _ string, _ []string, labels map[string]string, _ string, _ *int64, _ int64, _ string) (string, error) {
		mu.Lock()
		createdServers[name] = true
		mu.Unlock()
		assert.Equal(t, "cx31", serverType)
		assert.Equal(t, "worker", labels["role"])
		return "server-" + name, nil
	}

	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.2.1", nil
	}

	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 456}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionWorkers(ctx)
	assert.NoError(t, err)
	assert.Len(t, createdServers, 2, "two worker servers should have been created")
}

func TestProvisionControlPlane_ExistingServer(t *testing.T) {
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
		return "existing-server-id", nil
	}

	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.1.1", nil
	}

	// CreateServer should NOT be called since server exists
	mockInfra.CreateServerFunc = func(_ context.Context, _, _, _, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string) (string, error) {
		t.Fatal("CreateServer should not be called for existing server")
		return "", nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionControlPlane(ctx)
	assert.NoError(t, err)
	assert.NotEmpty(t, ctx.State.ControlPlaneIPs)
}
