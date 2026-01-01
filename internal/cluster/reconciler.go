package cluster

import (
	"context"
	"fmt"

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

	return nil
}
