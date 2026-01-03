package hcloud

import (
	"context"
	"net"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// MockClient is a mock implementation of InfrastructureManager.
type MockClient struct {
	CreateServerFunc   func(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string) (string, error)
	DeleteServerFunc   func(ctx context.Context, name string) error
	GetServerIPFunc    func(ctx context.Context, name string) (string, error)
	GetServerIDFunc    func(ctx context.Context, name string) (string, error)
	EnableRescueFunc   func(ctx context.Context, serverID string, sshKeyIDs []string) (string, error)
	ResetServerFunc    func(ctx context.Context, serverID string) error
	PoweroffServerFunc func(ctx context.Context, serverID string) error

	CreateSnapshotFunc func(ctx context.Context, serverID, snapshotDescription string) (string, error)
	DeleteImageFunc    func(ctx context.Context, imageID string) error

	CreateSSHKeyFunc func(ctx context.Context, name, publicKey string) (string, error)
	DeleteSSHKeyFunc func(ctx context.Context, name string) error

	// Network
	EnsureNetworkFunc func(ctx context.Context, name, ipRange, zone string, labels map[string]string) (*hcloud.Network, error)
	EnsureSubnetFunc  func(ctx context.Context, network *hcloud.Network, ipRange, networkZone string, subnetType hcloud.NetworkSubnetType) error
	DeleteNetworkFunc func(ctx context.Context, name string) error
	GetNetworkFunc    func(ctx context.Context, name string) (*hcloud.Network, error)

	// Firewall
	EnsureFirewallFunc func(ctx context.Context, name string, rules []hcloud.FirewallRule, labels map[string]string) (*hcloud.Firewall, error)
	DeleteFirewallFunc func(ctx context.Context, name string) error
	GetFirewallFunc    func(ctx context.Context, name string) (*hcloud.Firewall, error)

	// LoadBalancer
	EnsureLoadBalancerFunc func(ctx context.Context, name, location, lbType string, algorithm hcloud.LoadBalancerAlgorithmType, labels map[string]string) (*hcloud.LoadBalancer, error)
	ConfigureServiceFunc   func(ctx context.Context, lb *hcloud.LoadBalancer, service hcloud.LoadBalancerAddServiceOpts) error
	AttachToNetworkFunc    func(ctx context.Context, lb *hcloud.LoadBalancer, network *hcloud.Network, ip net.IP) error
	DeleteLoadBalancerFunc func(ctx context.Context, name string) error
	GetLoadBalancerFunc    func(ctx context.Context, name string) (*hcloud.LoadBalancer, error)

	// PlacementGroup
	EnsurePlacementGroupFunc func(ctx context.Context, name, pgType string, labels map[string]string) (*hcloud.PlacementGroup, error)
	DeletePlacementGroupFunc func(ctx context.Context, name string) error
	GetPlacementGroupFunc    func(ctx context.Context, name string) (*hcloud.PlacementGroup, error)

	// FloatingIP
	EnsureFloatingIPFunc func(ctx context.Context, name, homeLocation, ipType string, labels map[string]string) (*hcloud.FloatingIP, error)
	DeleteFloatingIPFunc func(ctx context.Context, name string) error
	GetFloatingIPFunc    func(ctx context.Context, name string) (*hcloud.FloatingIP, error)

	// Certificate
	EnsureCertificateFunc func(ctx context.Context, name, certificate, privateKey string, labels map[string]string) (*hcloud.Certificate, error)
	GetCertificateFunc    func(ctx context.Context, name string) (*hcloud.Certificate, error)
	DeleteCertificateFunc func(ctx context.Context, name string) error

	// IP
	GetPublicIPFunc func(ctx context.Context) (string, error)
}

// Ensure interface compliance
var _ InfrastructureManager = (*MockClient)(nil)

// CreateServer mocks server creation.
func (m *MockClient) CreateServer(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string) (string, error) {
	if m.CreateServerFunc != nil {
		return m.CreateServerFunc(ctx, name, imageType, serverType, location, sshKeys, labels, userData)
	}
	return "mock-id", nil
}

// DeleteServer mocks server deletion.
func (m *MockClient) DeleteServer(ctx context.Context, name string) error {
	if m.DeleteServerFunc != nil {
		return m.DeleteServerFunc(ctx, name)
	}
	return nil
}

// GetServerIP mocks getting server IP.
func (m *MockClient) GetServerIP(ctx context.Context, name string) (string, error) {
	if m.GetServerIPFunc != nil {
		return m.GetServerIPFunc(ctx, name)
	}
	return "127.0.0.1", nil
}

// GetServerID mocks getting server ID.
func (m *MockClient) GetServerID(ctx context.Context, name string) (string, error) {
	if m.GetServerIDFunc != nil {
		return m.GetServerIDFunc(ctx, name)
	}
	return "123", nil
}

// EnableRescue mocks enabling rescue mode.
func (m *MockClient) EnableRescue(ctx context.Context, serverID string, sshKeyIDs []string) (string, error) {
	if m.EnableRescueFunc != nil {
		return m.EnableRescueFunc(ctx, serverID, sshKeyIDs)
	}
	return "mock-password", nil
}

// ResetServer mocks resetting server.
func (m *MockClient) ResetServer(ctx context.Context, serverID string) error {
	if m.ResetServerFunc != nil {
		return m.ResetServerFunc(ctx, serverID)
	}
	return nil
}

// PoweroffServer mocks powering off server.
func (m *MockClient) PoweroffServer(ctx context.Context, serverID string) error {
	if m.PoweroffServerFunc != nil {
		return m.PoweroffServerFunc(ctx, serverID)
	}
	return nil
}

// CreateSnapshot mocks snapshot creation.
func (m *MockClient) CreateSnapshot(ctx context.Context, serverID, snapshotDescription string) (string, error) {
	if m.CreateSnapshotFunc != nil {
		return m.CreateSnapshotFunc(ctx, serverID, snapshotDescription)
	}
	return "mock-snapshot-id", nil
}

// DeleteImage mocks image deletion.
func (m *MockClient) DeleteImage(ctx context.Context, imageID string) error {
	if m.DeleteImageFunc != nil {
		return m.DeleteImageFunc(ctx, imageID)
	}
	return nil
}

// CreateSSHKey mocks ssh key creation.
func (m *MockClient) CreateSSHKey(ctx context.Context, name, publicKey string) (string, error) {
	if m.CreateSSHKeyFunc != nil {
		return m.CreateSSHKeyFunc(ctx, name, publicKey)
	}
	return "mock-key-id", nil
}

// DeleteSSHKey mocks ssh key deletion.
func (m *MockClient) DeleteSSHKey(ctx context.Context, name string) error {
	if m.DeleteSSHKeyFunc != nil {
		return m.DeleteSSHKeyFunc(ctx, name)
	}
	return nil
}

// Network mocks
func (m *MockClient) EnsureNetwork(ctx context.Context, name, ipRange, zone string, labels map[string]string) (*hcloud.Network, error) {
	if m.EnsureNetworkFunc != nil {
		return m.EnsureNetworkFunc(ctx, name, ipRange, zone, labels)
	}
	return &hcloud.Network{ID: 1}, nil
}
func (m *MockClient) EnsureSubnet(ctx context.Context, network *hcloud.Network, ipRange, networkZone string, subnetType hcloud.NetworkSubnetType) error {
	if m.EnsureSubnetFunc != nil {
		return m.EnsureSubnetFunc(ctx, network, ipRange, networkZone, subnetType)
	}
	return nil
}
func (m *MockClient) DeleteNetwork(ctx context.Context, name string) error {
	if m.DeleteNetworkFunc != nil {
		return m.DeleteNetworkFunc(ctx, name)
	}
	return nil
}
func (m *MockClient) GetNetwork(ctx context.Context, name string) (*hcloud.Network, error) {
	if m.GetNetworkFunc != nil {
		return m.GetNetworkFunc(ctx, name)
	}
	return nil, nil
}

// Firewall mocks
func (m *MockClient) EnsureFirewall(ctx context.Context, name string, rules []hcloud.FirewallRule, labels map[string]string) (*hcloud.Firewall, error) {
	if m.EnsureFirewallFunc != nil {
		return m.EnsureFirewallFunc(ctx, name, rules, labels)
	}
	return &hcloud.Firewall{ID: 1}, nil
}
func (m *MockClient) DeleteFirewall(ctx context.Context, name string) error {
	if m.DeleteFirewallFunc != nil {
		return m.DeleteFirewallFunc(ctx, name)
	}
	return nil
}
func (m *MockClient) GetFirewall(ctx context.Context, name string) (*hcloud.Firewall, error) {
	if m.GetFirewallFunc != nil {
		return m.GetFirewallFunc(ctx, name)
	}
	return nil, nil
}

// LoadBalancer mocks
func (m *MockClient) EnsureLoadBalancer(ctx context.Context, name, location, lbType string, algorithm hcloud.LoadBalancerAlgorithmType, labels map[string]string) (*hcloud.LoadBalancer, error) {
	if m.EnsureLoadBalancerFunc != nil {
		return m.EnsureLoadBalancerFunc(ctx, name, location, lbType, algorithm, labels)
	}
	return &hcloud.LoadBalancer{ID: 1}, nil
}
func (m *MockClient) ConfigureService(ctx context.Context, lb *hcloud.LoadBalancer, service hcloud.LoadBalancerAddServiceOpts) error {
	if m.ConfigureServiceFunc != nil {
		return m.ConfigureServiceFunc(ctx, lb, service)
	}
	return nil
}
func (m *MockClient) AttachToNetwork(ctx context.Context, lb *hcloud.LoadBalancer, network *hcloud.Network, ip net.IP) error {
	if m.AttachToNetworkFunc != nil {
		return m.AttachToNetworkFunc(ctx, lb, network, ip)
	}
	return nil
}
func (m *MockClient) DeleteLoadBalancer(ctx context.Context, name string) error {
	if m.DeleteLoadBalancerFunc != nil {
		return m.DeleteLoadBalancerFunc(ctx, name)
	}
	return nil
}
func (m *MockClient) GetLoadBalancer(ctx context.Context, name string) (*hcloud.LoadBalancer, error) {
	if m.GetLoadBalancerFunc != nil {
		return m.GetLoadBalancerFunc(ctx, name)
	}
	return nil, nil
}

// PlacementGroup mocks
func (m *MockClient) EnsurePlacementGroup(ctx context.Context, name, pgType string, labels map[string]string) (*hcloud.PlacementGroup, error) {
	if m.EnsurePlacementGroupFunc != nil {
		return m.EnsurePlacementGroupFunc(ctx, name, pgType, labels)
	}
	return &hcloud.PlacementGroup{ID: 1}, nil
}
func (m *MockClient) DeletePlacementGroup(ctx context.Context, name string) error {
	if m.DeletePlacementGroupFunc != nil {
		return m.DeletePlacementGroupFunc(ctx, name)
	}
	return nil
}
func (m *MockClient) GetPlacementGroup(ctx context.Context, name string) (*hcloud.PlacementGroup, error) {
	if m.GetPlacementGroupFunc != nil {
		return m.GetPlacementGroupFunc(ctx, name)
	}
	return nil, nil
}

// FloatingIP mocks
func (m *MockClient) EnsureFloatingIP(ctx context.Context, name, homeLocation, ipType string, labels map[string]string) (*hcloud.FloatingIP, error) {
	if m.EnsureFloatingIPFunc != nil {
		return m.EnsureFloatingIPFunc(ctx, name, homeLocation, ipType, labels)
	}
	return &hcloud.FloatingIP{ID: 1}, nil
}
func (m *MockClient) DeleteFloatingIP(ctx context.Context, name string) error {
	if m.DeleteFloatingIPFunc != nil {
		return m.DeleteFloatingIPFunc(ctx, name)
	}
	return nil
}
func (m *MockClient) GetFloatingIP(ctx context.Context, name string) (*hcloud.FloatingIP, error) {
	if m.GetFloatingIPFunc != nil {
		return m.GetFloatingIPFunc(ctx, name)
	}
	return nil, nil
}

// Certificate mocks
func (m *MockClient) EnsureCertificate(ctx context.Context, name, certificate, privateKey string, labels map[string]string) (*hcloud.Certificate, error) {
	if m.EnsureCertificateFunc != nil {
		return m.EnsureCertificateFunc(ctx, name, certificate, privateKey, labels)
	}
	return &hcloud.Certificate{ID: 1}, nil
}
func (m *MockClient) GetCertificate(ctx context.Context, name string) (*hcloud.Certificate, error) {
	if m.GetCertificateFunc != nil {
		return m.GetCertificateFunc(ctx, name)
	}
	return nil, nil
}
func (m *MockClient) DeleteCertificate(ctx context.Context, name string) error {
	if m.DeleteCertificateFunc != nil {
		return m.DeleteCertificateFunc(ctx, name)
	}
	return nil
}

// GetPublicIP mocks
func (m *MockClient) GetPublicIP(ctx context.Context) (string, error) {
	if m.GetPublicIPFunc != nil {
		return m.GetPublicIPFunc(ctx)
	}
	return "127.0.0.1", nil
}
