package hcloud

import (
	"context"
	"fmt"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"hcloud-k8s/internal/retry"
)

// EnsurePlacementGroup ensures that a placement group exists with the given specifications.
func (c *RealClient) EnsurePlacementGroup(ctx context.Context, name, pgType string, labels map[string]string) (*hcloud.PlacementGroup, error) {
	pg, _, err := c.client.PlacementGroup.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if pg != nil {
		return pg, nil
	}

	opts := hcloud.PlacementGroupCreateOpts{
		Name:   name,
		Type:   hcloud.PlacementGroupType(pgType),
		Labels: labels,
	}
	res, _, err := c.client.PlacementGroup.Create(ctx, opts)
	if err != nil {
		return nil, err
	}
	return res.PlacementGroup, nil
}

// DeletePlacementGroup deletes the placement group with the given name.
func (c *RealClient) DeletePlacementGroup(ctx context.Context, name string) error {
	// Add timeout context for delete operation
	ctx, cancel := context.WithTimeout(ctx, c.timeouts.Delete)
	defer cancel()

	// Delete with retry logic (resource might be locked)
	return retry.WithExponentialBackoff(ctx, func() error {
		pg, _, err := c.client.PlacementGroup.Get(ctx, name)
		if err != nil {
			return retry.Fatal(fmt.Errorf("failed to get placement group: %w", err))
		}
		if pg == nil {
			return nil // Placement group already deleted
		}

		_, err = c.client.PlacementGroup.Delete(ctx, pg)
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

// GetPlacementGroup returns the placement group with the given name.
func (c *RealClient) GetPlacementGroup(ctx context.Context, name string) (*hcloud.PlacementGroup, error) {
	pg, _, err := c.client.PlacementGroup.Get(ctx, name)
	return pg, err
}
