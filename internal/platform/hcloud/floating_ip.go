package hcloud

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// floatingIPCreateParams holds parameters for creating a floating IP.
type floatingIPCreateParams struct {
	name         string
	homeLocation string
	ipType       string
	labels       map[string]string
}

// EnsureFloatingIP ensures that a floating IP exists with the given specifications.
func (c *RealClient) EnsureFloatingIP(ctx context.Context, name, homeLocation, ipType string, labels map[string]string) (*hcloud.FloatingIP, error) {
	params := floatingIPCreateParams{
		name:         name,
		homeLocation: homeLocation,
		ipType:       ipType,
		labels:       labels,
	}

	return (&EnsureOperation[*hcloud.FloatingIP, floatingIPCreateParams, any]{
		Name:         name,
		ResourceType: "floating IP",
		Get:          c.client.FloatingIP.Get,
		Create:       c.createFloatingIPWithDeps,
		CreateOptsMapper: func() floatingIPCreateParams {
			return params
		},
	}).Execute(ctx, c)
}

// createFloatingIPWithDeps resolves dependencies and creates a floating IP.
func (c *RealClient) createFloatingIPWithDeps(ctx context.Context, params floatingIPCreateParams) (*CreateResult[*hcloud.FloatingIP], *hcloud.Response, error) {
	// Resolve location dependency (only when creating)
	loc, _, err := c.client.Location.Get(ctx, params.homeLocation)
	if err != nil {
		return nil, nil, err
	}

	// Build final opts with resolved dependencies
	opts := hcloud.FloatingIPCreateOpts{
		Name:         &params.name,
		Type:         hcloud.FloatingIPType(params.ipType),
		HomeLocation: loc,
		Labels:       params.labels,
	}

	// Create the floating IP
	res, resp, err := c.client.FloatingIP.Create(ctx, opts)
	if err != nil {
		return nil, resp, err
	}
	return &CreateResult[*hcloud.FloatingIP]{Resource: res.FloatingIP}, resp, nil
}

// DeleteFloatingIP deletes the floating IP with the given name.
func (c *RealClient) DeleteFloatingIP(ctx context.Context, name string) error {
	return (&DeleteOperation[*hcloud.FloatingIP]{
		Name:         name,
		ResourceType: "floating IP",
		Get:          c.client.FloatingIP.Get,
		Delete:       c.client.FloatingIP.Delete,
	}).Execute(ctx, c)
}

// GetFloatingIP returns the floating IP with the given name.
func (c *RealClient) GetFloatingIP(ctx context.Context, name string) (*hcloud.FloatingIP, error) {
	fip, _, err := c.client.FloatingIP.Get(ctx, name)
	return fip, err
}
