package hcloud

import (
	"context"
	"fmt"
	"log"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// CleanupByLabel deletes all Hetzner Cloud resources matching the given label selector.
// This is useful for cleaning up E2E test resources or orphaned resources.
func (c *RealClient) CleanupByLabel(ctx context.Context, labelSelector map[string]string) error {
	log.Printf("[Cleanup] Starting cleanup for resources with labels: %v", labelSelector)

	// Build label selector string
	labelString := buildLabelSelector(labelSelector)

	// Delete in order to respect dependencies:
	// 1. Servers (must be deleted before networks, load balancers)
	// 2. Load Balancers
	// 3. Floating IPs
	// 4. Firewalls
	// 5. Networks
	// 6. Placement Groups
	// 7. SSH Keys
	// 8. Certificates (Talos state)
	// 9. Snapshots (if specified)

	if err := c.deleteServersByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete servers: %v", err)
	}

	if err := c.deleteLoadBalancersByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete load balancers: %v", err)
	}

	if err := c.deleteFloatingIPsByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete floating IPs: %v", err)
	}

	if err := c.deleteFirewallsByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete firewalls: %v", err)
	}

	if err := c.deleteNetworksByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete networks: %v", err)
	}

	if err := c.deletePlacementGroupsByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete placement groups: %v", err)
	}

	if err := c.deleteSSHKeysByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete SSH keys: %v", err)
	}

	if err := c.deleteCertificatesByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete certificates: %v", err)
	}

	log.Printf("[Cleanup] Cleanup complete")
	return nil
}

// buildLabelSelector converts a map of labels to a Hetzner Cloud label selector string.
func buildLabelSelector(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	selector := ""
	for k, v := range labels {
		if selector != "" {
			selector += ","
		}
		selector += fmt.Sprintf("%s=%s", k, v)
	}
	return selector
}

// deleteServersByLabel deletes all servers matching the label selector.
func (c *RealClient) deleteServersByLabel(ctx context.Context, labelSelector string) error {
	servers, err := c.client.Server.AllWithOpts(ctx, hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
	})
	if err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	for _, server := range servers {
		log.Printf("[Cleanup] Deleting server: %s (ID: %d)", server.Name, server.ID)
		if _, _, err := c.client.Server.DeleteWithResult(ctx, server); err != nil {
			log.Printf("[Cleanup] Warning: Failed to delete server %s: %v", server.Name, err)
		}
	}

	return nil
}

// deleteLoadBalancersByLabel deletes all load balancers matching the label selector.
func (c *RealClient) deleteLoadBalancersByLabel(ctx context.Context, labelSelector string) error {
	lbs, err := c.client.LoadBalancer.AllWithOpts(ctx, hcloud.LoadBalancerListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
	})
	if err != nil {
		return fmt.Errorf("failed to list load balancers: %w", err)
	}

	for _, lb := range lbs {
		log.Printf("[Cleanup] Deleting load balancer: %s (ID: %d)", lb.Name, lb.ID)
		if _, err := c.client.LoadBalancer.Delete(ctx, lb); err != nil {
			log.Printf("[Cleanup] Warning: Failed to delete load balancer %s: %v", lb.Name, err)
		}
	}

	return nil
}

// deleteFloatingIPsByLabel deletes all floating IPs matching the label selector.
func (c *RealClient) deleteFloatingIPsByLabel(ctx context.Context, labelSelector string) error {
	fips, err := c.client.FloatingIP.AllWithOpts(ctx, hcloud.FloatingIPListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
	})
	if err != nil {
		return fmt.Errorf("failed to list floating IPs: %w", err)
	}

	for _, fip := range fips {
		log.Printf("[Cleanup] Deleting floating IP: %s (ID: %d)", fip.Name, fip.ID)
		if _, err := c.client.FloatingIP.Delete(ctx, fip); err != nil {
			log.Printf("[Cleanup] Warning: Failed to delete floating IP %s: %v", fip.Name, err)
		}
	}

	return nil
}

// deleteFirewallsByLabel deletes all firewalls matching the label selector.
func (c *RealClient) deleteFirewallsByLabel(ctx context.Context, labelSelector string) error {
	firewalls, err := c.client.Firewall.AllWithOpts(ctx, hcloud.FirewallListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
	})
	if err != nil {
		return fmt.Errorf("failed to list firewalls: %w", err)
	}

	for _, fw := range firewalls {
		log.Printf("[Cleanup] Deleting firewall: %s (ID: %d)", fw.Name, fw.ID)
		if _, err := c.client.Firewall.Delete(ctx, fw); err != nil {
			log.Printf("[Cleanup] Warning: Failed to delete firewall %s: %v", fw.Name, err)
		}
	}

	return nil
}

// deleteNetworksByLabel deletes all networks matching the label selector.
func (c *RealClient) deleteNetworksByLabel(ctx context.Context, labelSelector string) error {
	networks, err := c.client.Network.AllWithOpts(ctx, hcloud.NetworkListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
	})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	for _, network := range networks {
		log.Printf("[Cleanup] Deleting network: %s (ID: %d)", network.Name, network.ID)
		if _, err := c.client.Network.Delete(ctx, network); err != nil {
			log.Printf("[Cleanup] Warning: Failed to delete network %s: %v", network.Name, err)
		}
	}

	return nil
}

// deletePlacementGroupsByLabel deletes all placement groups matching the label selector.
func (c *RealClient) deletePlacementGroupsByLabel(ctx context.Context, labelSelector string) error {
	pgs, err := c.client.PlacementGroup.AllWithOpts(ctx, hcloud.PlacementGroupListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
	})
	if err != nil {
		return fmt.Errorf("failed to list placement groups: %w", err)
	}

	for _, pg := range pgs {
		log.Printf("[Cleanup] Deleting placement group: %s (ID: %d)", pg.Name, pg.ID)
		if _, err := c.client.PlacementGroup.Delete(ctx, pg); err != nil {
			log.Printf("[Cleanup] Warning: Failed to delete placement group %s: %v", pg.Name, err)
		}
	}

	return nil
}

// deleteSSHKeysByLabel deletes all SSH keys matching the label selector.
func (c *RealClient) deleteSSHKeysByLabel(ctx context.Context, labelSelector string) error {
	keys, err := c.client.SSHKey.AllWithOpts(ctx, hcloud.SSHKeyListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
	})
	if err != nil {
		return fmt.Errorf("failed to list SSH keys: %w", err)
	}

	for _, key := range keys {
		log.Printf("[Cleanup] Deleting SSH key: %s (ID: %d)", key.Name, key.ID)
		if _, err := c.client.SSHKey.Delete(ctx, key); err != nil {
			log.Printf("[Cleanup] Warning: Failed to delete SSH key %s: %v", key.Name, err)
		}
	}

	return nil
}

// deleteCertificatesByLabel deletes all certificates matching the label selector.
func (c *RealClient) deleteCertificatesByLabel(ctx context.Context, labelSelector string) error {
	certs, err := c.client.Certificate.AllWithOpts(ctx, hcloud.CertificateListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
	})
	if err != nil {
		return fmt.Errorf("failed to list certificates: %w", err)
	}

	for _, cert := range certs {
		log.Printf("[Cleanup] Deleting certificate: %s (ID: %d)", cert.Name, cert.ID)
		if _, err := c.client.Certificate.Delete(ctx, cert); err != nil {
			log.Printf("[Cleanup] Warning: Failed to delete certificate %s: %v", cert.Name, err)
		}
	}

	return nil
}
