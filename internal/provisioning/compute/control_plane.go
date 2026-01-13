// Package compute provides compute resource provisioning functionality including
// servers, control plane, workers, and node pool management.
package compute

import (
	"fmt"

	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/labels"
	"hcloud-k8s/internal/util/naming"
)

const phase = "compute"

// ProvisionControlPlane provisions control plane servers.
func (p *Provisioner) ProvisionControlPlane(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Reconciling control plane...", phase)

	// Collect all SANs
	var sans []string

	// LB IP (Public) - if Ingress enabled or API LB?
	// The API LB is "kube-api".
	lb, err := ctx.Infra.GetLoadBalancer(ctx, naming.KubeAPILoadBalancer(ctx.Config.ClusterName))
	if err != nil {
		return fmt.Errorf("failed to get load balancer: %w", err)
	}
	if lb != nil {
		// Use LB Public IP as endpoint
		if lb.PublicNet.IPv4.IP != nil {
			lbIP := lb.PublicNet.IPv4.IP.String()
			sans = append(sans, lbIP)

			// UPDATE TALOS ENDPOINT
			// We use the LB IP as the control plane endpoint.
			endpoint := fmt.Sprintf("https://%s:6443", lbIP)
			ctx.Logger.Printf("[%s] Setting Talos endpoint to: %s", phase, endpoint)
			ctx.Talos.SetEndpoint(endpoint)
		}

		// Also add private IP of LB
		for _, net := range lb.PrivateNet {
			sans = append(sans, net.IP.String())
		}
	}

	// Provision Servers (configs will be generated per-node in reconciler)
	for i, pool := range ctx.Config.ControlPlane.NodePools {
		// Placement Group for Control Plane
		lb := labels.NewLabelBuilder(ctx.Config.ClusterName).
			WithPool(pool.Name)
		if ctx.Config.TestID != "" {
			lb = lb.WithTestID(ctx.Config.TestID)
		}
		pgLabels := lb.Build()

		pg, err := ctx.Infra.EnsurePlacementGroup(ctx, naming.PlacementGroup(ctx.Config.ClusterName, pool.Name), "spread", pgLabels)
		if err != nil {
			return fmt.Errorf("failed to ensure placement group for pool %s: %w", pool.Name, err)
		}

		poolIPs, err := p.reconcileNodePool(ctx, NodePoolSpec{
			Name:             pool.Name,
			Count:            pool.Count,
			ServerType:       pool.ServerType,
			Location:         pool.Location,
			Image:            pool.Image,
			Role:             "control-plane",
			ExtraLabels:      pool.Labels,
			UserData:         "",
			PlacementGroupID: &pg.ID,
			PoolIndex:        i,
		})
		if err != nil {
			return fmt.Errorf("failed to reconcile node pool %s: %w", pool.Name, err)
		}
		for k, v := range poolIPs {
			ctx.State.ControlPlaneIPs[k] = v
		}
	}

	ctx.State.SANs = sans
	return nil
}
