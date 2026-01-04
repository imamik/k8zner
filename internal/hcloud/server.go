package hcloud

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/retry"
)

// CreateServer creates a new server with the given specifications.
func (c *RealClient) CreateServer(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string, placementGroupID *int64, networkID int64, privateIP string) (string, error) {
	// Add timeout context for server creation
	ctx, cancel := context.WithTimeout(ctx, c.timeouts.ServerCreate)
	defer cancel()

	serverTypeObj, _, err := c.client.ServerType.Get(ctx, serverType)
	if err != nil {
		return "", fmt.Errorf("failed to get server type: %w", err)
	}
	if serverTypeObj == nil {
		return "", fmt.Errorf("server type not found: %s", serverType)
	}

	var imageObj *hcloud.Image
	// Special handling for talos
	if imageType == "talos" {
		images, _, err := c.client.Image.List(ctx, hcloud.ImageListOpts{
			ListOpts: hcloud.ListOpts{
				LabelSelector: "os=talos",
			},
			Architecture: []hcloud.Architecture{serverTypeObj.Architecture},
			Sort:         []string{"created:desc"},
		})
		if err == nil && len(images) > 0 {
			imageObj = images[0]
		}
	}

	// Try to get image by name if not found via labels or if another image was requested.
	if imageObj == nil {
		imageObj, _, err = c.client.Image.Get(ctx, imageType) //nolint:staticcheck
		if err != nil {
			return "", fmt.Errorf("failed to get image: %w", err)
		}
	}

	if imageObj == nil {
		return "", fmt.Errorf("image not found: %s", imageType)
	}

	// Wait for image to be available if it's still creating
	if imageObj.Status != hcloud.ImageStatusAvailable {
		log.Printf("Image %s (%d) is in status %s, waiting for it to become available...", imageObj.Name, imageObj.ID, imageObj.Status)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		timeout := time.After(c.timeouts.ImageWait)
	waitLoop:
		for {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-timeout:
				return "", fmt.Errorf("timeout waiting for image %d to become available", imageObj.ID)
			case <-ticker.C:
				img, _, err := c.client.Image.GetByID(ctx, imageObj.ID)
				if err != nil {
					return "", fmt.Errorf("failed to get image status: %w", err)
				}
				if img.Status == hcloud.ImageStatusAvailable {
					imageObj = img
					break waitLoop
				}
				log.Printf("Still waiting for image %d (status: %s)...", imageObj.ID, img.Status)
			}
		}
	}

	// Check if image architecture matches server type architecture.
	if imageObj.Architecture != serverTypeObj.Architecture {
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

	var locObj *hcloud.Location
	if location != "" {
		locObj, _, err = c.client.Location.Get(ctx, location)
		if err != nil {
			return "", fmt.Errorf("failed to get location %s: %w", location, err)
		}
		if locObj == nil {
			return "", fmt.Errorf("location not found: %s", location)
		}
	}

	var pgObj *hcloud.PlacementGroup
	if placementGroupID != nil {
		pgObj = &hcloud.PlacementGroup{ID: *placementGroupID}
	}

	var startAfterCreate *bool
	if networkID != 0 && privateIP != "" {
		startAfterCreate = hcloud.Ptr(false)
	}

	opts := hcloud.ServerCreateOpts{
		Name:             name,
		ServerType:       serverTypeObj,
		Image:            imageObj,
		SSHKeys:          sshKeyObjs,
		Labels:           labels,
		UserData:         userData,
		Location:         locObj,
		PlacementGroup:   pgObj,
		StartAfterCreate: startAfterCreate,
	}

	// Create server with retry logic
	var result hcloud.ServerCreateResult
	err = retry.WithExponentialBackoff(ctx, func() error {
		res, _, err := c.client.Server.Create(ctx, opts)
		if err != nil {
			// Check if error is fatal (don't retry)
			if isInvalidParameter(err) {
				return retry.Fatal(err)
			}
			// Retryable error (rate limit, temporary failure, etc.)
			return err
		}
		result = res
		return nil
	}, retry.WithMaxRetries(c.timeouts.RetryMaxAttempts), retry.WithInitialDelay(c.timeouts.RetryInitialDelay))

	if err != nil {
		return "", fmt.Errorf("failed to create server: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, result.Action); err != nil {
		return "", fmt.Errorf("failed to wait for server creation: %w", err)
	}

	// Attach to Network if requested
	if networkID != 0 && privateIP != "" {
		ip := net.ParseIP(privateIP)
		if ip == nil {
			return "", fmt.Errorf("invalid private ip: %s", privateIP)
		}

		attachOpts := hcloud.ServerAttachToNetworkOpts{
			Network: &hcloud.Network{ID: networkID},
			IP:      ip,
		}

		// Attach to network with retry logic (network might not be ready immediately)
		err = retry.WithExponentialBackoff(ctx, func() error {
			action, _, err := c.client.Server.AttachToNetwork(ctx, result.Server, attachOpts)
			if err != nil {
				return err
			}
			return c.client.Action.WaitFor(ctx, action)
		}, retry.WithMaxRetries(c.timeouts.RetryMaxAttempts), retry.WithInitialDelay(c.timeouts.RetryInitialDelay))

		if err != nil {
			return "", fmt.Errorf("failed to attach server to network: %w", err)
		}

		// Power On
		action, _, err := c.client.Server.Poweron(ctx, result.Server)
		if err != nil {
			return "", fmt.Errorf("failed to power on server: %w", err)
		}
		if err := c.client.Action.WaitFor(ctx, action); err != nil {
			return "", fmt.Errorf("failed to wait for server power on: %w", err)
		}
	}

	return fmt.Sprintf("%d", result.Server.ID), nil
}

// DeleteServer deletes the server with the given name.
func (c *RealClient) DeleteServer(ctx context.Context, name string) error {
	// Add timeout context for delete operation
	ctx, cancel := context.WithTimeout(ctx, c.timeouts.Delete)
	defer cancel()

	// Delete with retry logic (resource might be locked)
	return retry.WithExponentialBackoff(ctx, func() error {
		server, _, err := c.client.Server.Get(ctx, name)
		if err != nil {
			return retry.Fatal(fmt.Errorf("failed to get server: %w", err))
		}
		if server == nil {
			return nil // Server already deleted
		}

		_, err = c.client.Server.Delete(ctx, server) //nolint:staticcheck
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
		return "", nil // Server not found, return empty ID
	}
	return fmt.Sprintf("%d", server.ID), nil
}
