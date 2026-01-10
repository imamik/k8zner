// Package provisioning provides shared types and interfaces for cluster provisioning.
//
// The provisioning domain is organized into focused subpackages:
//   - infrastructure/ — Network, Firewall, Load Balancers, Floating IPs
//   - compute/ — Servers, Control Plane, Workers, Node Pools
//   - image/ — Talos image building and snapshot management
//   - cluster/ — Bootstrap and Talos configuration application
//
// This root package contains shared interfaces and state types used across subpackages.
package provisioning

// Phase defines the interface for a provisioning phase.
type Phase interface {
	// Name returns the human-readable name of this phase.
	Name() string

	// Provision executes the provisioning logic for this phase.
	Provision(ctx *Context) error
}

// TalosConfigProducer defines the interface for generating Talos configurations.
// Implemented by internal/platform/talos.Generator.
type TalosConfigProducer interface {
	// GenerateControlPlaneConfig generates machine configuration for a control plane node.
	GenerateControlPlaneConfig(san []string, hostname string) ([]byte, error)

	// GenerateWorkerConfig generates machine configuration for a worker node.
	GenerateWorkerConfig(hostname string) ([]byte, error)

	// GetClientConfig returns the Talos client configuration for cluster access.
	GetClientConfig() ([]byte, error)

	// SetEndpoint updates the cluster endpoint (e.g., load balancer IP).
	SetEndpoint(endpoint string)
}
