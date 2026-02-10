package infrastructure

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp", Count: 1, ServerType: "cx21"},
			},
		},
	}
	require.NoError(t, cfg.CalculateSubnets())
	return cfg
}

func setupNetworkMock(mockInfra *hcloud_internal.MockClient) {
	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	mockInfra.EnsureNetworkFunc = func(_ context.Context, name, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return &hcloud.Network{ID: 1, Name: name, IPRange: ipNet}, nil
	}
	mockInfra.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		return nil
	}
}

func setupFirewallMock(mockInfra *hcloud_internal.MockClient) {
	mockInfra.EnsureFirewallFunc = func(_ context.Context, name string, _ []hcloud.FirewallRule, _ map[string]string, _ string) (*hcloud.Firewall, error) {
		return &hcloud.Firewall{ID: 1, Name: name}, nil
	}
}

func setupLBMock(mockInfra *hcloud_internal.MockClient) {
	mockInfra.EnsureLoadBalancerFunc = func(_ context.Context, name, _, _ string, _ hcloud.LoadBalancerAlgorithmType, _ map[string]string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID:   1,
			Name: name,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{IP: net.ParseIP("5.6.7.8")},
			},
		}, nil
	}
	mockInfra.AttachToNetworkFunc = func(_ context.Context, _ *hcloud.LoadBalancer, _ *hcloud.Network, _ net.IP) error {
		return nil
	}
	mockInfra.ConfigureServiceFunc = func(_ context.Context, _ *hcloud.LoadBalancer, _ hcloud.LoadBalancerAddServiceOpts) error {
		return nil
	}
	mockInfra.AddTargetFunc = func(_ context.Context, _ *hcloud.LoadBalancer, _ hcloud.LoadBalancerTargetType, _ string) error {
		return nil
	}
}

// --- Provision() orchestration tests ---

func TestProvision_Success(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	setupNetworkMock(mockInfra)
	setupFirewallMock(mockInfra)
	setupLBMock(mockInfra)

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.Provision(ctx)
	require.NoError(t, err)
	assert.NotNil(t, ctx.State.Network)
}

func TestProvision_NetworkError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	mockInfra.EnsureNetworkFunc = func(_ context.Context, _, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return nil, fmt.Errorf("network quota exceeded")
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.Provision(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network")
}

func TestProvision_FirewallError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	setupNetworkMock(mockInfra)
	mockInfra.EnsureFirewallFunc = func(_ context.Context, _ string, _ []hcloud.FirewallRule, _ map[string]string, _ string) (*hcloud.Firewall, error) {
		return nil, fmt.Errorf("firewall limit reached")
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.Provision(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "firewall")
}

func TestProvision_LoadBalancerError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	setupNetworkMock(mockInfra)
	setupFirewallMock(mockInfra)
	mockInfra.EnsureLoadBalancerFunc = func(_ context.Context, _, _, _ string, _ hcloud.LoadBalancerAlgorithmType, _ map[string]string) (*hcloud.LoadBalancer, error) {
		return nil, fmt.Errorf("LB type unavailable")
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.Provision(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LB type unavailable")
}

// --- Network error path tests ---

func TestProvisionNetwork_EnsureNetworkError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	mockInfra.EnsureNetworkFunc = func(_ context.Context, _, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return nil, fmt.Errorf("API error")
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionNetwork(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ensure network")
}

func TestProvisionNetwork_SubnetError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	mockInfra.EnsureNetworkFunc = func(_ context.Context, name, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return &hcloud.Network{ID: 1, Name: name, IPRange: ipNet}, nil
	}
	mockInfra.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		return fmt.Errorf("subnet overlap")
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionNetwork(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subnet")
}

func TestProvisionNetwork_PublicIPDetection(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)
	useIP := true
	cfg.Firewall.UseCurrentIPv4 = &useIP

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	mockInfra.EnsureNetworkFunc = func(_ context.Context, name, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return &hcloud.Network{ID: 1, Name: name, IPRange: ipNet}, nil
	}
	mockInfra.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		return nil
	}
	mockInfra.GetPublicIPFunc = func(_ context.Context) (string, error) {
		return "203.0.113.5", nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionNetwork(ctx)
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.5", ctx.State.PublicIP)
}

func TestProvisionNetwork_PublicIPDetectionError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)
	useIP := true
	cfg.Firewall.UseCurrentIPv4 = &useIP

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	mockInfra.EnsureNetworkFunc = func(_ context.Context, name, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return &hcloud.Network{ID: 1, Name: name, IPRange: ipNet}, nil
	}
	mockInfra.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		return nil
	}
	mockInfra.GetPublicIPFunc = func(_ context.Context) (string, error) {
		return "", fmt.Errorf("network unreachable")
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	// Public IP failure is non-fatal (warning only)
	err := p.ProvisionNetwork(ctx)
	require.NoError(t, err)
	assert.Empty(t, ctx.State.PublicIP, "should remain empty on error")
}

func TestProvisionNetwork_WorkerSubnetError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)
	cfg.Workers = []config.WorkerNodePool{
		{Name: "pool-a", Count: 2, ServerType: "cx21"},
	}
	require.NoError(t, cfg.CalculateSubnets())

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	subnetCount := 0
	mockInfra.EnsureNetworkFunc = func(_ context.Context, name, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return &hcloud.Network{ID: 1, Name: name, IPRange: ipNet}, nil
	}
	mockInfra.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		subnetCount++
		if subnetCount > 2 { // Fail on worker subnet (3rd call)
			return fmt.Errorf("worker subnet error")
		}
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionNetwork(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker subnet")
}

// --- Firewall error path tests ---

func TestProvisionFirewall_EnsureError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	mockInfra.EnsureFirewallFunc = func(_ context.Context, _ string, _ []hcloud.FirewallRule, _ map[string]string, _ string) (*hcloud.Firewall, error) {
		return nil, fmt.Errorf("firewall API unavailable")
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionFirewall(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "firewall")
}

// --- Load balancer error path tests ---

func TestProvisionLoadBalancers_NoCPNodes(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)
	cfg.ControlPlane.NodePools = nil // No CP nodes

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	ctx := createTestContext(t, mockInfra, cfg)
	ctx.State.Network = &hcloud.Network{ID: 1, IPRange: ipNet}

	p := NewProvisioner()
	err := p.ProvisionLoadBalancers(ctx)

	// Should succeed with no LBs needed
	require.NoError(t, err)
}

func TestProvisionLoadBalancers_ServiceConfigError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	ctx := createTestContext(t, mockInfra, cfg)
	ctx.State.Network = &hcloud.Network{ID: 1, IPRange: ipNet}

	mockInfra.EnsureLoadBalancerFunc = func(_ context.Context, name, _, _ string, _ hcloud.LoadBalancerAlgorithmType, _ map[string]string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID:   1,
			Name: name,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{IP: net.ParseIP("5.6.7.8")},
			},
		}, nil
	}
	mockInfra.ConfigureServiceFunc = func(_ context.Context, _ *hcloud.LoadBalancer, _ hcloud.LoadBalancerAddServiceOpts) error {
		return fmt.Errorf("service config failed")
	}

	p := NewProvisioner()
	err := p.ProvisionLoadBalancers(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "service")
}

func TestProvisionLoadBalancers_AttachNetworkError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	ctx := createTestContext(t, mockInfra, cfg)
	ctx.State.Network = &hcloud.Network{ID: 1, IPRange: ipNet}

	mockInfra.EnsureLoadBalancerFunc = func(_ context.Context, name, _, _ string, _ hcloud.LoadBalancerAlgorithmType, _ map[string]string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID:   1,
			Name: name,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{IP: net.ParseIP("5.6.7.8")},
			},
		}, nil
	}
	mockInfra.ConfigureServiceFunc = func(_ context.Context, _ *hcloud.LoadBalancer, _ hcloud.LoadBalancerAddServiceOpts) error {
		return nil
	}
	mockInfra.AttachToNetworkFunc = func(_ context.Context, _ *hcloud.LoadBalancer, _ *hcloud.Network, _ net.IP) error {
		return fmt.Errorf("network attachment failed")
	}

	p := NewProvisioner()
	err := p.ProvisionLoadBalancers(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "network")
}
