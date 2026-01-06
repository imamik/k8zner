package hcloud

import (
	"context"
	"fmt"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// EnsureSSHKey ensures that an SSH key exists.
// Note: Hetzner Cloud SSH keys are immutable. If a key with the same name exists, it is returned.
func (c *RealClient) EnsureSSHKey(ctx context.Context, name, publicKey string) (*hcloud.SSHKey, error) {
	return reconcileResource(ctx, name, ReconcileFuncs[hcloud.SSHKey]{
		Get: func(ctx context.Context, name string) (*hcloud.SSHKey, error) {
			key, _, err := c.client.SSHKey.Get(ctx, name)
			return key, err
		},
		Create: func(ctx context.Context) (*hcloud.SSHKey, error) {
			opts := hcloud.SSHKeyCreateOpts{
				Name:      name,
				PublicKey: publicKey,
			}
			key, _, err := c.client.SSHKey.Create(ctx, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to create ssh key: %w", err)
			}
			return key, nil
		},
		NeedsUpdate: nil, // SSH keys are immutable
		Update:      nil,
	})
}

// CreateSSHKey creates a new SSH key.
// Deprecated: Use EnsureSSHKey instead.
func (c *RealClient) CreateSSHKey(ctx context.Context, name, publicKey string) (string, error) {
	key, err := c.EnsureSSHKey(ctx, name, publicKey)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", key.ID), nil
}

// DeleteSSHKey deletes the SSH key with the given name.
func (c *RealClient) DeleteSSHKey(ctx context.Context, name string) error {
	return deleteResource(ctx, name, DeleteFuncs[hcloud.SSHKey]{
		Get: func(ctx context.Context, name string) (*hcloud.SSHKey, error) {
			key, _, err := c.client.SSHKey.Get(ctx, name)
			return key, err
		},
		Delete: func(ctx context.Context, key *hcloud.SSHKey) error {
			_, err := c.client.SSHKey.Delete(ctx, key)
			return err
		},
	}, c.getGenericTimeouts())
}
