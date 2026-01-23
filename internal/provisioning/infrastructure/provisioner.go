package infrastructure

import (
	"k8zner/internal/provisioning"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// Provisioner handles infrastructure provisioning (network, firewall, load balancers).
type Provisioner struct{}

// NewProvisioner creates a new infrastructure provisioner.
func NewProvisioner() *Provisioner {
	return &Provisioner{}
}

// Name implements the provisioning.Phase interface.
func (p *Provisioner) Name() string {
	return "infrastructure"
}

// Provision implements the provisioning.Phase interface.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	// 1. Network
	if err := p.ProvisionNetwork(ctx); err != nil {
		return err
	}

	// 2. Firewall
	if err := p.ProvisionFirewall(ctx); err != nil {
		return err
	}

	// 3. Load Balancers
	if err := p.ProvisionLoadBalancers(ctx); err != nil {
		return err
	}

	// 4. Floating IPs
	if err := p.ProvisionFloatingIPs(ctx); err != nil {
		return err
	}

	return nil
}

// GetNetwork returns the provisioned network from state.
func (p *Provisioner) GetNetwork(ctx *provisioning.Context) *hcloud.Network {
	return ctx.State.Network
}
