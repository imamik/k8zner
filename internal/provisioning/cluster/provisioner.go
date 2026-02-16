package cluster

import (
	"github.com/imamik/k8zner/internal/provisioning"
)

// Provisioner handles cluster lifecycle operations (bootstrap, upgrade).
type Provisioner struct{}

// NewProvisioner creates a new cluster provisioner.
func NewProvisioner() *Provisioner {
	return &Provisioner{}
}

// Provision bootstraps the cluster.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	return p.BootstrapCluster(ctx)
}
