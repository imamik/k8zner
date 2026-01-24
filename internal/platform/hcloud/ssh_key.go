package hcloud

import (
	"context"
	"fmt"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// CreateSSHKey creates a new SSH key.
func (c *RealClient) CreateSSHKey(ctx context.Context, name, publicKey string, labels map[string]string) (string, error) {
	opts := hcloud.SSHKeyCreateOpts{
		Name:      name,
		PublicKey: publicKey,
		Labels:    labels,
	}
	key, _, err := c.client.SSHKey.Create(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("failed to create ssh key: %w", err)
	}
	return fmt.Sprintf("%d", key.ID), nil
}

// DeleteSSHKey deletes the SSH key with the given name.
func (c *RealClient) DeleteSSHKey(ctx context.Context, name string) error {
	return (&DeleteOperation[*hcloud.SSHKey]{
		Name:         name,
		ResourceType: "ssh key",
		Get:          c.client.SSHKey.Get,
		Delete:       c.client.SSHKey.Delete,
	}).Execute(ctx, c)
}
