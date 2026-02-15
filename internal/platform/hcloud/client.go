// Package hcloud provides a wrapper around the Hetzner Cloud API.
package hcloud

import (
	"context"
	"net"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// ServerCreateOpts holds all parameters for creating an HCloud server.
type ServerCreateOpts struct {
	Name             string
	ImageType        string
	ServerType       string
	Location         string
	SSHKeys          []string
	Labels           map[string]string
	UserData         string
	PlacementGroupID *int64
	NetworkID        int64
	PrivateIP        string
	EnablePublicIPv4 bool
	EnablePublicIPv6 bool
}

// ServerProvisioner defines the interface for provisioning servers.
type ServerProvisioner interface {
	// CreateServer creates a new server with the given specifications.
	// PublicIPv4/IPv6 control public IP assignment:
	// - both true: dual-stack (default)
	// - IPv4=false, IPv6=true: IPv6-only
	// - both false: private network only
	CreateServer(ctx context.Context, opts ServerCreateOpts) (string, error)
	DeleteServer(ctx context.Context, name string) error
	// GetServerIP returns the public IP of the server.
	// Prefers IPv4 for backwards compatibility, falls back to IPv6 if no IPv4.
	GetServerIP(ctx context.Context, name string) (string, error)
	GetServersByLabel(ctx context.Context, labels map[string]string) ([]*hcloud.Server, error)
	EnableRescue(ctx context.Context, serverID string, sshKeyIDs []string) (string, error)
	ResetServer(ctx context.Context, serverID string) error
	PoweroffServer(ctx context.Context, serverID string) error
	GetServerID(ctx context.Context, name string) (string, error)
	// GetServerByName returns the full server object by name, or nil if not found.
	GetServerByName(ctx context.Context, name string) (*hcloud.Server, error)
	// AttachServerToNetwork attaches an existing server to a network.
	// The server must exist and not already be attached to the network.
	AttachServerToNetwork(ctx context.Context, serverName string, networkID int64, privateIP string) error
}

// SnapshotManager defines the interface for managing snapshots.
type SnapshotManager interface {
	CreateSnapshot(ctx context.Context, serverID, snapshotDescription string, labels map[string]string) (string, error)
	DeleteImage(ctx context.Context, imageID string) error
	GetSnapshotByLabels(ctx context.Context, labels map[string]string) (*hcloud.Image, error)

	// CleanupByLabel deletes all resources matching the given label selector
	CleanupByLabel(ctx context.Context, labelSelector map[string]string) error
}

// SSHKeyManager defines the interface for managing SSH keys.
type SSHKeyManager interface {
	CreateSSHKey(ctx context.Context, name, publicKey string, labels map[string]string) (string, error)
	DeleteSSHKey(ctx context.Context, name string) error
}

// NetworkManager defines the interface for managing networks.
type NetworkManager interface {
	EnsureNetwork(ctx context.Context, name, ipRange, zone string, labels map[string]string) (*hcloud.Network, error)
	EnsureSubnet(ctx context.Context, network *hcloud.Network, ipRange, networkZone string, subnetType hcloud.NetworkSubnetType) error
	DeleteNetwork(ctx context.Context, name string) error
	GetNetwork(ctx context.Context, name string) (*hcloud.Network, error)
}

// FirewallManager defines the interface for managing firewalls.
type FirewallManager interface {
	EnsureFirewall(ctx context.Context, name string, rules []hcloud.FirewallRule, labels map[string]string, applyToLabelSelector string) (*hcloud.Firewall, error)
	DeleteFirewall(ctx context.Context, name string) error
	GetFirewall(ctx context.Context, name string) (*hcloud.Firewall, error)
}

// LoadBalancerManager defines the interface for managing load balancers.
type LoadBalancerManager interface {
	EnsureLoadBalancer(ctx context.Context, name, location, lbType string, algorithm hcloud.LoadBalancerAlgorithmType, labels map[string]string) (*hcloud.LoadBalancer, error)
	ConfigureService(ctx context.Context, lb *hcloud.LoadBalancer, service hcloud.LoadBalancerAddServiceOpts) error
	AddTarget(ctx context.Context, lb *hcloud.LoadBalancer, targetType hcloud.LoadBalancerTargetType, labelSelector string) error
	AttachToNetwork(ctx context.Context, lb *hcloud.LoadBalancer, network *hcloud.Network, ip net.IP) error
	DeleteLoadBalancer(ctx context.Context, name string) error
	GetLoadBalancer(ctx context.Context, name string) (*hcloud.LoadBalancer, error)
}

// PlacementGroupManager defines the interface for managing placement groups.
type PlacementGroupManager interface {
	EnsurePlacementGroup(ctx context.Context, name, pgType string, labels map[string]string) (*hcloud.PlacementGroup, error)
	DeletePlacementGroup(ctx context.Context, name string) error
	GetPlacementGroup(ctx context.Context, name string) (*hcloud.PlacementGroup, error)
}

// CertificateManager defines the interface for managing certificates.
type CertificateManager interface {
	EnsureCertificate(ctx context.Context, name, certificate, privateKey string, labels map[string]string) (*hcloud.Certificate, error)
	GetCertificate(ctx context.Context, name string) (*hcloud.Certificate, error)
	DeleteCertificate(ctx context.Context, name string) error
}

// RDNSManager defines the interface for managing reverse DNS.
type RDNSManager interface {
	SetServerRDNS(ctx context.Context, serverID int64, ipAddress, dnsPtr string) error
	SetLoadBalancerRDNS(ctx context.Context, lbID int64, ipAddress, dnsPtr string) error
}

// InfrastructureManager combines all infrastructure interfaces.
type InfrastructureManager interface {
	ServerProvisioner
	SnapshotManager
	SSHKeyManager
	NetworkManager
	FirewallManager
	LoadBalancerManager
	PlacementGroupManager
	CertificateManager
	RDNSManager
	GetPublicIP(ctx context.Context) (string, error)
}
