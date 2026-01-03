package hcloud

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
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
	pg, _, err := c.client.PlacementGroup.Get(ctx, name)
	if err != nil {
		return err
	}
	if pg == nil {
		return nil
	}
	_, err = c.client.PlacementGroup.Delete(ctx, pg)
	return err
}

// GetPlacementGroup returns the placement group with the given name.
func (c *RealClient) GetPlacementGroup(ctx context.Context, name string) (*hcloud.PlacementGroup, error) {
	pg, _, err := c.client.PlacementGroup.Get(ctx, name)
	return pg, err
}
