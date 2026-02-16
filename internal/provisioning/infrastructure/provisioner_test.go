package infrastructure

import (
	"context"
	"net"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"

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

func TestProvisionNetwork_Success(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

	mockInfra.EnsureFirewallFunc = func(_ context.Context, name string, _ []hcloud.FirewallRule, _ map[string]string, applyToLabelSelector string) (*hcloud.Firewall, error) {
		assert.Contains(t, name, "test-cluster")
		assert.Equal(t, "cluster=test-cluster", applyToLabelSelector)
		// Rules may be empty if no control plane or workers configured
		return &hcloud.Firewall{ID: 1, Name: name}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.ProvisionFirewall(ctx)
	assert.NoError(t, err)
}

func TestProvisionLoadBalancers_Success(t *testing.T) {
	t.Parallel()
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

func TestGetNetwork(t *testing.T) {
	t.Parallel()
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

func TestParseProtocol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected hcloud.FirewallRuleProtocol
	}{
		{"tcp", "tcp", hcloud.FirewallRuleProtocolTCP},
		{"udp", "udp", hcloud.FirewallRuleProtocolUDP},
		{"icmp", "icmp", hcloud.FirewallRuleProtocolICMP},
		{"gre", "gre", hcloud.FirewallRuleProtocolGRE},
		{"esp", "esp", hcloud.FirewallRuleProtocolESP},
		{"unknown defaults to tcp", "unknown", hcloud.FirewallRuleProtocolTCP},
		{"empty defaults to tcp", "", hcloud.FirewallRuleProtocolTCP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := parseProtocol(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseCIDRs(t *testing.T) {
	t.Parallel()
	t.Run("valid CIDRs", func(t *testing.T) {
		t.Parallel()
		cidrs := []string{"10.0.0.0/8", "192.168.1.0/24", "172.16.0.0/12"}
		result := parseCIDRs(cidrs)
		assert.Len(t, result, 3)
	})

	t.Run("mixed valid and invalid CIDRs", func(t *testing.T) {
		t.Parallel()
		cidrs := []string{"10.0.0.0/8", "invalid", "192.168.1.0/24"}
		result := parseCIDRs(cidrs)
		assert.Len(t, result, 2, "invalid CIDRs should be skipped")
	})

	t.Run("all invalid CIDRs", func(t *testing.T) {
		t.Parallel()
		cidrs := []string{"invalid", "also-invalid", "not-a-cidr"}
		result := parseCIDRs(cidrs)
		assert.Empty(t, result)
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()
		result := parseCIDRs([]string{})
		assert.Empty(t, result)
	})
}

func TestCollectAPISources(t *testing.T) {
	t.Parallel()
	t.Run("specific sources take precedence", func(t *testing.T) {
		t.Parallel()
		specific := []string{"10.0.0.0/8"}
		fallback := []string{"192.168.0.0/16"}
		result := collectAPISources(specific, fallback, "", nil)
		assert.Equal(t, []string{"10.0.0.0/8"}, result)
	})

	t.Run("fallback used when specific is empty", func(t *testing.T) {
		t.Parallel()
		specific := []string{}
		fallback := []string{"192.168.0.0/16"}
		result := collectAPISources(specific, fallback, "", nil)
		assert.Equal(t, []string{"192.168.0.0/16"}, result)
	})

	t.Run("public IP appended when useCurrentIP is true", func(t *testing.T) {
		t.Parallel()
		specific := []string{"10.0.0.0/8"}
		useIP := true
		result := collectAPISources(specific, nil, "1.2.3.4", &useIP)
		assert.Contains(t, result, "10.0.0.0/8")
		assert.Contains(t, result, "1.2.3.4/32")
	})

	t.Run("public IP not appended when useCurrentIP is false", func(t *testing.T) {
		t.Parallel()
		specific := []string{"10.0.0.0/8"}
		useIP := false
		result := collectAPISources(specific, nil, "1.2.3.4", &useIP)
		assert.Equal(t, []string{"10.0.0.0/8"}, result)
	})

	t.Run("public IP not appended when nil", func(t *testing.T) {
		t.Parallel()
		specific := []string{"10.0.0.0/8"}
		result := collectAPISources(specific, nil, "1.2.3.4", nil)
		assert.Equal(t, []string{"10.0.0.0/8"}, result)
	})

	t.Run("empty public IP not appended", func(t *testing.T) {
		t.Parallel()
		specific := []string{"10.0.0.0/8"}
		useIP := true
		result := collectAPISources(specific, nil, "", &useIP)
		assert.Equal(t, []string{"10.0.0.0/8"}, result)
	})
}

func TestBuildFirewallRule(t *testing.T) {
	t.Parallel()
	t.Run("inbound TCP rule", func(t *testing.T) {
		t.Parallel()
		rule := config.FirewallRule{
			Description: "Test rule",
			Direction:   "in",
			Protocol:    "tcp",
			Port:        "443",
			SourceIPs:   []string{"10.0.0.0/8"},
		}
		result := buildFirewallRule(rule)

		assert.Equal(t, "Test rule", *result.Description)
		assert.Equal(t, hcloud.FirewallRuleDirectionIn, result.Direction)
		assert.Equal(t, hcloud.FirewallRuleProtocolTCP, result.Protocol)
		assert.Equal(t, "443", *result.Port)
		assert.Len(t, result.SourceIPs, 1)
	})

	t.Run("outbound UDP rule", func(t *testing.T) {
		t.Parallel()
		rule := config.FirewallRule{
			Description:    "Outbound DNS",
			Direction:      "out",
			Protocol:       "udp",
			Port:           "53",
			DestinationIPs: []string{"8.8.8.8/32", "8.8.4.4/32"},
		}
		result := buildFirewallRule(rule)

		assert.Equal(t, hcloud.FirewallRuleDirectionOut, result.Direction)
		assert.Equal(t, hcloud.FirewallRuleProtocolUDP, result.Protocol)
		assert.Len(t, result.DestinationIPs, 2)
	})

	t.Run("rule without port", func(t *testing.T) {
		t.Parallel()
		rule := config.FirewallRule{
			Description: "ICMP rule",
			Direction:   "in",
			Protocol:    "icmp",
			SourceIPs:   []string{"0.0.0.0/0"},
		}
		result := buildFirewallRule(rule)

		assert.Nil(t, result.Port)
		assert.Equal(t, hcloud.FirewallRuleProtocolICMP, result.Protocol)
	})
}

func TestProvisionNetwork_WithWorkers(t *testing.T) {
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
			{Name: "pool1", Count: 2, ServerType: "cx21"},
			{Name: "pool2", Count: 1, ServerType: "cx21"},
		},
	}
	require.NoError(t, cfg.CalculateSubnets())

	_, ipNet, err := net.ParseCIDR("10.0.0.0/16")
	require.NoError(t, err)

	var subnetCount int
	mockInfra.EnsureNetworkFunc = func(_ context.Context, name, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return &hcloud.Network{ID: 1, Name: name, IPRange: ipNet}, nil
	}
	mockInfra.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _ string, _ string, _ hcloud.NetworkSubnetType) error {
		subnetCount++
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err = p.ProvisionNetwork(ctx)
	assert.NoError(t, err)
	// Should create: 1 CP subnet + 1 LB subnet + 2 worker subnets = 4 subnets
	assert.Equal(t, 4, subnetCount)
}
