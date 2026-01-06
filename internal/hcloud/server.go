package hcloud

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/retry"
)

// EnsureServer ensures that a server exists with the given specifications.
// It returns the created or existing server.
func (c *RealClient) EnsureServer(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string, placementGroupID *int64, networkID int64, privateIP string) (*hcloud.Server, error) {
	return reconcileResource(ctx, name, ReconcileFuncs[hcloud.Server]{
		Get: func(ctx context.Context, name string) (*hcloud.Server, error) {
			server, _, err := c.client.Server.Get(ctx, name)
			return server, err
		},
		Create: func(ctx context.Context) (*hcloud.Server, error) {
			// Add timeout context for server creation
			createCtx, cancel := context.WithTimeout(ctx, c.timeouts.ServerCreate)
			defer cancel()

			// Resolve server type
			serverTypeObj, _, err := c.client.ServerType.Get(createCtx, serverType)
			if err != nil {
				return nil, fmt.Errorf("failed to get server type: %w", err)
			}
			if serverTypeObj == nil {
				return nil, fmt.Errorf("server type not found: %s", serverType)
			}

			// Resolve and wait for image
			imageObj, err := c.resolveImage(createCtx, imageType, serverTypeObj)
			if err != nil {
				return nil, err
			}

			// Resolve SSH keys
			sshKeyObjs, err := c.resolveSSHKeys(createCtx, sshKeys)
			if err != nil {
				return nil, err
			}

			// Resolve location
			locObj, err := c.resolveLocation(createCtx, location)
			if err != nil {
				return nil, err
			}

			// Resolve placement group
			pgObj := resolvePlacementGroup(placementGroupID)

			// Determine if server should start after creation
			var startAfterCreate *bool
			if networkID != 0 && privateIP != "" {
				startAfterCreate = hcloud.Ptr(false)
			}

			// Build server creation options
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
			err = retry.WithExponentialBackoff(createCtx, func() error {
				res, _, err := c.client.Server.Create(createCtx, opts)
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
				return nil, fmt.Errorf("failed to create server: %w", err)
			}

			// Wait for server creation to complete
			if err := c.client.Action.WaitFor(createCtx, result.Action); err != nil {
				return nil, fmt.Errorf("failed to wait for server creation: %w", err)
			}

			// Attach to network if requested
			if networkID != 0 && privateIP != "" {
				if err := c.attachServerToNetwork(createCtx, result.Server, networkID, privateIP); err != nil {
					return nil, err
				}
			}

			return result.Server, nil
		},
		NeedsUpdate: nil, // TODO: Implement update logic if needed
		Update:      nil,
	})
}

// CreateServer creates a new server with the given specifications.
// Deprecated: Use EnsureServer instead. This is kept for compatibility with ServerProvisioner interface.
func (c *RealClient) CreateServer(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string, placementGroupID *int64, networkID int64, privateIP string) (string, error) {
	server, err := c.EnsureServer(ctx, name, imageType, serverType, location, sshKeys, labels, userData, placementGroupID, networkID, privateIP)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", server.ID), nil
}

// DeleteServer deletes the server with the given name.
func (c *RealClient) DeleteServer(ctx context.Context, name string) error {
	return deleteResource(ctx, name, DeleteFuncs[hcloud.Server]{
		Get: func(ctx context.Context, name string) (*hcloud.Server, error) {
			server, _, err := c.client.Server.Get(ctx, name)
			return server, err
		},
		Delete: func(ctx context.Context, server *hcloud.Server) error {
			_, err := c.client.Server.Delete(ctx, server) //nolint:staticcheck
			return err
		},
	}, c.getGenericTimeouts())
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
