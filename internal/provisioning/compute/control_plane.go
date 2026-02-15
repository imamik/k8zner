// Package compute provides compute resource provisioning functionality including
// servers, control plane, workers, and node pool management.
package compute

import (
	"fmt"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/labels"
	"github.com/imamik/k8zner/internal/util/naming"
)

const phase = "compute"

// ProvisionControlPlane provisions control plane servers.
// Note: When called from Provision(), the endpoint is already set up via prepareControlPlaneEndpoint().
// This method is kept for backward compatibility and direct invocation in tests.
func (p *Provisioner) ProvisionControlPlane(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[%s] Reconciling control plane...", phase)

	// Setup endpoint if not already done (for backward compatibility when called directly)
	if len(ctx.State.SANs) == 0 {
		var sans []string

		lb, err := ctx.Infra.GetLoadBalancer(ctx, naming.KubeAPILoadBalancer(ctx.Config.ClusterName))
		if err != nil {
			return fmt.Errorf("failed to get load balancer: %w", err)
		}
		if lb != nil {
			if lbIP := hcloud.LoadBalancerIPv4(lb); lbIP != "" {
				sans = append(sans, lbIP)
				endpoint := fmt.Sprintf("https://%s:%d", lbIP, config.KubeAPIPort)
				ctx.Observer.Printf("[%s] Setting Talos endpoint to: %s", phase, endpoint)
				ctx.Talos.SetEndpoint(endpoint)
			}

			for _, net := range lb.PrivateNet {
				sans = append(sans, net.IP.String())
			}
		}
		ctx.State.SANs = sans
	}

	// Provision Servers (configs will be generated per-node in reconciler)
	for i, pool := range ctx.Config.ControlPlane.NodePools {
		// Placement Group for Control Plane
		pgLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
			WithPool(pool.Name).
			WithTestIDIfSet(ctx.Config.TestID).
			Build()

		pg, err := ctx.Infra.EnsurePlacementGroup(ctx, naming.PlacementGroup(ctx.Config.ClusterName, pool.Name), "spread", pgLabels)
		if err != nil {
			return fmt.Errorf("failed to ensure placement group for pool %s: %w", pool.Name, err)
		}

		poolResult, err := p.reconcileNodePool(ctx, NodePoolSpec{
			Name:             pool.Name,
			Count:            pool.Count,
			ServerType:       pool.ServerType,
			Location:         pool.Location,
			Image:            pool.Image,
			Role:             "control-plane",
			ExtraLabels:      pool.Labels,
			PlacementGroupID: &pg.ID,
			PoolIndex:        i,
			EnablePublicIPv4: ctx.Config.ShouldEnablePublicIPv4(),
			EnablePublicIPv6: ctx.Config.ShouldEnablePublicIPv6(),
		})
		if err != nil {
			return fmt.Errorf("failed to reconcile node pool %s: %w", pool.Name, err)
		}
		for k, v := range poolResult.IPs {
			ctx.State.ControlPlaneIPs[k] = v
		}
		for k, v := range poolResult.ServerIDs {
			ctx.State.ControlPlaneServerIDs[k] = v
		}
	}

	return nil
}
