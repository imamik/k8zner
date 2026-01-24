package testing

import (
	"context"
	"net"

	hcloud_internal "k8zner/internal/platform/hcloud"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// InfraFixture provides pre-configured mock infrastructure for common test scenarios.
type InfraFixture struct {
	mock *hcloud_internal.MockClient
}

// NewInfraFixture creates a new infrastructure fixture.
func NewInfraFixture() *InfraFixture {
	return &InfraFixture{
		mock: &hcloud_internal.MockClient{},
	}
}

// Mock returns the underlying MockClient for custom configuration.
func (f *InfraFixture) Mock() *hcloud_internal.MockClient {
	return f.mock
}

// SuccessfulProvisioning configures the mock for a successful provisioning scenario.
// Returns the same mock for chaining.
func (f *InfraFixture) SuccessfulProvisioning() *hcloud_internal.MockClient {
	f.mock.EnsureNetworkFunc = func(_ context.Context, _, ipRange, _ string, _ map[string]string) (*hcloud.Network, error) {
		_, ipNet, _ := net.ParseCIDR(ipRange)
		return &hcloud.Network{ID: 1, IPRange: ipNet}, nil
	}
	f.mock.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		return nil
	}
	f.mock.GetPublicIPFunc = func(_ context.Context) (string, error) {
		return "1.2.3.4", nil
	}
	f.mock.EnsureFirewallFunc = func(_ context.Context, _ string, _ []hcloud.FirewallRule, _ map[string]string, _ string) (*hcloud.Firewall, error) {
		return &hcloud.Firewall{ID: 1}, nil
	}
	f.mock.EnsureLoadBalancerFunc = func(_ context.Context, _, _, _ string, _ hcloud.LoadBalancerAlgorithmType, _ map[string]string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID: 1,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("5.6.7.8"),
				},
			},
		}, nil
	}
	f.mock.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID: 1,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("5.6.7.8"),
				},
			},
		}, nil
	}
	f.mock.EnsurePlacementGroupFunc = func(_ context.Context, _, _ string, _ map[string]string) (*hcloud.PlacementGroup, error) {
		return &hcloud.PlacementGroup{ID: 1}, nil
	}
	f.mock.EnsureFloatingIPFunc = func(_ context.Context, _, _, _ string, _ map[string]string) (*hcloud.FloatingIP, error) {
		return &hcloud.FloatingIP{ID: 1}, nil
	}
	f.mock.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "", nil // Does not exist
	}
	f.mock.CreateServerFunc = func(_ context.Context, _, _, _, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string) (string, error) {
		return "server-id", nil
	}
	f.mock.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.1.2", nil
	}
	f.mock.GetSnapshotByLabelsFunc = func(_ context.Context, labels map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{
			ID:          123,
			Description: "talos-v1.8.3-k8s-v1.31.0-amd64",
			Labels:      labels,
		}, nil
	}
	f.mock.GetCertificateFunc = func(_ context.Context, name string) (*hcloud.Certificate, error) {
		// Default: cluster is already bootstrapped (state marker exists)
		if name == "test-cluster-state" {
			return &hcloud.Certificate{ID: 99, Name: "test-cluster-state"}, nil
		}
		return nil, nil
	}

	return f.mock
}

// NetworkOnly configures the mock for network-related tests only.
func (f *InfraFixture) NetworkOnly() *hcloud_internal.MockClient {
	f.mock.EnsureNetworkFunc = func(_ context.Context, _, ipRange, _ string, _ map[string]string) (*hcloud.Network, error) {
		_, ipNet, _ := net.ParseCIDR(ipRange)
		return &hcloud.Network{ID: 1, IPRange: ipNet}, nil
	}
	f.mock.EnsureSubnetFunc = func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
		return nil
	}
	f.mock.GetPublicIPFunc = func(_ context.Context) (string, error) {
		return "1.2.3.4", nil
	}
	return f.mock
}

// WithNetworkError configures the mock to fail on network creation.
func (f *InfraFixture) WithNetworkError(err error) *hcloud_internal.MockClient {
	f.mock.EnsureNetworkFunc = func(_ context.Context, _, _, _ string, _ map[string]string) (*hcloud.Network, error) {
		return nil, err
	}
	return f.mock
}

// WithServerError configures the mock to fail on server creation.
func (f *InfraFixture) WithServerError(err error) *hcloud_internal.MockClient {
	f.NetworkOnly()
	f.mock.EnsureFirewallFunc = func(_ context.Context, _ string, _ []hcloud.FirewallRule, _ map[string]string, _ string) (*hcloud.Firewall, error) {
		return &hcloud.Firewall{}, nil
	}
	f.mock.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return nil, nil
	}
	f.mock.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "", nil
	}
	f.mock.EnsurePlacementGroupFunc = func(_ context.Context, _, _ string, _ map[string]string) (*hcloud.PlacementGroup, error) {
		return &hcloud.PlacementGroup{ID: 1}, nil
	}
	f.mock.CreateServerFunc = func(_ context.Context, _, _, _, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string) (string, error) {
		return "", err
	}
	f.mock.GetSnapshotByLabelsFunc = func(_ context.Context, labels map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{
			ID:          123,
			Description: "talos-v1.8.3-k8s-v1.31.0-amd64",
			Labels:      labels,
		}, nil
	}
	return f.mock
}
