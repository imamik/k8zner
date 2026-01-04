package hcloud

import (
	"context"
	"fmt"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/retry"
)

// EnsureFirewall ensures that a firewall exists with the given specifications.
func (c *RealClient) EnsureFirewall(ctx context.Context, name string, rules []hcloud.FirewallRule, labels map[string]string) (*hcloud.Firewall, error) {
	fw, _, err := c.client.Firewall.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall: %w", err)
	}

	if fw != nil {
		// Update Rules
		// We use SetRules to overwrite existing rules with the desired state
		actions, _, err := c.client.Firewall.SetRules(ctx, fw, hcloud.FirewallSetRulesOpts{
			Rules: rules,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to set firewall rules: %w", err)
		}
		if err := c.client.Action.WaitFor(ctx, actions...); err != nil {
			return nil, fmt.Errorf("failed to wait for firewall rules update: %w", err)
		}
		return fw, nil
	}

	// Create
	opts := hcloud.FirewallCreateOpts{
		Name:   name,
		Rules:  rules,
		Labels: labels,
	}
	res, _, err := c.client.Firewall.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create firewall: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, res.Actions...); err != nil {
		return nil, fmt.Errorf("failed to wait for firewall creation: %w", err)
	}

	return res.Firewall, nil
}

// DeleteFirewall deletes the firewall with the given name.
func (c *RealClient) DeleteFirewall(ctx context.Context, name string) error {
	// Add timeout context for delete operation
	ctx, cancel := context.WithTimeout(ctx, c.timeouts.Delete)
	defer cancel()

	// Delete with retry logic (resource might be locked)
	return retry.WithExponentialBackoff(ctx, func() error {
		fw, _, err := c.client.Firewall.Get(ctx, name)
		if err != nil {
			return retry.Fatal(fmt.Errorf("failed to get firewall: %w", err))
		}
		if fw == nil {
			return nil // Firewall already deleted
		}

		_, err = c.client.Firewall.Delete(ctx, fw)
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

// GetFirewall returns the firewall with the given name.
func (c *RealClient) GetFirewall(ctx context.Context, name string) (*hcloud.Firewall, error) {
	fw, _, err := c.client.Firewall.Get(ctx, name)
	return fw, err
}
