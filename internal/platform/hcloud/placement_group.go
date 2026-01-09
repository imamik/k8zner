package hcloud

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// EnsurePlacementGroup ensures that a placement group exists with the given specifications.
func (c *RealClient) EnsurePlacementGroup(ctx context.Context, name, pgType string, labels map[string]string) (*hcloud.PlacementGroup, error) {
	return (&EnsureOperation[*hcloud.PlacementGroup, hcloud.PlacementGroupCreateOpts, any]{
		Name:         name,
		ResourceType: "placement group",
		Get:          c.client.PlacementGroup.Get,
		Create:       c.createPlacementGroup,
		CreateOptsMapper: func() hcloud.PlacementGroupCreateOpts {
			return hcloud.PlacementGroupCreateOpts{
				Name:   name,
				Type:   hcloud.PlacementGroupType(pgType),
				Labels: labels,
			}
		},
	}).Execute(ctx, c)
}

func (c *RealClient) createPlacementGroup(ctx context.Context, opts hcloud.PlacementGroupCreateOpts) (*CreateResult[*hcloud.PlacementGroup], *hcloud.Response, error) {
	res, resp, err := c.client.PlacementGroup.Create(ctx, opts)
	if err != nil {
		return nil, resp, err
	}
	return &CreateResult[*hcloud.PlacementGroup]{Resource: res.PlacementGroup}, resp, nil
}

// DeletePlacementGroup deletes the placement group with the given name.
func (c *RealClient) DeletePlacementGroup(ctx context.Context, name string) error {
	return (&DeleteOperation[*hcloud.PlacementGroup]{
		Name:         name,
		ResourceType: "placement group",
		Get:          c.client.PlacementGroup.Get,
		Delete:       c.client.PlacementGroup.Delete,
	}).Execute(ctx, c)
}

// GetPlacementGroup returns the placement group with the given name.
func (c *RealClient) GetPlacementGroup(ctx context.Context, name string) (*hcloud.PlacementGroup, error) {
	pg, _, err := c.client.PlacementGroup.Get(ctx, name)
	return pg, err
}
