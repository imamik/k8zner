package hcloud

import (
	"context"
	"fmt"
	"net"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"hcloud-k8s/internal/retry"
)

// EnsureNetwork ensures that a network exists with the given specifications.
func (c *RealClient) EnsureNetwork(ctx context.Context, name, ipRange, _ string, labels map[string]string) (*hcloud.Network, error) {
	network, _, err := c.client.Network.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get network: %w", err)
	}

	if network != nil {
		// Verify IP Range matches
		if network.IPRange.String() != ipRange {
			return nil, fmt.Errorf("network %s exists but with different IP range %s (expected %s)", name, network.IPRange.String(), ipRange)
		}
		// TODO: Update labels if needed
		return network, nil
	}

	// Create
	_, ipNet, err := net.ParseCIDR(ipRange)
	if err != nil {
		return nil, fmt.Errorf("invalid ip range: %w", err)
	}

	opts := hcloud.NetworkCreateOpts{
		Name:    name,
		IPRange: ipNet,
		Labels:  labels,
	}
	network, _, err = c.client.Network.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}

	return network, nil
}

// EnsureSubnet ensures that a subnet exists in the given network.
func (c *RealClient) EnsureSubnet(ctx context.Context, network *hcloud.Network, ipRange, networkZone string, subnetType hcloud.NetworkSubnetType) error {
	// Check if subnet exists
	for _, subnet := range network.Subnets {
		if subnet.IPRange.String() == ipRange {
			return nil // Exists
		}
	}

	// Create Subnet
	_, ipNet, err := net.ParseCIDR(ipRange)
	if err != nil {
		return fmt.Errorf("invalid subnet ip range: %w", err)
	}

	opts := hcloud.NetworkAddSubnetOpts{
		Subnet: hcloud.NetworkSubnet{
			Type:        subnetType,
			IPRange:     ipNet,
			NetworkZone: hcloud.NetworkZone(networkZone),
		},
	}

	action, _, err := c.client.Network.AddSubnet(ctx, network, opts)
	if err != nil {
		return fmt.Errorf("failed to add subnet: %w", err)
	}

	if err := c.client.Action.WaitFor(ctx, action); err != nil {
		return fmt.Errorf("failed to wait for subnet creation: %w", err)
	}

	return nil
}

// DeleteNetwork deletes the network with the given name.
func (c *RealClient) DeleteNetwork(ctx context.Context, name string) error {
	// Add timeout context for delete operation
	ctx, cancel := context.WithTimeout(ctx, c.timeouts.Delete)
	defer cancel()

	// Delete with retry logic (resource might be locked)
	return retry.WithExponentialBackoff(ctx, func() error {
		network, _, err := c.client.Network.Get(ctx, name)
		if err != nil {
			return retry.Fatal(fmt.Errorf("failed to get network: %w", err))
		}
		if network == nil {
			return nil // Network already deleted
		}

		_, err = c.client.Network.Delete(ctx, network)
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

// GetNetwork returns the network with the given name.
func (c *RealClient) GetNetwork(ctx context.Context, name string) (*hcloud.Network, error) {
	network, _, err := c.client.Network.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return network, nil
}
