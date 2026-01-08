package provisioning

import (
	"context"
	"log"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func (r *Reconciler) reconcileNetwork(ctx context.Context) error {
	log.Printf("Reconciling Network %s...", r.config.ClusterName)
	labels := map[string]string{
		"cluster": r.config.ClusterName,
	}

	network, err := r.networkManager.EnsureNetwork(ctx, r.config.ClusterName, r.config.Network.IPv4CIDR, r.config.Network.Zone, labels)
	if err != nil {
		return err
	}
	r.network = network

	// Subnets
	// Note: We do NOT create the parent NodeIPv4CIDR as a subnet, only the leaf subnets (CP, LB, Worker)
	// creating the parent would cause "overlaps" error in HCloud.

	// Control Plane Subnet
	cpSubnet, err := r.config.GetSubnetForRole("control-plane", 0)
	if err != nil {
		return err
	}
	err = r.networkManager.EnsureSubnet(ctx, network, cpSubnet, r.config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
	if err != nil {
		return err
	}

	// LB Subnet
	lbSubnet, err := r.config.GetSubnetForRole("load-balancer", 0)
	if err != nil {
		return err
	}
	err = r.networkManager.EnsureSubnet(ctx, network, lbSubnet, r.config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
	if err != nil {
		return err
	}

	// Worker Subnets
	for i := range r.config.Workers {
		wSubnet, err := r.config.GetSubnetForRole("worker", i)
		if err != nil {
			return err
		}

		err = r.networkManager.EnsureSubnet(ctx, network, wSubnet, r.config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
		if err != nil {
			return err
		}
	}

	return nil
}
