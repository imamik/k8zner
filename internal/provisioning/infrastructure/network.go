package infrastructure

import (
	"log"

	"hcloud-k8s/internal/provisioning"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// ProvisionNetwork provisions the private network and subnets.
func (p *Provisioner) ProvisionNetwork(ctx *provisioning.Context) error {
	log.Printf("Reconciling Network %s...", ctx.Config.ClusterName)

	// 0. Calculate subnets if not already set (replaces Terraform's CIDR calculations)
	if err := ctx.Config.CalculateSubnets(); err != nil {
		return err
	}

	labels := map[string]string{
		"cluster": ctx.Config.ClusterName,
	}

	network, err := ctx.Infra.EnsureNetwork(ctx, ctx.Config.ClusterName, ctx.Config.Network.IPv4CIDR, ctx.Config.Network.Zone, labels)
	if err != nil {
		return err
	}
	ctx.State.Network = network

	// Subnets
	// Note: We do NOT create the parent NodeIPv4CIDR as a subnet, only the leaf subnets (CP, LB, Worker)
	// creating the parent would cause "overlaps" error in HCloud.

	// Control Plane Subnet
	cpSubnet, err := ctx.Config.GetSubnetForRole("control-plane", 0)
	if err != nil {
		return err
	}
	err = ctx.Infra.EnsureSubnet(ctx, network, cpSubnet, ctx.Config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
	if err != nil {
		return err
	}

	// LB Subnet
	lbSubnet, err := ctx.Config.GetSubnetForRole("load-balancer", 0)
	if err != nil {
		return err
	}
	err = ctx.Infra.EnsureSubnet(ctx, network, lbSubnet, ctx.Config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
	if err != nil {
		return err
	}

	// Worker Subnets
	for i := range ctx.Config.Workers {
		wSubnet, err := ctx.Config.GetSubnetForRole("worker", i)
		if err != nil {
			return err
		}

		err = ctx.Infra.EnsureSubnet(ctx, network, wSubnet, ctx.Config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
		if err != nil {
			return err
		}
	}

	return nil
}
