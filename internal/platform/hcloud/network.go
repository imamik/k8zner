package hcloud

import (
	"context"
	"fmt"
	"net"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// EnsureNetwork ensures that a network exists with the given specifications.
func (c *RealClient) EnsureNetwork(ctx context.Context, name, ipRange, _ string, labels map[string]string) (*hcloud.Network, error) {
	return (&EnsureOperation[*hcloud.Network, hcloud.NetworkCreateOpts, any]{
		Name:         name,
		ResourceType: "network",
		Get:          c.client.Network.Get,
		Create:       simpleCreate(c.client.Network.Create),
		Validate: func(network *hcloud.Network) error {
			if network.IPRange.String() != ipRange {
				return fmt.Errorf("network %s exists but with different IP range %s (expected %s)",
					name, network.IPRange.String(), ipRange)
			}
			return nil
		},
		CreateOptsMapper: func() hcloud.NetworkCreateOpts {
			_, ipNet, _ := net.ParseCIDR(ipRange)
			return hcloud.NetworkCreateOpts{
				Name:    name,
				IPRange: ipNet,
				Labels:  labels,
			}
		},
	}).Execute(ctx, c)
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
	return (&DeleteOperation[*hcloud.Network]{
		Name:         name,
		ResourceType: "network",
		Get:          c.client.Network.Get,
		Delete:       c.client.Network.Delete,
	}).Execute(ctx, c)
}

// GetNetwork returns the network with the given name.
func (c *RealClient) GetNetwork(ctx context.Context, name string) (*hcloud.Network, error) {
	network, _, err := c.client.Network.Get(ctx, name)
	return network, err
}
