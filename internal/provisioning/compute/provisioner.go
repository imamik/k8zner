package compute

import (
	"github.com/imamik/k8zner/internal/provisioning"
)

// Provisioner handles compute resource provisioning (servers, node pools).
type Provisioner struct{}

// NewProvisioner creates a new compute provisioner.
func NewProvisioner() *Provisioner {
	return &Provisioner{}
}

// Name implements the provisioning.Phase interface.
func (p *Provisioner) Name() string {
	return "compute"
}

// Provision implements the provisioning.Phase interface.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	// 1. Control plane nodes
	if err := p.ProvisionControlPlane(ctx); err != nil {
		return err
	}

	// 2. Worker nodes
	if err := p.ProvisionWorkers(ctx); err != nil {
		return err
	}

	return nil
}
