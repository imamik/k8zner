package hcloud

import (
	"context"
	"fmt"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// CreateSSHKey creates a new SSH key.
func (c *RealClient) CreateSSHKey(ctx context.Context, name, publicKey string) (string, error) {
	opts := hcloud.SSHKeyCreateOpts{
		Name:      name,
		PublicKey: publicKey,
	}
	key, _, err := c.client.SSHKey.Create(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("failed to create ssh key: %w", err)
	}
	return fmt.Sprintf("%d", key.ID), nil
}

// DeleteSSHKey deletes the SSH key with the given name.
func (c *RealClient) DeleteSSHKey(ctx context.Context, name string) error {
	key, _, err := c.client.SSHKey.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get ssh key: %w", err)
	}
	if key == nil {
		return nil
	}
	_, err = c.client.SSHKey.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete ssh key: %w", err)
	}
	return nil
}
