// Package hcloud provides a wrapper around the Hetzner Cloud API.
package hcloud

import (
	"context"
)

// NetworkManager handles network operations.
type NetworkManager interface {
	EnsureNetwork(ctx context.Context, name, ipRange string, labels map[string]string) error
	DeleteNetwork(ctx context.Context, name string) error
}

// FirewallManager handles firewall operations.
type FirewallManager interface {
	EnsureFirewall(ctx context.Context, name string, rules []FirewallRule, labels map[string]string) error
	DeleteFirewall(ctx context.Context, name string) error
}

// FirewallRule represents a simplified firewall rule.
type FirewallRule struct {
	Direction string
	Port      string
	Protocol  string
	SourceIPs []string
}

// LoadBalancerManager handles load balancer operations.
type LoadBalancerManager interface {
	EnsureLoadBalancer(ctx context.Context, name, networkName, ip string, labels map[string]string) error
	DeleteLoadBalancer(ctx context.Context, name string) error
}

// PlacementGroupManager handles placement group operations.
type PlacementGroupManager interface {
	EnsurePlacementGroup(ctx context.Context, name string, labels map[string]string) error
}

// FloatingIPManager handles floating IP operations.
type FloatingIPManager interface {
	EnsureFloatingIP(ctx context.Context, name string, labels map[string]string) (string, error)
}

// ServerProvisioner defines the interface for provisioning servers.
type ServerProvisioner interface {
	CreateServer(ctx context.Context, name, imageType, serverType string, sshKeys []string, labels map[string]string) (string, error)
	DeleteServer(ctx context.Context, name string) error
	GetServerIP(ctx context.Context, name string) (string, error)
	EnableRescue(ctx context.Context, serverID string, sshKeyIDs []string) (string, error)
	ResetServer(ctx context.Context, serverID string) error
	PoweroffServer(ctx context.Context, serverID string) error
	GetServerID(ctx context.Context, name string) (string, error)
}

// SnapshotManager defines the interface for managing snapshots.
type SnapshotManager interface {
	CreateSnapshot(ctx context.Context, serverID, snapshotDescription string) (string, error)
	DeleteImage(ctx context.Context, imageID string) error
}

// SSHKeyManager defines the interface for managing SSH keys.
type SSHKeyManager interface {
	CreateSSHKey(ctx context.Context, name, publicKey string) (string, error)
	DeleteSSHKey(ctx context.Context, name string) error
}
