package infrastructure

import (
	"context"
	"net"
	"testing"

	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestContext(t *testing.T, mockInfra *hcloud_internal.MockClient, cfg *config.Config) *provisioning.Context {
	t.Helper()

	ctx := provisioning.NewContext(
		context.Background(),
		cfg,
		mockInfra,
		nil,
	)

	return ctx
}

func TestProvisioner_Name(t *testing.T) {
	p := NewProvisioner()
	assert.Equal(t, "infrastructure", p.Name())
}

func TestProvisionNetwork_Success(t *testing.T) {
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
	}
	require.NoError(t, cfg.CalculateSubnets())

	_, ipNet, err := net.ParseCIDR("10.0.0.0/16")
	require.NoError(t, err)

	// Setup mock expectations
	mockInfra.EnsureNetworkFunc = func(_ context.Context, name, ipRange, _ string, _ map[string]string) (*hcloud.Network, error) {
		assert.Contains(t, name, "test-cluster")
		assert.Equal(t, "10.0.0.0/16", ipRange)
		return &hcloud.Network{
			ID:      1,
			Name:    name,
			IPRange: ipNet,
		}, nil
	}

	mockInfra.EnsureSubnetFunc = func(_ context.Context, network *hcloud.Network, cidr string, _ string, _ hcloud.NetworkSubnetType) error {
		assert.NotNil(t, network)
		assert.NotEmpty(t, cidr)
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err = p.ProvisionNetwork(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, ctx.State.Network)
}

func TestProvisionFirewall_Success(t *testing.T) {
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
	}

	// Setup mock expectations
	mockInfra.GetPublicIPFunc = func(_ context.Context) (string, error) {
		return "1.2.3.4", nil
	}

	mockInfra.EnsureFirewallFunc = func(_ context.Context, name string, _ []hcloud.FirewallRule, _ map[string]string) (*hcloud.Firewall, error) {
		assert.Contains(t, name, "test-cluster")
		// Rules may be empty if no control plane or workers configured
		return &hcloud.Firewall{ID: 1, Name: name}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionFirewall(ctx)
	assert.NoError(t, err)
}

func TestProvisionLoadBalancers_Success(t *testing.T) {
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
	}
	require.NoError(t, cfg.CalculateSubnets())

	_, ipNet, err := net.ParseCIDR("10.0.0.0/16")
	require.NoError(t, err)

	// Initialize state with network
	ctx := createTestContext(t, mockInfra, cfg)
	ctx.State.Network = &hcloud.Network{ID: 1, IPRange: ipNet}

	mockInfra.EnsureLoadBalancerFunc = func(_ context.Context, name, _ string, _ string, _ hcloud.LoadBalancerAlgorithmType, _ map[string]string) (*hcloud.LoadBalancer, error) {
		assert.Contains(t, name, "test-cluster")
		return &hcloud.LoadBalancer{
			ID:   1,
			Name: name,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("5.6.7.8"),
				},
			},
		}, nil
	}

	mockInfra.AttachToNetworkFunc = func(_ context.Context, lb *hcloud.LoadBalancer, network *hcloud.Network, _ net.IP) error {
		assert.NotNil(t, lb)
		assert.NotNil(t, network)
		return nil
	}

	mockInfra.ConfigureServiceFunc = func(_ context.Context, _ *hcloud.LoadBalancer, _ hcloud.LoadBalancerAddServiceOpts) error {
		return nil
	}

	p := NewProvisioner()

	err = p.ProvisionLoadBalancers(ctx)
	assert.NoError(t, err)
}

func TestProvisionFloatingIPs_Disabled(t *testing.T) {
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			PublicVIPIPv4Enabled:  false,
			PrivateVIPIPv4Enabled: false,
		},
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionFloatingIPs(ctx)
	assert.NoError(t, err)
}

func TestGetNetwork(t *testing.T) {
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
	}

	ctx := createTestContext(t, mockInfra, cfg)

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	expectedNetwork := &hcloud.Network{ID: 1, IPRange: ipNet}
	ctx.State.Network = expectedNetwork

	p := NewProvisioner()
	network := p.GetNetwork(ctx)

	assert.Equal(t, expectedNetwork, network)
}
