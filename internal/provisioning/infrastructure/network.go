package infrastructure

import (
	"fmt"

	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/labels"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// ProvisionNetwork provisions the private network and subnets.
func (p *Provisioner) ProvisionNetwork(ctx *provisioning.Context) error {
	provisioning.LogResourceCreating(ctx.Observer, "infrastructure", "network", ctx.Config.ClusterName)

	// Subnets are calculated during validation phase

	networkLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
		WithTestIDIfSet(ctx.Config.TestID).
		Build()

	network, err := ctx.Infra.EnsureNetwork(ctx, ctx.Config.ClusterName, ctx.Config.Network.IPv4CIDR, ctx.Config.Network.Zone, networkLabels)
	if err != nil {
		return fmt.Errorf("failed to ensure network: %w", err)
	}
	provisioning.LogResourceCreated(ctx.Observer, "infrastructure", "network", ctx.Config.ClusterName, fmt.Sprintf("%d", network.ID))
	ctx.State.Network = network

	// Detect Public IP for Firewall (if needed)
	if ctx.Config.Firewall.UseCurrentIPv4 != nil && *ctx.Config.Firewall.UseCurrentIPv4 && ctx.State.PublicIP == "" {
		ip, err := ctx.Infra.GetPublicIP(ctx)
		if err != nil {
			ctx.Observer.Printf("[Infra:Network] Warning: failed to detect public IP: %v", err)
		} else {
			ctx.State.PublicIP = ip
			ctx.Observer.Printf("[Infra:Network] Detected public IP: %s", ip)
		}
	}

	// Subnets
	// Note: We do NOT create the parent NodeIPv4CIDR as a subnet, only the leaf subnets (CP, LB, Worker)
	// creating the parent would cause "overlaps" error in HCloud.

	// Control Plane Subnet
	cpSubnet, err := ctx.Config.GetSubnetForRole("control-plane", 0)
	if err != nil {
		return fmt.Errorf("failed to calculate control-plane subnet: %w", err)
	}
	err = ctx.Infra.EnsureSubnet(ctx, network, cpSubnet, ctx.Config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
	if err != nil {
		return fmt.Errorf("failed to ensure control-plane subnet: %w", err)
	}

	// LB Subnet
	lbSubnet, err := ctx.Config.GetSubnetForRole("load-balancer", 0)
	if err != nil {
		return fmt.Errorf("failed to calculate load-balancer subnet: %w", err)
	}
	err = ctx.Infra.EnsureSubnet(ctx, network, lbSubnet, ctx.Config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
	if err != nil {
		return fmt.Errorf("failed to ensure load-balancer subnet: %w", err)
	}

	// Worker Subnets
	for i := range ctx.Config.Workers {
		wSubnet, err := ctx.Config.GetSubnetForRole("worker", i)
		if err != nil {
			return fmt.Errorf("failed to calculate worker subnet for pool %d: %w", i, err)
		}

		err = ctx.Infra.EnsureSubnet(ctx, network, wSubnet, ctx.Config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
		if err != nil {
			return fmt.Errorf("failed to ensure worker subnet for pool %d: %w", i, err)
		}
	}

	// Autoscaler Subnet (for dynamically created nodes)
	if ctx.Config.Autoscaler.Enabled && len(ctx.Config.Autoscaler.NodePools) > 0 {
		autoscalerSubnet, err := ctx.Config.GetSubnetForRole("autoscaler", 0)
		if err != nil {
			return fmt.Errorf("failed to calculate autoscaler subnet: %w", err)
		}

		err = ctx.Infra.EnsureSubnet(ctx, network, autoscalerSubnet, ctx.Config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
		if err != nil {
			return fmt.Errorf("failed to ensure autoscaler subnet: %w", err)
		}
	}

	return nil
}
