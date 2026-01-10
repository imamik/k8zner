package cluster

import (
	"hcloud-k8s/internal/provisioning"
)

// Provisioner handles cluster lifecycle operations (bootstrap, upgrade).
type Provisioner struct{}

// NewProvisioner creates a new cluster provisioner.
func NewProvisioner() *Provisioner {
	return &Provisioner{}
}

// Name implements the provisioning.Phase interface.
func (p *Provisioner) Name() string {
	return "cluster"
}

// Provision implements the provisioning.Phase interface.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	return p.BootstrapCluster(ctx)
}
