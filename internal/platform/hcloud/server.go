package hcloud

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/imamik/k8zner/internal/util/retry"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// CreateServer creates a new server with the given specifications.
// The enablePublicIPv4 and enablePublicIPv6 parameters control public IP assignment.
func (c *RealClient) CreateServer(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string, placementGroupID *int64, networkID int64, privateIP string, enablePublicIPv4, enablePublicIPv6 bool) (string, error) {
	// Validate network parameters: both must be provided together or both empty
	if (networkID != 0) != (privateIP != "") {
		return "", fmt.Errorf("networkID and privateIP must both be provided or both be empty")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeouts.ServerCreate)
	defer cancel()

	// Resolve dependencies and build create options
	opts, err := c.buildServerCreateOpts(ctx, name, imageType, serverType, location, sshKeys, labels, userData, placementGroupID, networkID, privateIP, enablePublicIPv4, enablePublicIPv6)
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
func (c *RealClient) buildServerCreateOpts(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string, placementGroupID *int64, networkID int64, privateIP string, enablePublicIPv4, enablePublicIPv6 bool) (hcloud.ServerCreateOpts, error) {
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

	// Configure public network (IPv4/IPv6)
	// Only set PublicNet if we're not using the defaults (both enabled)
	var publicNet *hcloud.ServerCreatePublicNet
	if !enablePublicIPv4 || !enablePublicIPv6 {
		publicNet = &hcloud.ServerCreatePublicNet{
			EnableIPv4: enablePublicIPv4,
			EnableIPv6: enablePublicIPv6,
		}
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
		PublicNet:        publicNet,
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
// Prefers IPv4 for backwards compatibility, falls back to IPv6 if no IPv4.
func (c *RealClient) GetServerIP(ctx context.Context, name string) (string, error) {
	server, _, err := c.client.Server.Get(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to get server: %w", err)
	}
	if server == nil {
		return "", fmt.Errorf("server not found: %s", name)
	}

	// Prefer IPv4 for backwards compatibility
	if server.PublicNet.IPv4.IP != nil {
		return server.PublicNet.IPv4.IP.String(), nil
	}

	// Fall back to IPv6 for IPv6-only servers
	if server.PublicNet.IPv6.IP != nil {
		// Hetzner assigns a /64 network to each server.
		// The server's primary address is the network address with ::1 suffix.
		// We construct this by taking the network prefix and adding ::1.
		ipv6Net := server.PublicNet.IPv6.IP.To16()
		if ipv6Net != nil {
			// Create a copy and set the host portion to ::1
			ip := make([]byte, 16)
			copy(ip, ipv6Net)
			ip[15] = 1 // Set the last byte to 1 for ::1
			return net.IP(ip).String(), nil
		}
	}

	return "", fmt.Errorf("server has no public IP (neither IPv4 nor IPv6)")
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

// GetServerByName returns the full server object by name, or nil if not found.
func (c *RealClient) GetServerByName(ctx context.Context, name string) (*hcloud.Server, error) {
	server, _, err := c.client.Server.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get server: %w", err)
	}
	return server, nil
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

// AttachServerToNetwork attaches an existing server to a network.
// If the server is already attached to the network, this is a no-op.
// The server will be powered on after successful attachment.
func (c *RealClient) AttachServerToNetwork(ctx context.Context, serverName string, networkID int64, privateIP string) error {
	// Get the server
	server, _, err := c.client.Server.Get(ctx, serverName)
	if err != nil {
		return fmt.Errorf("failed to get server %s: %w", serverName, err)
	}
	if server == nil {
		return fmt.Errorf("server not found: %s", serverName)
	}

	// Check if already attached to this network
	for _, pn := range server.PrivateNet {
		if pn.Network.ID == networkID {
			// Already attached, ensure server is running
			if server.Status != hcloud.ServerStatusRunning {
				action, _, err := c.client.Server.Poweron(ctx, server)
				if err != nil {
					return fmt.Errorf("failed to power on server: %w", err)
				}
				if err := c.client.Action.WaitFor(ctx, action); err != nil {
					return fmt.Errorf("failed to wait for server power on: %w", err)
				}
			}
			return nil // Already attached
		}
	}

	// Use the existing helper to attach and power on
	return c.attachServerToNetwork(ctx, server, networkID, privateIP)
}
