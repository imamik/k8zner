package hcloud

import (
	"context"
	"fmt"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/retry"
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
	// Add timeout context for delete operation
	ctx, cancel := context.WithTimeout(ctx, c.timeouts.Delete)
	defer cancel()

	// Delete with retry logic (resource might be locked)
	return retry.WithExponentialBackoff(ctx, func() error {
		key, _, err := c.client.SSHKey.Get(ctx, name)
		if err != nil {
			return retry.Fatal(fmt.Errorf("failed to get ssh key: %w", err))
		}
		if key == nil {
			return nil // SSH key already deleted
		}

		_, err = c.client.SSHKey.Delete(ctx, key)
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
