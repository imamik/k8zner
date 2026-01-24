package image

import (
	"github.com/imamik/k8zner/internal/provisioning"
)

// Provisioner handles image provisioning (building and managing Talos images).
type Provisioner struct{}

// NewProvisioner creates a new image provisioner.
func NewProvisioner() *Provisioner {
	return &Provisioner{}
}

// Name implements the provisioning.Phase interface.
func (p *Provisioner) Name() string {
	return "image"
}

// Provision implements the provisioning.Phase interface.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	return p.EnsureAllImages(ctx)
}
