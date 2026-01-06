package hcloud

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// EnsureFloatingIP ensures that a floating IP exists with the given specifications.
func (c *RealClient) EnsureFloatingIP(ctx context.Context, name, homeLocation, ipType string, labels map[string]string) (*hcloud.FloatingIP, error) {
	return reconcileResource(ctx, name, ReconcileFuncs[hcloud.FloatingIP]{
		Get: func(ctx context.Context, name string) (*hcloud.FloatingIP, error) {
			fip, _, err := c.client.FloatingIP.Get(ctx, name)
			return fip, err
		},
		Create: func(ctx context.Context) (*hcloud.FloatingIP, error) {
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
		},
		NeedsUpdate: nil,
		Update:      nil,
	})
}

// DeleteFloatingIP deletes the floating IP with the given name.
func (c *RealClient) DeleteFloatingIP(ctx context.Context, name string) error {
	return deleteResource(ctx, name, DeleteFuncs[hcloud.FloatingIP]{
		Get: func(ctx context.Context, name string) (*hcloud.FloatingIP, error) {
			fip, _, err := c.client.FloatingIP.Get(ctx, name)
			return fip, err
		},
		Delete: func(ctx context.Context, fip *hcloud.FloatingIP) error {
			_, err := c.client.FloatingIP.Delete(ctx, fip)
			return err
		},
	}, c.getGenericTimeouts())
}

// GetFloatingIP returns the floating IP with the given name.
func (c *RealClient) GetFloatingIP(ctx context.Context, name string) (*hcloud.FloatingIP, error) {
	fip, _, err := c.client.FloatingIP.Get(ctx, name)
	return fip, err
}
