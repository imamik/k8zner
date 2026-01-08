package cluster

import (
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
)

// Provisioner handles cluster lifecycle operations (bootstrap, upgrade).
type Provisioner struct {
	hClient hcloud_internal.InfrastructureManager
}

// NewProvisioner creates a new cluster provisioner.
func NewProvisioner(infra hcloud_internal.InfrastructureManager) *Provisioner {
	return &Provisioner{
		hClient: infra,
	}
}
