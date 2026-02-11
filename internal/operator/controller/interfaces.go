// Package controller contains the Kubernetes controllers for the k8zner operator.
package controller

import (
	"context"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"

	"github.com/imamik/k8zner/internal/platform/hcloud"
)

// hcloudClient defines the interface for Hetzner Cloud operations.
// This interface enables testing with mocks.
type hcloudClient interface {
	// Server operations
	CreateServer(ctx context.Context, opts hcloud.ServerCreateOpts) (string, error)
	DeleteServer(ctx context.Context, name string) error
	GetServerIP(ctx context.Context, name string) (string, error)
	GetServerID(ctx context.Context, name string) (string, error)
	GetServerByName(ctx context.Context, name string) (*hcloudgo.Server, error)
	GetServersByLabel(ctx context.Context, labels map[string]string) ([]*hcloudgo.Server, error)

	// SSH Key operations (for ephemeral keys to avoid password emails)
	CreateSSHKey(ctx context.Context, name, publicKey string, labels map[string]string) (string, error)
	DeleteSSHKey(ctx context.Context, name string) error

	// Network operations
	GetNetwork(ctx context.Context, name string) (*hcloudgo.Network, error)

	// Image operations
	GetSnapshotByLabels(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error)
}

// talosConfigGenerator defines the interface for generating Talos machine configs.
// This interface is a subset of provisioning.TalosConfigProducer and uses the same method names.
type talosConfigGenerator interface {
	// GenerateControlPlaneConfig generates a Talos config for a control plane node.
	GenerateControlPlaneConfig(sans []string, hostname string, serverID int64) ([]byte, error)

	// GenerateWorkerConfig generates a Talos config for a worker node.
	GenerateWorkerConfig(hostname string, serverID int64) ([]byte, error)

	// SetEndpoint updates the control plane endpoint.
	SetEndpoint(endpoint string)

	// GetClientConfig returns the Talos client configuration for cluster access.
	GetClientConfig() ([]byte, error)
}

// talosClient defines the interface for Talos API operations.
type talosClient interface {
	// ApplyConfig applies a machine configuration to a node.
	ApplyConfig(ctx context.Context, nodeIP string, config []byte) error

	// IsNodeInMaintenanceMode checks if a node is unconfigured.
	IsNodeInMaintenanceMode(ctx context.Context, nodeIP string) (bool, error)

	// GetEtcdMembers returns the list of etcd members.
	GetEtcdMembers(ctx context.Context, nodeIP string) ([]etcdMember, error)

	// RemoveEtcdMember removes a member from the etcd cluster.
	RemoveEtcdMember(ctx context.Context, nodeIP string, memberID string) error

	// WaitForNodeReady waits for a node to become ready.
	WaitForNodeReady(ctx context.Context, nodeIP string, timeout int) error
}

// etcdMember represents an etcd cluster member.
type etcdMember struct {
	ID       string
	Name     string
	Endpoint string
	IsLeader bool
}

// clusterState holds the current state of a cluster for node operations.
type clusterState struct {
	Name           string
	Region         string
	NetworkID      int64
	ControlPlaneIP string // Load balancer IP
	SANs           []string
	SSHKeyIDs      []string
	Labels         map[string]string
}
