package hcloud

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
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
	fip, _, err := c.client.FloatingIP.Get(ctx, name)
	if err != nil {
		return err
	}
	if fip == nil {
		return nil
	}
	_, err = c.client.FloatingIP.Delete(ctx, fip)
	return err
}

// GetFloatingIP returns the floating IP with the given name.
func (c *RealClient) GetFloatingIP(ctx context.Context, name string) (*hcloud.FloatingIP, error) {
	fip, _, err := c.client.FloatingIP.Get(ctx, name)
	return fip, err
}
