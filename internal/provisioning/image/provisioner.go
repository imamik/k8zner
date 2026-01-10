package image

import (
	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/provisioning"
)

// Provisioner handles image provisioning (building and managing Talos images).
type Provisioner struct {
	timeouts *config.Timeouts
}

// NewProvisioner creates a new image provisioner.
func NewProvisioner() *Provisioner {
	return &Provisioner{
		timeouts: config.LoadTimeouts(),
	}
}

// Provision implements the provisioning.Phase interface.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	return p.EnsureAllImages(ctx)
}
