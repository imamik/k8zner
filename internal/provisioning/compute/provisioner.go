package compute

import (
	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning"
)

// Provisioner handles compute resource provisioning (servers, node pools).
type Provisioner struct {
	serverProvisioner hcloud_internal.ServerProvisioner
	lbManager         hcloud_internal.LoadBalancerManager
	pgManager         hcloud_internal.PlacementGroupManager
	snapshotManager   hcloud_internal.SnapshotManager
	infra             hcloud_internal.InfrastructureManager
	talosGenerator    provisioning.TalosConfigProducer
	config            *config.Config
	timeouts          *config.Timeouts
	state             *provisioning.State
}

// NewProvisioner creates a new compute provisioner.
func NewProvisioner(
	infra hcloud_internal.InfrastructureManager,
	talosGenerator provisioning.TalosConfigProducer,
	cfg *config.Config,
	state *provisioning.State,
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
		state:             state,
	}
}
