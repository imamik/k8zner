package infra

import (
	"context"
	"fmt"
	"net"

	"github.com/hcloud-k8s/internal/config"
	"github.com/hcloud-k8s/internal/hcloud"
	hcloudlib "github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type Manager struct {
	client hcloud.Client
	cfg    *config.Config
}

func NewManager(client hcloud.Client, cfg *config.Config) *Manager {
	return &Manager{
		client: client,
		cfg:    cfg,
	}
}

// EnsureNetwork ensures the network exists and has the correct subnets.
func (m *Manager) EnsureNetwork(ctx context.Context) error {
	networkClient := m.client.Network()

	// 1. Check if network exists
	network, _, err := networkClient.Get(ctx, m.cfg.ClusterName)
	if err != nil {
		return fmt.Errorf("failed to get network: %w", err)
	}

	if network != nil {
		// Network exists, logic to reconcile subnets could go here
		// For now we assume it is correct or manual intervention
		return nil
	}

	// 2. Create network
	_, ipRange, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}

	subnets := []hcloudlib.NetworkSubnet{
		{
			Type:        hcloudlib.NetworkSubnetTypeCloud,
			IPRange:     &net.IPNet{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(24, 32)},
			NetworkZone: hcloudlib.NetworkZone(m.cfg.Hetzner.NetworkZone),
		},
	}

	opts := hcloudlib.NetworkCreateOpts{
		Name:    m.cfg.ClusterName,
		IPRange: ipRange,
		Subnets: subnets,
		Labels: map[string]string{
			"cluster": m.cfg.ClusterName,
		},
	}

	_, _, err = networkClient.Create(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to create network: %w", err)
	}

	return nil
}
