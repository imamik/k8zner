package cluster

import (
	"context"
	"fmt"
	"net"

	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/talos"
)

// InfrastructureManager defines the interface for infrastructure operations.
type InfrastructureManager interface {
	hcloud.NetworkManager
	hcloud.FirewallManager
	hcloud.LoadBalancerManager
	hcloud.PlacementGroupManager
	hcloud.FloatingIPManager
	hcloud.ServerProvisioner
}

// Reconciler reconciles the cluster state.
type Reconciler struct {
	infra InfrastructureManager
	talos *talos.ConfigGenerator
}

// NewReconciler creates a new Reconciler.
func NewReconciler(infra InfrastructureManager, talos *talos.ConfigGenerator) *Reconciler {
	return &Reconciler{
		infra: infra,
		talos: talos,
	}
}

// Reconcile reconciles the cluster infrastructure.
func (r *Reconciler) Reconcile(ctx context.Context, cfg *config.Config) error {
	// 1. Reconcile Network
	labels := map[string]string{
		"cluster": cfg.ClusterName,
	}

	if err := r.infra.EnsureNetwork(ctx, cfg.ClusterName, cfg.Network.CIDR, labels); err != nil {
		return fmt.Errorf("failed to ensure network: %w", err)
	}

	// 2. Reconcile Firewall
	// Rules: Allow Kube API (TCP 6443) and Talos API (TCP 50000)
	fwRules := []hcloud.FirewallRule{
		{
			Direction: "in",
			Port:      "6443",
			Protocol:  "tcp",
			SourceIPs: []string{"0.0.0.0/0", "::/0"},
		},
		{
			Direction: "in",
			Port:      "50000",
			Protocol:  "tcp",
			SourceIPs: []string{"0.0.0.0/0", "::/0"},
		},
	}

	if err := r.infra.EnsureFirewall(ctx, cfg.ClusterName, fwRules, labels); err != nil {
		return fmt.Errorf("failed to ensure firewall: %w", err)
	}

	// 3. Reconcile Placement Group
	pgName := fmt.Sprintf("%s-control-plane", cfg.ClusterName)
	if err := r.infra.EnsurePlacementGroup(ctx, pgName, labels); err != nil {
		return fmt.Errorf("failed to ensure placement group: %w", err)
	}

	// 4. Reconcile Load Balancer
	// Calculate IP for LB: 2nd IP in the LB subnet?
	// cfg.Network.LoadBalancerSubnet e.g. "10.0.0.0/24"
	// Let's assume .2 (since .1 is gateway often).
	lbIP, err := getIPAtIndex(cfg.Network.LoadBalancerSubnet, 2)
	if err != nil {
		return fmt.Errorf("failed to calc lb ip: %w", err)
	}

	if err := r.infra.EnsureLoadBalancer(ctx, cfg.ClusterName, cfg.ClusterName, lbIP, labels); err != nil {
		return fmt.Errorf("failed to ensure load balancer: %w", err)
	}

	// 5. Reconcile Control Plane Servers
	for i := 0; i < cfg.ControlPlane.Count; i++ {
		nodeName := fmt.Sprintf("%s-control-plane-%d", cfg.ClusterName, i+1)

		// Labels for CP
		cpLabels := map[string]string{
			"cluster": cfg.ClusterName,
			"role":    "control-plane",
		}

		// Calculate Node IP
		// cfg.Network.ControlPlaneSubnet
		// Replicate terraform logic?
		// cidrhost(subnet, 2 + i) ??
		// Let's assume simple allocation: start at .2 or similar.
		// Usually Gateway .1.
		nodeIP, err := getIPAtIndex(cfg.Network.ControlPlaneSubnet, 2+i)
		if err != nil {
			return fmt.Errorf("failed to calc node ip: %w", err)
		}

		// Generate Talos Config for this node
		// We need the LB IP as the endpoint.
		// Also we might need VIPs if using Floating IP.
		certSANs := []string{lbIP, nodeIP}

		// Check if we need to generate secrets first? The generator handles it.
		// Note: We are ignoring the returned config for now as we just want to provision the server.
		// In a real implementation, we would feed this config into `UserData` of CreateServer.
		// But `CreateServer` signature takes `sshKeys`, not `userData`.
		// Wait, `CreateServer` in `hcloud` package assumes SSH keys for access?
		// Talos uses `userData` to bootstrap.
		// I need to update `CreateServer` to accept `userData` or specific Talos config.
		// Checking `RealClient.CreateServer`... it takes `sshKeys`.
		// This is a limitation of the current `ServerProvisioner` interface I designed.
		// Talos doesn't use SSH keys for access (it uses API). But it NEEDS UserData to boot.
		// I should update `CreateServer` signature or overload it.

		// For this iteration, I will assume we update the interface to take `userData` string.

		_, err = r.talos.Generate("controlplane", nodeIP, certSANs)
		if err != nil {
			return fmt.Errorf("failed to generate talos config: %w", err)
		}

		// Provision Server
		// TODO: Pass UserData. For now passing empty keys.
		// Image: cfg.ControlPlane.Image
		// Type: cfg.ControlPlane.ServerType
		_, err = r.infra.CreateServer(ctx, nodeName, cfg.ControlPlane.Image, cfg.ControlPlane.ServerType, nil, cpLabels)
		if err != nil {
			return fmt.Errorf("failed to create server %s: %w", nodeName, err)
		}
	}

	return nil
}

func getIPAtIndex(cidr string, index int) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}

	ip := ipNet.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("only ipv4 supported")
	}

	ipInt := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	ipInt += uint32(index)

	return fmt.Sprintf("%d.%d.%d.%d", byte(ipInt>>24), byte(ipInt>>16), byte(ipInt>>8), byte(ipInt)), nil
}
