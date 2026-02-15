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

// --- Firewall: rule generation paths ---

func TestProvisionFirewall_WithKubeAPIAndTalosRules(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Firewall: config.FirewallConfig{
			KubeAPISource:  []string{"10.0.0.0/8"},
			TalosAPISource: []string{"192.168.0.0/16"},
		},
	}

	var capturedRules []hcloud.FirewallRule
	mockInfra.EnsureFirewallFunc = func(_ context.Context, _ string, rules []hcloud.FirewallRule, _ map[string]string, _ string) (*hcloud.Firewall, error) {
		capturedRules = rules
		return &hcloud.Firewall{ID: 1, Name: "test-cluster"}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionFirewall(ctx)
	require.NoError(t, err)

	// Should have 2 rules: KubeAPI (6443) and TalosAPI (50000)
	require.Len(t, capturedRules, 2)
	assert.Equal(t, "6443", *capturedRules[0].Port)
	assert.Equal(t, "50000", *capturedRules[1].Port)
}

func TestProvisionFirewall_WithExtraRules(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Firewall: config.FirewallConfig{
			ExtraRules: []config.FirewallRule{
				{
					Description: "Allow SSH",
					Direction:   "in",
					Protocol:    "tcp",
					Port:        "22",
					SourceIPs:   []string{"10.0.0.0/8"},
				},
				{
					Description: "Allow ICMP",
					Direction:   "in",
					Protocol:    "icmp",
					SourceIPs:   []string{"0.0.0.0/0"},
				},
			},
		},
	}

	var capturedRules []hcloud.FirewallRule
	mockInfra.EnsureFirewallFunc = func(_ context.Context, _ string, rules []hcloud.FirewallRule, _ map[string]string, _ string) (*hcloud.Firewall, error) {
		capturedRules = rules
		return &hcloud.Firewall{ID: 1, Name: "test-cluster"}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionFirewall(ctx)
	require.NoError(t, err)

	// No KubeAPI/Talos sources, but 2 extra rules
	require.Len(t, capturedRules, 2)
	assert.Equal(t, "Allow SSH", *capturedRules[0].Description)
	assert.Equal(t, "Allow ICMP", *capturedRules[1].Description)
}

func TestProvisionFirewall_WithAllRuleTypes(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	useIP := true
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Firewall: config.FirewallConfig{
			KubeAPISource:  []string{"10.0.0.0/8"},
			TalosAPISource: []string{"172.16.0.0/12"},
			UseCurrentIPv4: &useIP,
			ExtraRules: []config.FirewallRule{
				{
					Description: "Custom rule",
					Direction:   "in",
					Protocol:    "tcp",
					Port:        "8080",
					SourceIPs:   []string{"192.168.0.0/16"},
				},
			},
		},
	}

	var capturedRules []hcloud.FirewallRule
	mockInfra.EnsureFirewallFunc = func(_ context.Context, _ string, rules []hcloud.FirewallRule, _ map[string]string, _ string) (*hcloud.Firewall, error) {
		capturedRules = rules
		return &hcloud.Firewall{ID: 1, Name: "test-cluster"}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	ctx.State.PublicIP = "1.2.3.4"
	p := NewProvisioner()

	err := p.ProvisionFirewall(ctx)
	require.NoError(t, err)

	// KubeAPI + TalosAPI + 1 extra rule = 3 rules
	require.Len(t, capturedRules, 3)
	assert.Equal(t, "6443", *capturedRules[0].Port)
	assert.Equal(t, "50000", *capturedRules[1].Port)
	assert.Equal(t, "8080", *capturedRules[2].Port)
}

func TestProvisionFirewall_APISourceFallback(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Firewall: config.FirewallConfig{
			// Use APISource as fallback (no KubeAPI/Talos specific)
			APISource: []string{"10.0.0.0/8"},
		},
	}

	var capturedRules []hcloud.FirewallRule
	mockInfra.EnsureFirewallFunc = func(_ context.Context, _ string, rules []hcloud.FirewallRule, _ map[string]string, _ string) (*hcloud.Firewall, error) {
		capturedRules = rules
		return &hcloud.Firewall{ID: 1, Name: "test-cluster"}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionFirewall(ctx)
	require.NoError(t, err)

	// Both KubeAPI and TalosAPI rules using APISource fallback
	require.Len(t, capturedRules, 2)
}

// --- Network: LB subnet error paths ---

func TestProvisionNetwork_LBSubnetCalcError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
			// Deliberately leave NodeIPv4CIDR empty to cause GetSubnetForRole error
			// after CP succeeds
		},
	}
	// Calculate subnets then corrupt the node CIDR to cause LB subnet error
	require.NoError(t, cfg.CalculateSubnets())

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	subnetCallCount := 0
	mockInfra.EnsureNetworkFunc = func(_ context.Context, name, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return &hcloud.Network{ID: 1, Name: name, IPRange: ipNet}, nil
	}
	mockInfra.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		subnetCallCount++
		if subnetCallCount == 2 {
			// Fail on LB subnet (second call, after CP)
			return fmt.Errorf("LB subnet create error")
		}
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionNetwork(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load-balancer subnet")
}

// --- Network: GetSubnetForRole error paths ---

func TestProvisionNetwork_CPSubnetCalcError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
	}
	require.NoError(t, cfg.CalculateSubnets())

	// Corrupt the NodeIPv4CIDR to trigger GetSubnetForRole error
	cfg.Network.NodeIPv4CIDR = "invalid-cidr"

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	mockInfra.EnsureNetworkFunc = func(_ context.Context, name, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return &hcloud.Network{ID: 1, Name: name, IPRange: ipNet}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionNetwork(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "control-plane subnet")
}

func TestProvisionNetwork_LBSubnetCalcError_GetSubnetForRole(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
	}
	require.NoError(t, cfg.CalculateSubnets())

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	cpSubnetDone := false
	mockInfra.EnsureNetworkFunc = func(_ context.Context, name, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return &hcloud.Network{ID: 1, Name: name, IPRange: ipNet}, nil
	}
	mockInfra.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		if !cpSubnetDone {
			cpSubnetDone = true
			// After CP subnet succeeds, corrupt CIDR so LB subnet calc fails
			cfg.Network.NodeIPv4CIDR = "invalid-cidr"
		}
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionNetwork(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load-balancer subnet")
}

func TestProvisionNetwork_WorkerSubnetCalcError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		Workers: []config.WorkerNodePool{
			{Name: "pool-a", Count: 2, ServerType: "cx21"},
		},
	}
	require.NoError(t, cfg.CalculateSubnets())

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	subnetCount := 0
	mockInfra.EnsureNetworkFunc = func(_ context.Context, name, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return &hcloud.Network{ID: 1, Name: name, IPRange: ipNet}, nil
	}
	mockInfra.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		subnetCount++
		if subnetCount == 2 {
			// After LB subnet succeeds (2nd call), corrupt CIDR so worker calc fails
			cfg.Network.NodeIPv4CIDR = "invalid-cidr"
		}
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionNetwork(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker subnet")
}

// --- Load Balancer: GetSubnetForRole error paths ---

func TestProvisionLoadBalancers_APILBSubnetError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)
	// Corrupt CIDR after subnets calculated to trigger GetSubnetForRole error
	cfg.Network.NodeIPv4CIDR = "invalid-cidr"

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

	p := NewProvisioner()
	err := p.ProvisionLoadBalancers(ctx)

	require.Error(t, err)
}

// --- Load Balancer: full success path with refresh ---

func TestProvisionLoadBalancers_FullSuccessWithRefresh(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	ctx := createTestContext(t, mockInfra, cfg)
	ctx.State.Network = &hcloud.Network{ID: 1, IPRange: ipNet}

	setupLBMock(mockInfra)

	refreshedLB := &hcloud.LoadBalancer{
		ID:   1,
		Name: "test-cluster-kube",
		PublicNet: hcloud.LoadBalancerPublicNet{
			IPv4: hcloud.LoadBalancerPublicNetIPv4{IP: net.ParseIP("5.6.7.8")},
		},
		PrivateNet: []hcloud.LoadBalancerPrivateNet{
			{IP: net.ParseIP("10.0.64.254")},
		},
	}
	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return refreshedLB, nil
	}

	p := NewProvisioner()
	err := p.ProvisionLoadBalancers(ctx)

	require.NoError(t, err)
	assert.Equal(t, refreshedLB, ctx.State.LoadBalancer)
}
