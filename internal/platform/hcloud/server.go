package hcloud

import (
	"context"
	"fmt"
	"strconv"

	"hcloud-k8s/internal/util/retry"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// CreateServer creates a new server with the given specifications.
func (c *RealClient) CreateServer(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string, placementGroupID *int64, networkID int64, privateIP string) (string, error) {
	// Validate network parameters: both must be provided together or both empty
	if (networkID != 0) != (privateIP != "") {
		return "", fmt.Errorf("networkID and privateIP must both be provided or both be empty")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeouts.ServerCreate)
	defer cancel()

	// Resolve dependencies and build create options
	opts, err := c.buildServerCreateOpts(ctx, name, imageType, serverType, location, sshKeys, labels, userData, placementGroupID, networkID, privateIP)
	if err != nil {
		return "", err
	}

	// Create server with retry
	result, err := c.createServerWithRetry(ctx, opts)
	if err != nil {
		return "", err
	}

	// Attach to network if requested
	if networkID != 0 && privateIP != "" {
		if err := c.attachServerToNetwork(ctx, result.Server, networkID, privateIP); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("%d", result.Server.ID), nil
}

// buildServerCreateOpts resolves all dependencies and builds server creation options.
func (c *RealClient) buildServerCreateOpts(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string, placementGroupID *int64, networkID int64, privateIP string) (hcloud.ServerCreateOpts, error) {
	// Resolve server type
	serverTypeObj, _, err := c.client.ServerType.Get(ctx, serverType)
	if err != nil {
		return hcloud.ServerCreateOpts{}, fmt.Errorf("failed to get server type: %w", err)
	}
	if serverTypeObj == nil {
		return hcloud.ServerCreateOpts{}, fmt.Errorf("server type not found: %s", serverType)
	}

	// Resolve image
	imageObj, err := c.resolveImage(ctx, imageType, serverTypeObj)
	if err != nil {
		return hcloud.ServerCreateOpts{}, err
	}

	// Resolve SSH keys
	sshKeyObjs, err := c.resolveSSHKeys(ctx, sshKeys)
	if err != nil {
		return hcloud.ServerCreateOpts{}, err
	}

	// Resolve location
	locObj, err := c.resolveLocation(ctx, location)
	if err != nil {
		return hcloud.ServerCreateOpts{}, err
	}

	// Determine if server should start after creation
	var startAfterCreate *bool
	if networkID != 0 && privateIP != "" {
		startAfterCreate = hcloud.Ptr(false)
	}

	return hcloud.ServerCreateOpts{
		Name:             name,
		ServerType:       serverTypeObj,
		Image:            imageObj,
		SSHKeys:          sshKeyObjs,
		Labels:           labels,
		UserData:         userData,
		Location:         locObj,
		PlacementGroup:   resolvePlacementGroup(placementGroupID),
		StartAfterCreate: startAfterCreate,
	}, nil
}

// createServerWithRetry creates a server with exponential backoff retry logic.
func (c *RealClient) createServerWithRetry(ctx context.Context, opts hcloud.ServerCreateOpts) (hcloud.ServerCreateResult, error) {
	var result hcloud.ServerCreateResult

	err := retry.WithExponentialBackoff(ctx, func() error {
		res, _, err := c.client.Server.Create(ctx, opts)
		if err != nil {
			if isInvalidParameter(err) {
				return retry.Fatal(err)
			}
			return err
		}
		result = res
		return nil
	}, retry.WithMaxRetries(c.timeouts.RetryMaxAttempts), retry.WithInitialDelay(c.timeouts.RetryInitialDelay))

	if err != nil {
		return result, fmt.Errorf("failed to create server: %w", err)
	}

	// Wait for server creation to complete
	if err := c.client.Action.WaitFor(ctx, result.Action); err != nil {
		return result, fmt.Errorf("failed to wait for server creation: %w", err)
	}

	return result, nil
}

// DeleteServer deletes the server with the given name.
func (c *RealClient) DeleteServer(ctx context.Context, name string) error {
	return (&DeleteOperation[*hcloud.Server]{
		Name:         name,
		ResourceType: "server",
		Get:          c.client.Server.Get,
		Delete: func(ctx context.Context, server *hcloud.Server) (*hcloud.Response, error) {
			_, resp, err := c.client.Server.DeleteWithResult(ctx, server)
			return resp, err
		},
	}).Execute(ctx, c)
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

// GetServersByLabel returns all servers matching the given labels.
func (c *RealClient) GetServersByLabel(ctx context.Context, labels map[string]string) ([]*hcloud.Server, error) {
	labelSelector := buildLabelSelector(labels)
	servers, err := c.client.Server.AllWithOpts(ctx, hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelSelector},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}
	return servers, nil
}
