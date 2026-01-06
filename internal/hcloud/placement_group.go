package hcloud

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// EnsurePlacementGroup ensures that a placement group exists with the given specifications.
func (c *RealClient) EnsurePlacementGroup(ctx context.Context, name, pgType string, labels map[string]string) (*hcloud.PlacementGroup, error) {
	return reconcileResource(ctx, name, ReconcileFuncs[hcloud.PlacementGroup]{
		Get: func(ctx context.Context, name string) (*hcloud.PlacementGroup, error) {
			pg, _, err := c.client.PlacementGroup.Get(ctx, name)
			return pg, err
		},
		Create: func(ctx context.Context) (*hcloud.PlacementGroup, error) {
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
		},
		NeedsUpdate: nil,
		Update:      nil,
	})
}

// DeletePlacementGroup deletes the placement group with the given name.
func (c *RealClient) DeletePlacementGroup(ctx context.Context, name string) error {
	return deleteResource(ctx, name, DeleteFuncs[hcloud.PlacementGroup]{
		Get: func(ctx context.Context, name string) (*hcloud.PlacementGroup, error) {
			pg, _, err := c.client.PlacementGroup.Get(ctx, name)
			return pg, err
		},
		Delete: func(ctx context.Context, pg *hcloud.PlacementGroup) error {
			_, err := c.client.PlacementGroup.Delete(ctx, pg)
			return err
		},
	}, c.getGenericTimeouts())
}

// GetPlacementGroup returns the placement group with the given name.
func (c *RealClient) GetPlacementGroup(ctx context.Context, name string) (*hcloud.PlacementGroup, error) {
	pg, _, err := c.client.PlacementGroup.Get(ctx, name)
	return pg, err
}
