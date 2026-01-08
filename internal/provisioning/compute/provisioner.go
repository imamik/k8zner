package compute

import (
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
)

// TalosConfigProducer defines the interface for generating Talos configurations.
type TalosConfigProducer interface {
	GenerateControlPlaneConfig(san []string, hostname string) ([]byte, error)
	GenerateWorkerConfig(hostname string) ([]byte, error)
	SetEndpoint(endpoint string)
}

// Provisioner handles compute resource provisioning (servers, node pools).
type Provisioner struct {
	serverProvisioner hcloud_internal.ServerProvisioner
	lbManager         hcloud_internal.LoadBalancerManager
	pgManager         hcloud_internal.PlacementGroupManager
	snapshotManager   hcloud_internal.SnapshotManager
	infra             hcloud_internal.InfrastructureManager
	talosGenerator    TalosConfigProducer
	config            *config.Config
	timeouts          *config.Timeouts
	network           *hcloud.Network
}

// NewProvisioner creates a new compute provisioner.
func NewProvisioner(
	infra hcloud_internal.InfrastructureManager,
	talosGenerator TalosConfigProducer,
	cfg *config.Config,
	network *hcloud.Network,
) *Provisioner {
	return &Provisioner{
		serverProvisioner: infra,
		lbManager:         infra,
		pgManager:         infra,
		snapshotManager:   infra,
		infra:             infra,
		talosGenerator:    talosGenerator,
		config:            cfg,
		timeouts:          config.LoadTimeouts(),
		network:           network,
	}
}
