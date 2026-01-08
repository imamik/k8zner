package infrastructure

import (
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
)

// Provisioner handles infrastructure provisioning (network, firewall, load balancers).
type Provisioner struct {
	networkManager  hcloud_internal.NetworkManager
	firewallManager hcloud_internal.FirewallManager
	lbManager       hcloud_internal.LoadBalancerManager
	fipManager      hcloud_internal.FloatingIPManager
	config          *config.Config

	// State
	network  *hcloud.Network
	firewall *hcloud.Firewall
}

// NewProvisioner creates a new infrastructure provisioner.
func NewProvisioner(
	infra hcloud_internal.InfrastructureManager,
	cfg *config.Config,
) *Provisioner {
	return &Provisioner{
		networkManager:  infra,
		firewallManager: infra,
		lbManager:       infra,
		fipManager:      infra,
		config:          cfg,
	}
}

// GetNetwork returns the provisioned network (to be passed to compute provisioner).
func (p *Provisioner) GetNetwork() *hcloud.Network {
	return p.network
}
