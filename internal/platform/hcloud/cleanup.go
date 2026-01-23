package hcloud

import (
	"context"
	"fmt"
	"log"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// resource is a constraint for Hetzner Cloud resources that have Name and ID fields.
type resource interface {
	*hcloud.Server | *hcloud.LoadBalancer | *hcloud.FloatingIP | *hcloud.Firewall |
		*hcloud.Network | *hcloud.PlacementGroup | *hcloud.SSHKey | *hcloud.Certificate
}

// resourceInfo extracts common fields from a resource for logging.
type resourceInfo struct {
	Name string
	ID   int64
}

// getResourceInfo extracts name and ID from various resource types.
func getResourceInfo[T resource](r T) resourceInfo {
	switch v := any(r).(type) {
	case *hcloud.Server:
		return resourceInfo{Name: v.Name, ID: v.ID}
	case *hcloud.LoadBalancer:
		return resourceInfo{Name: v.Name, ID: v.ID}
	case *hcloud.FloatingIP:
		return resourceInfo{Name: v.Name, ID: v.ID}
	case *hcloud.Firewall:
		return resourceInfo{Name: v.Name, ID: v.ID}
	case *hcloud.Network:
		return resourceInfo{Name: v.Name, ID: v.ID}
	case *hcloud.PlacementGroup:
		return resourceInfo{Name: v.Name, ID: v.ID}
	case *hcloud.SSHKey:
		return resourceInfo{Name: v.Name, ID: v.ID}
	case *hcloud.Certificate:
		return resourceInfo{Name: v.Name, ID: v.ID}
	default:
		return resourceInfo{}
	}
}

// deleteResourcesByLabel is a generic helper for deleting resources by label selector.
func deleteResourcesByLabel[T resource](
	ctx context.Context,
	resourceType string,
	listFn func(context.Context) ([]T, error),
	deleteFn func(context.Context, T) error,
) error {
	resources, err := listFn(ctx)
	if err != nil {
		return fmt.Errorf("failed to list %s: %w", resourceType, err)
	}

	for _, r := range resources {
		info := getResourceInfo(r)
		log.Printf("[Cleanup] Deleting %s: %s (ID: %d)", resourceType, info.Name, info.ID)
		if err := deleteFn(ctx, r); err != nil {
			log.Printf("[Cleanup] Warning: Failed to delete %s %s: %v", resourceType, info.Name, err)
		}
	}

	return nil
}

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
	// 8. Certificates

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
	return deleteResourcesByLabel(ctx, "server",
		func(ctx context.Context) ([]*hcloud.Server, error) {
			return c.client.Server.AllWithOpts(ctx, hcloud.ServerListOpts{
				ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
			})
		},
		func(ctx context.Context, s *hcloud.Server) error {
			_, _, err := c.client.Server.DeleteWithResult(ctx, s)
			return err
		},
	)
}

// deleteLoadBalancersByLabel deletes all load balancers matching the label selector.
func (c *RealClient) deleteLoadBalancersByLabel(ctx context.Context, labelSelector string) error {
	return deleteResourcesByLabel(ctx, "load balancer",
		func(ctx context.Context) ([]*hcloud.LoadBalancer, error) {
			return c.client.LoadBalancer.AllWithOpts(ctx, hcloud.LoadBalancerListOpts{
				ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
			})
		},
		func(ctx context.Context, lb *hcloud.LoadBalancer) error {
			_, err := c.client.LoadBalancer.Delete(ctx, lb)
			return err
		},
	)
}

// deleteFloatingIPsByLabel deletes all floating IPs matching the label selector.
func (c *RealClient) deleteFloatingIPsByLabel(ctx context.Context, labelSelector string) error {
	return deleteResourcesByLabel(ctx, "floating IP",
		func(ctx context.Context) ([]*hcloud.FloatingIP, error) {
			return c.client.FloatingIP.AllWithOpts(ctx, hcloud.FloatingIPListOpts{
				ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
			})
		},
		func(ctx context.Context, fip *hcloud.FloatingIP) error {
			_, err := c.client.FloatingIP.Delete(ctx, fip)
			return err
		},
	)
}

// deleteFirewallsByLabel deletes all firewalls matching the label selector.
func (c *RealClient) deleteFirewallsByLabel(ctx context.Context, labelSelector string) error {
	return deleteResourcesByLabel(ctx, "firewall",
		func(ctx context.Context) ([]*hcloud.Firewall, error) {
			return c.client.Firewall.AllWithOpts(ctx, hcloud.FirewallListOpts{
				ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
			})
		},
		func(ctx context.Context, fw *hcloud.Firewall) error {
			_, err := c.client.Firewall.Delete(ctx, fw)
			return err
		},
	)
}

// deleteNetworksByLabel deletes all networks matching the label selector.
func (c *RealClient) deleteNetworksByLabel(ctx context.Context, labelSelector string) error {
	return deleteResourcesByLabel(ctx, "network",
		func(ctx context.Context) ([]*hcloud.Network, error) {
			return c.client.Network.AllWithOpts(ctx, hcloud.NetworkListOpts{
				ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
			})
		},
		func(ctx context.Context, n *hcloud.Network) error {
			_, err := c.client.Network.Delete(ctx, n)
			return err
		},
	)
}

// deletePlacementGroupsByLabel deletes all placement groups matching the label selector.
func (c *RealClient) deletePlacementGroupsByLabel(ctx context.Context, labelSelector string) error {
	return deleteResourcesByLabel(ctx, "placement group",
		func(ctx context.Context) ([]*hcloud.PlacementGroup, error) {
			return c.client.PlacementGroup.AllWithOpts(ctx, hcloud.PlacementGroupListOpts{
				ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
			})
		},
		func(ctx context.Context, pg *hcloud.PlacementGroup) error {
			_, err := c.client.PlacementGroup.Delete(ctx, pg)
			return err
		},
	)
}

// deleteSSHKeysByLabel deletes all SSH keys matching the label selector.
func (c *RealClient) deleteSSHKeysByLabel(ctx context.Context, labelSelector string) error {
	return deleteResourcesByLabel(ctx, "SSH key",
		func(ctx context.Context) ([]*hcloud.SSHKey, error) {
			return c.client.SSHKey.AllWithOpts(ctx, hcloud.SSHKeyListOpts{
				ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
			})
		},
		func(ctx context.Context, k *hcloud.SSHKey) error {
			_, err := c.client.SSHKey.Delete(ctx, k)
			return err
		},
	)
}

// deleteCertificatesByLabel deletes all certificates matching the label selector.
func (c *RealClient) deleteCertificatesByLabel(ctx context.Context, labelSelector string) error {
	return deleteResourcesByLabel(ctx, "certificate",
		func(ctx context.Context) ([]*hcloud.Certificate, error) {
			return c.client.Certificate.AllWithOpts(ctx, hcloud.CertificateListOpts{
				ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
			})
		},
		func(ctx context.Context, cert *hcloud.Certificate) error {
			_, err := c.client.Certificate.Delete(ctx, cert)
			return err
		},
	)
}
