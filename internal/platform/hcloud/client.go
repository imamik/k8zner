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

// InfrastructureManager defines the interface for all Hetzner Cloud infrastructure operations.
type InfrastructureManager interface {
	// Server operations
	CreateServer(ctx context.Context, opts ServerCreateOpts) (string, error)
	DeleteServer(ctx context.Context, name string) error
	GetServerIP(ctx context.Context, name string) (string, error)
	GetServersByLabel(ctx context.Context, labels map[string]string) ([]*hcloud.Server, error)
	EnableRescue(ctx context.Context, serverID string, sshKeyIDs []string) (string, error)
	ResetServer(ctx context.Context, serverID string) error
	PoweroffServer(ctx context.Context, serverID string) error
	GetServerID(ctx context.Context, name string) (string, error)
	GetServerByName(ctx context.Context, name string) (*hcloud.Server, error)
	AttachServerToNetwork(ctx context.Context, serverName string, networkID int64, privateIP string) error

	// Snapshot operations
	CreateSnapshot(ctx context.Context, serverID, snapshotDescription string, labels map[string]string) (string, error)
	DeleteImage(ctx context.Context, imageID string) error
	GetSnapshotByLabels(ctx context.Context, labels map[string]string) (*hcloud.Image, error)
	CleanupByLabel(ctx context.Context, labelSelector map[string]string) error

	// SSH key operations
	CreateSSHKey(ctx context.Context, name, publicKey string, labels map[string]string) (string, error)
	DeleteSSHKey(ctx context.Context, name string) error

	// Network operations
	EnsureNetwork(ctx context.Context, name, ipRange, zone string, labels map[string]string) (*hcloud.Network, error)
	EnsureSubnet(ctx context.Context, network *hcloud.Network, ipRange, networkZone string, subnetType hcloud.NetworkSubnetType) error
	DeleteNetwork(ctx context.Context, name string) error
	GetNetwork(ctx context.Context, name string) (*hcloud.Network, error)

	// Firewall operations
	EnsureFirewall(ctx context.Context, name string, rules []hcloud.FirewallRule, labels map[string]string, applyToLabelSelector string) (*hcloud.Firewall, error)
	DeleteFirewall(ctx context.Context, name string) error
	GetFirewall(ctx context.Context, name string) (*hcloud.Firewall, error)

	// Load balancer operations
	EnsureLoadBalancer(ctx context.Context, name, location, lbType string, algorithm hcloud.LoadBalancerAlgorithmType, labels map[string]string) (*hcloud.LoadBalancer, error)
	ConfigureService(ctx context.Context, lb *hcloud.LoadBalancer, service hcloud.LoadBalancerAddServiceOpts) error
	AddTarget(ctx context.Context, lb *hcloud.LoadBalancer, targetType hcloud.LoadBalancerTargetType, labelSelector string) error
	AttachToNetwork(ctx context.Context, lb *hcloud.LoadBalancer, network *hcloud.Network, ip net.IP) error
	DeleteLoadBalancer(ctx context.Context, name string) error
	GetLoadBalancer(ctx context.Context, name string) (*hcloud.LoadBalancer, error)

	// Placement group operations
	EnsurePlacementGroup(ctx context.Context, name, pgType string, labels map[string]string) (*hcloud.PlacementGroup, error)
	DeletePlacementGroup(ctx context.Context, name string) error
	GetPlacementGroup(ctx context.Context, name string) (*hcloud.PlacementGroup, error)

	// Certificate operations
	EnsureCertificate(ctx context.Context, name, certificate, privateKey string, labels map[string]string) (*hcloud.Certificate, error)
	GetCertificate(ctx context.Context, name string) (*hcloud.Certificate, error)
	DeleteCertificate(ctx context.Context, name string) error

	// Public IP
	GetPublicIP(ctx context.Context) (string, error)
}
