// Package controller contains the Kubernetes controllers for the k8zner operator.
package controller

import (
	"context"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// HCloudClient defines the interface for Hetzner Cloud operations.
// This interface enables testing with mocks.
type HCloudClient interface {
	// Server operations
	CreateServer(ctx context.Context, name, imageType, serverType, location string,
		sshKeys []string, labels map[string]string, userData string,
		placementGroupID *int64, networkID int64, privateIP string,
		enablePublicIPv4, enablePublicIPv6 bool) (string, error)
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

// TalosConfigGenerator defines the interface for generating Talos machine configs.
// This interface is a subset of provisioning.TalosConfigProducer and uses the same method names.
type TalosConfigGenerator interface {
	// GenerateControlPlaneConfig generates a Talos config for a control plane node.
	GenerateControlPlaneConfig(sans []string, hostname string, serverID int64) ([]byte, error)

	// GenerateWorkerConfig generates a Talos config for a worker node.
	GenerateWorkerConfig(hostname string, serverID int64) ([]byte, error)

	// SetEndpoint updates the control plane endpoint.
	SetEndpoint(endpoint string)

	// GetClientConfig returns the Talos client configuration for cluster access.
	GetClientConfig() ([]byte, error)
}

// TalosClient defines the interface for Talos API operations.
type TalosClient interface {
	// ApplyConfig applies a machine configuration to a node.
	ApplyConfig(ctx context.Context, nodeIP string, config []byte) error

	// IsNodeInMaintenanceMode checks if a node is unconfigured.
	IsNodeInMaintenanceMode(ctx context.Context, nodeIP string) (bool, error)

	// GetEtcdMembers returns the list of etcd members.
	GetEtcdMembers(ctx context.Context, nodeIP string) ([]EtcdMember, error)

	// RemoveEtcdMember removes a member from the etcd cluster.
	RemoveEtcdMember(ctx context.Context, nodeIP string, memberID string) error

	// WaitForNodeReady waits for a node to become ready.
	WaitForNodeReady(ctx context.Context, nodeIP string, timeout int) error
}

// EtcdMember represents an etcd cluster member.
type EtcdMember struct {
	ID       string
	Name     string
	Endpoint string
	IsLeader bool
}

// NodeReplacer handles the logic for replacing nodes.
type NodeReplacer interface {
	// ReplaceControlPlane replaces an unhealthy control plane node.
	ReplaceControlPlane(ctx context.Context, cluster *ClusterState, node *NodeInfo) error

	// ReplaceWorker replaces an unhealthy worker node.
	ReplaceWorker(ctx context.Context, cluster *ClusterState, node *NodeInfo) error
}

// ClusterState holds the current state of a cluster for node operations.
type ClusterState struct {
	Name           string
	Region         string
	NetworkID      int64
	ControlPlaneIP string // Load balancer IP
	SANs           []string
	SSHKeyIDs      []string
	Labels         map[string]string
}

// NodeInfo holds information about a node to be replaced.
type NodeInfo struct {
	Name       string
	ServerID   int64
	PrivateIP  string
	Role       string // "control-plane" or "worker"
	Pool       string
	ServerType string
}

// HealthChecker checks the health of cluster components.
type HealthChecker interface {
	// CheckNodeHealth returns the health status of a Kubernetes node.
	CheckNodeHealth(ctx context.Context, nodeName string) (*NodeHealth, error)

	// CheckEtcdHealth returns the health status of the etcd cluster.
	CheckEtcdHealth(ctx context.Context, controlPlaneIP string) (*EtcdHealth, error)
}

// NodeHealth represents the health status of a node.
type NodeHealth struct {
	Ready            bool
	MemoryPressure   bool
	DiskPressure     bool
	PIDPressure      bool
	NetworkAvailable bool
	Reason           string
}

// EtcdHealth represents the health status of the etcd cluster.
type EtcdHealth struct {
	Healthy     bool
	MemberCount int
	LeaderID    string
	Members     []EtcdMember
}
