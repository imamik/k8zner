package hcloud

import (
	"context"
	"fmt"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/retry"
)

// EnsureFloatingIP ensures that a floating IP exists with the given specifications.
func (c *RealClient) EnsureFloatingIP(ctx context.Context, name, homeLocation, ipType string, labels map[string]string) (*hcloud.FloatingIP, error) {
	fip, _, err := c.client.FloatingIP.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if fip != nil {
		return fip, nil
	}

	loc, _, err := c.client.Location.Get(ctx, homeLocation)
	if err != nil {
		return nil, err
	}

	opts := hcloud.FloatingIPCreateOpts{
		Name:         &name,
		Type:         hcloud.FloatingIPType(ipType),
		HomeLocation: loc,
		Labels:       labels,
	}
	res, _, err := c.client.FloatingIP.Create(ctx, opts)
	if err != nil {
		return nil, err
	}
	return res.FloatingIP, nil
}

// DeleteFloatingIP deletes the floating IP with the given name.
func (c *RealClient) DeleteFloatingIP(ctx context.Context, name string) error {
	// Add timeout context for delete operation
	ctx, cancel := context.WithTimeout(ctx, c.timeouts.Delete)
	defer cancel()

	// Delete with retry logic (resource might be locked)
	return retry.WithExponentialBackoff(ctx, func() error {
		fip, _, err := c.client.FloatingIP.Get(ctx, name)
		if err != nil {
			return retry.Fatal(fmt.Errorf("failed to get floating IP: %w", err))
		}
		if fip == nil {
			return nil // Floating IP already deleted
		}

		_, err = c.client.FloatingIP.Delete(ctx, fip)
		if err != nil {
			// Check if resource is locked (retryable)
			if isResourceLocked(err) {
				return err
			}
			// Other errors are fatal
			return retry.Fatal(err)
		}
		return nil
	}, retry.WithMaxRetries(c.timeouts.RetryMaxAttempts), retry.WithInitialDelay(c.timeouts.RetryInitialDelay))
}

// GetFloatingIP returns the floating IP with the given name.
func (c *RealClient) GetFloatingIP(ctx context.Context, name string) (*hcloud.FloatingIP, error) {
	fip, _, err := c.client.FloatingIP.Get(ctx, name)
	return fip, err
}

// AssignFloatingIP assigns the floating IP to the server.
func (c *RealClient) AssignFloatingIP(ctx context.Context, fip *hcloud.FloatingIP, serverID int64) error {
	server := &hcloud.Server{ID: serverID}
	action, _, err := c.client.FloatingIP.Assign(ctx, fip, server)
	if err != nil {
		return fmt.Errorf("failed to assign floating IP: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, action); err != nil {
		return fmt.Errorf("failed to wait for floating IP assignment: %w", err)
	}

	return nil
}
