package provisioning

import (
	"context"

	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// State holds the shared results of provisioning phases.
// It is progressively populated as each phase completes and is passed
// to subsequent phases that need earlier results.
type State struct {
	// Infrastructure results (populated by infrastructure provisioner)
	Network      *hcloud.Network
	Firewall     *hcloud.Firewall
	LoadBalancer *hcloud.LoadBalancer // API load balancer (for control plane)
	PublicIP     string               // Current execution environment's public IPv4
	SSHKeyID     int64                // SSH key ID (for operator server creation)

	// Compute results (populated by compute provisioner)
	ControlPlaneIPs       map[string]string // nodeName -> publicIP
	WorkerIPs             map[string]string // nodeName -> publicIP
	ControlPlaneServerIDs map[string]int64  // nodeName -> serverID (for nodeid label)
	WorkerServerIDs       map[string]int64  // nodeName -> serverID (for nodeid label)
	SANs                  []string          // Subject Alternative Names for certs

	// Cluster results (populated by cluster bootstrapper)
	Kubeconfig  []byte
	TalosConfig []byte
}

// NewState creates an empty provisioning state.
func NewState() *State {
	return &State{
		ControlPlaneIPs:       make(map[string]string),
		WorkerIPs:             make(map[string]string),
		ControlPlaneServerIDs: make(map[string]int64),
		WorkerServerIDs:       make(map[string]int64),
	}
}

// Context wraps all dependencies and state needed for a provisioning phase.
type Context struct {
	context.Context
	Config   *config.Config
	State    *State
	Infra    hcloud_internal.InfrastructureManager
	Talos    TalosConfigProducer
	Observer Observer
	Timeouts *config.Timeouts
}

// NewContext creates a new provisioning context.
func NewContext(
	ctx context.Context,
	cfg *config.Config,
	infra hcloud_internal.InfrastructureManager,
	talos TalosConfigProducer,
) *Context {
	observer := NewConsoleObserver()
	return &Context{
		Context:  ctx,
		Config:   cfg,
		State:    NewState(),
		Infra:    infra,
		Talos:    talos,
		Observer: observer,
		Timeouts: config.LoadTimeouts(),
	}
}
