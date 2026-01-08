package compute

import (
	"context"
	"fmt"
	"log"

	"hcloud-k8s/internal/util/labels"
	"hcloud-k8s/internal/util/naming"
)

// reconcileControlPlane provisions control plane servers and returns a map of ServerName -> PublicIP and the SANs to use.
func (p *Provisioner) ProvisionControlPlane(ctx context.Context) (map[string]string, []string, error) {
	log.Printf("Reconciling Control Plane...")

	// Collect all SANs
	var sans []string

	// LB IP (Public) - if Ingress enabled or API LB?
	// The API LB is "kube-api".
	lb, err := p.lbManager.GetLoadBalancer(ctx, naming.KubeAPILoadBalancer(p.config.ClusterName))
	if err != nil {
		return nil, nil, err
	}
	if lb != nil {
		// Use LB Public IP as endpoint
		if lb.PublicNet.IPv4.IP != nil {
			lbIP := lb.PublicNet.IPv4.IP.String()
			sans = append(sans, lbIP)

			// UPDATE TALOS ENDPOINT
			// We use the LB IP as the control plane endpoint.
			endpoint := fmt.Sprintf("https://%s:6443", lbIP)
			log.Printf("Setting Talos Endpoint to: %s", endpoint)
			p.talosGenerator.SetEndpoint(endpoint)
		}

		// Also add private IP of LB
		for _, net := range lb.PrivateNet {
			sans = append(sans, net.IP.String())
		}
	}

	// Add Floating IPs if any (Control Plane VIP)
	// if p.config.ControlPlane.PublicVIPIPv4Enabled {
	// 	// TODO: Implement VIP lookup if ID not provided
	// 	// For now assume standard pattern
	// }

	// Provision Servers (configs will be generated per-node in reconciler)
	ips := make(map[string]string)
	for i, pool := range p.config.ControlPlane.NodePools {
		// Placement Group for Control Plane
		pgLabels := labels.NewLabelBuilder(p.config.ClusterName).
			WithPool(pool.Name).
			Build()

		pg, err := p.pgManager.EnsurePlacementGroup(ctx, naming.PlacementGroup(p.config.ClusterName, pool.Name), "spread", pgLabels)
		if err != nil {
			return nil, nil, err
		}

		poolIPs, err := p.reconcileNodePool(ctx, pool.Name, pool.Count, pool.ServerType, pool.Location, pool.Image, "control-plane", pool.Labels, "", &pg.ID, i)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range poolIPs {
			ips[k] = v
		}
	}

	return ips, sans, nil
}
