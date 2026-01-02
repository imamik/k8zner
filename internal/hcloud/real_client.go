package hcloud

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// RealClient implements ServerProvisioner, SnapshotManager and SSHKeyManager using the Hetzner Cloud API.
type RealClient struct {
	client *hcloud.Client
}

// NewRealClient creates a new RealClient.
func NewRealClient(token string) *RealClient {
	return &RealClient{
		client: hcloud.NewClient(hcloud.WithToken(token)),
	}
}

// CreateServer creates a new server with the given specifications.
func (c *RealClient) CreateServer(ctx context.Context, name, imageType, serverType string, sshKeys []string, labels map[string]string) (string, error) {
	serverTypeObj, _, err := c.client.ServerType.Get(ctx, serverType)
	if err != nil {
		return "", fmt.Errorf("failed to get server type: %w", err)
	}
	if serverTypeObj == nil {
		return "", fmt.Errorf("server type not found: %s", serverType)
	}

	// Try to get image by name first.
	imageObj, _, err := c.client.Image.Get(ctx, imageType) //nolint:staticcheck
	if err != nil {
		return "", fmt.Errorf("failed to get image: %w", err)
	}

	// Check if image architecture matches server type architecture.
	if imageObj != nil && imageObj.Architecture != serverTypeObj.Architecture {
		// Mismatch. If looking for "debian-12" (or other name) we might have picked the wrong one.
		// Try to find one with correct architecture.
		images, _, err := c.client.Image.List(ctx, hcloud.ImageListOpts{
			Name:         imageType,
			Architecture: []hcloud.Architecture{serverTypeObj.Architecture},
		})
		if err != nil {
			return "", fmt.Errorf("failed to list images: %w", err)
		}
		if len(images) > 0 {
			imageObj = images[0]
		}
		// If we didn't find one, we stick with the original and let the API fail (or we could error here).
	}

	// If not found yet (e.g. imageType was ID but Get returned nil?), try list specific for "debian-12" special handling.
	if imageObj == nil {
		if imageType == "debian-12" {
			// Find debian-12 for specific architecture.
			images, _, err := c.client.Image.List(ctx, hcloud.ImageListOpts{
				Name:         "debian-12",
				Architecture: []hcloud.Architecture{serverTypeObj.Architecture},
			})
			if err != nil {
				return "", fmt.Errorf("failed to list images: %w", err)
			}
			if len(images) > 0 {
				imageObj = images[0]
			}
		}
	}

	if imageObj == nil {
		return "", fmt.Errorf("image not found: %s", imageType)
	}

	var sshKeyObjs []*hcloud.SSHKey
	for _, key := range sshKeys {
		keyObj, _, err := c.client.SSHKey.Get(ctx, key)
		if err != nil {
			return "", fmt.Errorf("failed to get ssh key %s: %w", key, err)
		}
		if keyObj == nil {
			return "", fmt.Errorf("ssh key not found: %s", key)
		}
		sshKeyObjs = append(sshKeyObjs, keyObj)
	}

	opts := hcloud.ServerCreateOpts{
		Name:       name,
		ServerType: serverTypeObj,
		Image:      imageObj,
		SSHKeys:    sshKeyObjs,
		Labels:     labels,
	}

	result, _, err := c.client.Server.Create(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("failed to create server: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, result.Action); err != nil {
		return "", fmt.Errorf("failed to wait for server creation: %w", err)
	}

	return fmt.Sprintf("%d", result.Server.ID), nil
}

// DeleteServer deletes the server with the given name.
func (c *RealClient) DeleteServer(ctx context.Context, name string) error {
	server, _, err := c.client.Server.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get server: %w", err)
	}
	if server == nil {
		return nil // Server already deleted.
	}

	_, err = c.client.Server.Delete(ctx, server) //nolint:staticcheck
	if err != nil {
		return fmt.Errorf("failed to delete server: %w", err)
	}

	return nil
}

// GetServerIP returns the public IP of the server.
func (c *RealClient) GetServerIP(ctx context.Context, name string) (string, error) {
	server, _, err := c.client.Server.Get(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to get server: %w", err)
	}
	if server == nil {
		return "", fmt.Errorf("server not found: %s", name)
	}

	if server.PublicNet.IPv4.IP == nil {
		return "", fmt.Errorf("server has no public IPv4")
	}

	return server.PublicNet.IPv4.IP.String(), nil
}

// EnableRescue enables rescue mode for the server.
func (c *RealClient) EnableRescue(ctx context.Context, serverID string, sshKeyIDs []string) (string, error) {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid server id: %s", serverID)
	}
	server := &hcloud.Server{ID: id}

	var sshKeys []*hcloud.SSHKey
	for _, kid := range sshKeyIDs {
		kidInt, err := strconv.ParseInt(kid, 10, 64)
		if err != nil {
			continue // Ignore invalid.
		}
		sshKeys = append(sshKeys, &hcloud.SSHKey{ID: kidInt})
	}

	result, _, err := c.client.Server.EnableRescue(ctx, server, hcloud.ServerEnableRescueOpts{
		SSHKeys: sshKeys,
	})
	if err != nil {
		return "", fmt.Errorf("failed to enable rescue: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, result.Action); err != nil {
		return "", fmt.Errorf("failed to wait for rescue enable: %w", err)
	}

	return result.RootPassword, nil
}

// ResetServer resets (reboots) the server.
func (c *RealClient) ResetServer(ctx context.Context, serverID string) error {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid server id: %s", serverID)
	}
	server := &hcloud.Server{ID: id}

	result, _, err := c.client.Server.Reset(ctx, server)
	if err != nil {
		return fmt.Errorf("failed to reset server: %w", err)
	}

	// Result from Reset is just an Action, not a struct with Action field like Create.
	if err := c.client.Action.WaitFor(ctx, result); err != nil {
		return fmt.Errorf("failed to wait for reset: %w", err)
	}
	return nil
}

// PoweroffServer shuts down the server.
func (c *RealClient) PoweroffServer(ctx context.Context, serverID string) error {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid server id: %s", serverID)
	}
	server := &hcloud.Server{ID: id}

	result, _, err := c.client.Server.Poweroff(ctx, server)
	if err != nil {
		return fmt.Errorf("failed to poweroff server: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, result); err != nil {
		return fmt.Errorf("failed to wait for poweroff: %w", err)
	}
	return nil
}

// GetServerID returns the ID of the server by name.
func (c *RealClient) GetServerID(ctx context.Context, name string) (string, error) {
	server, _, err := c.client.Server.Get(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to get server: %w", err)
	}
	if server == nil {
		return "", fmt.Errorf("server not found: %s", name)
	}
	return fmt.Sprintf("%d", server.ID), nil
}

// CreateSnapshot creates a snapshot of the server.
func (c *RealClient) CreateSnapshot(ctx context.Context, serverID, snapshotDescription string) (string, error) {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid server id: %s", serverID)
	}
	server := &hcloud.Server{ID: id}

	result, _, err := c.client.Server.CreateImage(ctx, server, &hcloud.ServerCreateImageOpts{
		Type:        hcloud.ImageTypeSnapshot,
		Description: &snapshotDescription,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, result.Action); err != nil {
		// Attempt to cleanup the partial/failed snapshot.
		if result.Image != nil {
			_ = c.DeleteImage(context.Background(), fmt.Sprintf("%d", result.Image.ID))
		}
		return "", fmt.Errorf("failed to wait for snapshot creation: %w", err)
	}

	return fmt.Sprintf("%d", result.Image.ID), nil
}

// DeleteImage deletes an image by ID.
func (c *RealClient) DeleteImage(ctx context.Context, imageID string) error {
	id, err := strconv.ParseInt(imageID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid image id: %s", imageID)
	}
	image := &hcloud.Image{ID: id}

	_, err = c.client.Image.Delete(ctx, image)
	if err != nil {
		return fmt.Errorf("failed to delete image: %w", err)
	}
	return nil
}

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
