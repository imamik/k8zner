package infrastructure

import (
	"context"
	"log"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// ProvisionNetwork provisions the private network and subnets.
func (p *Provisioner) ProvisionNetwork(ctx context.Context) error {
	log.Printf("Reconciling Network %s...", p.config.ClusterName)
	labels := map[string]string{
		"cluster": p.config.ClusterName,
	}

	network, err := p.networkManager.EnsureNetwork(ctx, p.config.ClusterName, p.config.Network.IPv4CIDR, p.config.Network.Zone, labels)
	if err != nil {
		return err
	}
	p.network = network

	// Subnets
	// Note: We do NOT create the parent NodeIPv4CIDR as a subnet, only the leaf subnets (CP, LB, Worker)
	// creating the parent would cause "overlaps" error in HCloud.

	// Control Plane Subnet
	cpSubnet, err := p.config.GetSubnetForRole("control-plane", 0)
	if err != nil {
		return err
	}
	err = p.networkManager.EnsureSubnet(ctx, network, cpSubnet, p.config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
	if err != nil {
		return err
	}

	// LB Subnet
	lbSubnet, err := p.config.GetSubnetForRole("load-balancer", 0)
	if err != nil {
		return err
	}
	err = p.networkManager.EnsureSubnet(ctx, network, lbSubnet, p.config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
	if err != nil {
		return err
	}

	// Worker Subnets
	for i := range p.config.Workers {
		wSubnet, err := p.config.GetSubnetForRole("worker", i)
		if err != nil {
			return err
		}

		err = p.networkManager.EnsureSubnet(ctx, network, wSubnet, p.config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
		if err != nil {
			return err
		}
	}

	return nil
}
