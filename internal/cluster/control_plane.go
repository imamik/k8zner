package cluster

import (
	"context"
	"fmt"
	"log"
)

// reconcileControlPlane provisions control plane servers and returns a map of ServerName -> PublicIP.
func (r *Reconciler) reconcileControlPlane(ctx context.Context) (map[string]string, error) {
	log.Printf("Reconciling Control Plane...")

	names := NewNames(r.config.ClusterName)

	// Collect all SANs
	var sans []string

	// LB IP (Public) - if Ingress enabled or API LB?
	// The API LB is "kube-api".
	lb, err := r.lbManager.GetLoadBalancer(ctx, names.KubeAPILoadBalancer())
	if err != nil {
		return nil, err
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
			r.talosGenerator.SetEndpoint(endpoint)
		}

		// Also add private IP of LB
		for _, net := range lb.PrivateNet {
			sans = append(sans, net.IP.String())
		}
	} else if r.config.ControlPlane.DisableKubeAPILoadBalancer && r.config.ControlPlane.PublicVIPIPv4Enabled {
		// If LB is disabled and FIP is enabled, use FIP as endpoint
		fipName := names.ControlPlaneFloatingIP()
		fip, err := r.fipManager.GetFloatingIP(ctx, fipName)
		if err != nil {
			return nil, fmt.Errorf("failed to get control plane floating IP: %w", err)
		}
		if fip != nil {
			fipIP := fip.IP.String()
			sans = append(sans, fipIP)

			endpoint := fmt.Sprintf("https://%s:6443", fipIP)
			log.Printf("Setting Talos Endpoint to Floating IP: %s", endpoint)
			r.talosGenerator.SetEndpoint(endpoint)
		}
	}

	// Generate Talos Config for CP
	cpConfig, err := r.talosGenerator.GenerateControlPlaneConfig(sans)
	if err != nil {
		return nil, err
	}

	// Provision Servers
	ips := make(map[string]string)
	for i, pool := range r.config.ControlPlane.NodePools {
		// Placement Group for Control Plane
		pgLabels := NewLabelBuilder(r.config.ClusterName).
			WithPool(pool.Name).
			Build()

		pg, err := r.pgManager.EnsurePlacementGroup(ctx, names.PlacementGroup(pool.Name), "spread", pgLabels)
		if err != nil {
			return nil, err
		}

		poolIPs, err := r.reconcileNodePool(ctx, pool.Name, pool.Count, pool.ServerType, pool.Location, pool.Image, "control-plane", pool.Labels, string(cpConfig), &pg.ID, i)
		if err != nil {
			return nil, err
		}
		for k, v := range poolIPs {
			ips[k] = v
		}

		// If single control plane node and LB is disabled and FIP is enabled, assign FIP to the server
		if pool.Count == 1 && r.config.ControlPlane.DisableKubeAPILoadBalancer && r.config.ControlPlane.PublicVIPIPv4Enabled {
			// Find the server name (key of poolIPs)
			var serverName string
			for k := range poolIPs {
				serverName = k
				break
			}

			if serverName != "" {
				log.Printf("Assigning Floating IP to single control plane node: %s", serverName)
				serverID, err := r.serverProvisioner.GetServerID(ctx, serverName)
				if err != nil {
					return nil, fmt.Errorf("failed to get server ID for FIP assignment: %w", err)
				}

				// Parse server ID to int64
				var sID int64
				_, err = fmt.Sscanf(serverID, "%d", &sID)
				if err != nil {
					return nil, fmt.Errorf("failed to parse server ID: %w", err)
				}

				fipName := names.ControlPlaneFloatingIP()
				fip, err := r.fipManager.GetFloatingIP(ctx, fipName)
				if err != nil {
					return nil, fmt.Errorf("failed to get floating IP: %w", err)
				}
				if fip != nil {
					if err := r.fipManager.AssignFloatingIP(ctx, fip, sID); err != nil {
						return nil, fmt.Errorf("failed to assign floating IP to server: %w", err)
					}
				}
			}
		}
	}

	return ips, nil
}
