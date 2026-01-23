package provisioning

import (
	"context"
	"log"

	"k8zner/internal/config"
	hcloud_internal "k8zner/internal/platform/hcloud"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// Logger defines a simple logging interface for provisioning phases.
type Logger interface {
	Printf(format string, v ...interface{})
}

// DefaultLogger is a logger that uses the standard log package.
type DefaultLogger struct{}

// Printf implements Logger interface for DefaultLogger.
func (l *DefaultLogger) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}

// State holds the shared results of provisioning phases.
// It is progressively populated as each phase completes and is passed
// to subsequent phases that need earlier results.
type State struct {
	// Infrastructure results (populated by infrastructure provisioner)
	Network  *hcloud.Network
	Firewall *hcloud.Firewall
	PublicIP string // Current execution environment's public IPv4
	SSHKeyID int64  // SSH key ID (for autoscaler addon)

	// Compute results (populated by compute provisioner)
	ControlPlaneIPs map[string]string // nodeName -> publicIP
	WorkerIPs       map[string]string // nodeName -> publicIP
	SANs            []string          // Subject Alternative Names for certs

	// Cluster results (populated by cluster bootstrapper)
	Kubeconfig  []byte
	TalosConfig []byte
}

// NewState creates an empty provisioning state.
func NewState() *State {
	return &State{
		ControlPlaneIPs: make(map[string]string),
		WorkerIPs:       make(map[string]string),
	}
}

// Context wraps all dependencies and state needed for a provisioning phase.
type Context struct {
	context.Context
	Config   *config.Config
	State    *State
	Infra    hcloud_internal.InfrastructureManager
	Talos    TalosConfigProducer
	Observer Observer // Replaced Logger with Observer for structured logging
	Logger   Logger   // Keep for backward compatibility (points to same Observer)
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
		Logger:   observer, // Observer implements Logger interface
		Timeouts: config.LoadTimeouts(),
	}
}
