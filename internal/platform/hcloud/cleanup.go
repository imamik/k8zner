package hcloud

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// CleanupError represents accumulated errors from cleanup operations.
type CleanupError struct {
	Errors []error
}

func (e *CleanupError) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	return fmt.Sprintf("cleanup encountered %d errors: %v", len(e.Errors), e.Errors)
}

func (e *CleanupError) Unwrap() error {
	if len(e.Errors) == 1 {
		return e.Errors[0]
	}
	return errors.Join(e.Errors...)
}

func (e *CleanupError) Add(err error) {
	if err != nil {
		e.Errors = append(e.Errors, err)
	}
}

func (e *CleanupError) HasErrors() bool {
	return len(e.Errors) > 0
}

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
// Returns an error if listing fails, or a combined error of all deletion failures.
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

	var deleteErrs []error
	for _, r := range resources {
		info := getResourceInfo(r)
		log.Printf("[Cleanup] Deleting %s: %s (ID: %d)", resourceType, info.Name, info.ID)
		if err := deleteFn(ctx, r); err != nil {
			log.Printf("[Cleanup] Warning: Failed to delete %s %s: %v", resourceType, info.Name, err)
			deleteErrs = append(deleteErrs, fmt.Errorf("%s %q: %w", resourceType, info.Name, err))
		}
	}

	if len(deleteErrs) > 0 {
		return errors.Join(deleteErrs...)
	}
	return nil
}

// CleanupByLabel deletes all Hetzner Cloud resources matching the given label selector.
// This is useful for cleaning up E2E test resources or orphaned resources.
// Returns a CleanupError containing all errors encountered during cleanup.
// The function attempts to delete all resource types even if some deletions fail.
func (c *RealClient) CleanupByLabel(ctx context.Context, labelSelector map[string]string) error {
	log.Printf("[Cleanup] Starting cleanup for resources with labels: %v", labelSelector)

	// Build label selector string
	labelString := buildLabelSelector(labelSelector)
	cleanupErrs := &CleanupError{}

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
		cleanupErrs.Add(fmt.Errorf("servers: %w", err))
	}

	if err := c.deleteLoadBalancersByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete load balancers: %v", err)
		cleanupErrs.Add(fmt.Errorf("load balancers: %w", err))
	}

	if err := c.deleteFloatingIPsByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete floating IPs: %v", err)
		cleanupErrs.Add(fmt.Errorf("floating IPs: %w", err))
	}

	if err := c.deleteFirewallsByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete firewalls: %v", err)
		cleanupErrs.Add(fmt.Errorf("firewalls: %w", err))
	}

	if err := c.deleteNetworksByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete networks: %v", err)
		cleanupErrs.Add(fmt.Errorf("networks: %w", err))
	}

	if err := c.deletePlacementGroupsByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete placement groups: %v", err)
		cleanupErrs.Add(fmt.Errorf("placement groups: %w", err))
	}

	if err := c.deleteSSHKeysByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete SSH keys: %v", err)
		cleanupErrs.Add(fmt.Errorf("SSH keys: %w", err))
	}

	if err := c.deleteCertificatesByLabel(ctx, labelString); err != nil {
		log.Printf("[Cleanup] Warning: Failed to delete certificates: %v", err)
		cleanupErrs.Add(fmt.Errorf("certificates: %w", err))
	}

	if cleanupErrs.HasErrors() {
		log.Printf("[Cleanup] Cleanup completed with %d errors", len(cleanupErrs.Errors))
		return cleanupErrs
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

// deleteServersByLabel deletes all servers matching the label selector
// and waits for them to be fully deleted.
func (c *RealClient) deleteServersByLabel(ctx context.Context, labelSelector string) error {
	// First, list all servers to delete
	servers, err := c.client.Server.AllWithOpts(ctx, hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
	})
	if err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	// Delete all servers
	for _, s := range servers {
		log.Printf("[Cleanup] Deleting server: %s (ID: %d)", s.Name, s.ID)
		if _, _, err := c.client.Server.DeleteWithResult(ctx, s); err != nil {
			log.Printf("[Cleanup] Warning: Failed to delete server %s: %v", s.Name, err)
		}
	}

	// Wait for all servers to be fully deleted
	if len(servers) > 0 {
		log.Printf("[Cleanup] Waiting for %d servers to be fully deleted...", len(servers))
		for i := 0; i < 60; i++ { // Wait up to 5 minutes (60 * 5s)
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			remaining, err := c.client.Server.AllWithOpts(ctx, hcloud.ServerListOpts{
				ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
			})
			if err != nil {
				log.Printf("[Cleanup] Warning: Failed to check remaining servers: %v", err)
				break
			}
			if len(remaining) == 0 {
				log.Printf("[Cleanup] All servers deleted successfully")
				break
			}
			time.Sleep(5 * time.Second)
		}
	}

	return nil
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
// It retries if the firewall is still in use (e.g., servers being deleted).
func (c *RealClient) deleteFirewallsByLabel(ctx context.Context, labelSelector string) error {
	firewalls, err := c.client.Firewall.AllWithOpts(ctx, hcloud.FirewallListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
	})
	if err != nil {
		return fmt.Errorf("failed to list firewalls: %w", err)
	}

	for _, fw := range firewalls {
		log.Printf("[Cleanup] Deleting firewall: %s (ID: %d)", fw.Name, fw.ID)

		// Retry up to 30 times (2.5 minutes) in case firewall is still in use
		for i := 0; i < 30; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			_, err := c.client.Firewall.Delete(ctx, fw)
			if err == nil {
				break
			}

			// Check if error is "resource in use"
			if hcloud.IsError(err, hcloud.ErrorCodeResourceInUse) {
				if i < 29 {
					log.Printf("[Cleanup] Firewall %s still in use, waiting...", fw.Name)
					time.Sleep(5 * time.Second)
					continue
				}
			}

			log.Printf("[Cleanup] Warning: Failed to delete firewall %s: %v", fw.Name, err)
			break
		}
	}

	return nil
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
