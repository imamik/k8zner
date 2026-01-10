package image

import (
	"hcloud-k8s/internal/provisioning"
)

// Provisioner handles image provisioning (building and managing Talos images).
type Provisioner struct{}

// NewProvisioner creates a new image provisioner.
func NewProvisioner() *Provisioner {
	return &Provisioner{}
}

// Provision implements the provisioning.Phase interface.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	return p.EnsureAllImages(ctx)
}
