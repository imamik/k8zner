package image

import (
	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
)

// Provisioner handles image provisioning (building and managing Talos images).
type Provisioner struct {
	snapshotManager hcloud_internal.SnapshotManager
	infra           hcloud_internal.InfrastructureManager
	config          *config.Config
	timeouts        *config.Timeouts
}

// NewProvisioner creates a new image provisioner.
func NewProvisioner(
	infra hcloud_internal.InfrastructureManager,
	cfg *config.Config,
) *Provisioner {
	return &Provisioner{
		snapshotManager: infra,
		infra:           infra,
		config:          cfg,
		timeouts:        config.LoadTimeouts(),
	}
}
