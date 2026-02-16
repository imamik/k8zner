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

// Provision ensures all required Talos images exist.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	return p.EnsureAllImages(ctx)
}
