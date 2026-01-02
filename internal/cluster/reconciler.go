package cluster

import (
	"context"
	"fmt"
	"log"

	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/talos"
)

// Reconciler is responsible for reconciling the state of the cluster.
type Reconciler struct {
	serverProvisioner hcloud.ServerProvisioner
	talosGenerator    *talos.ConfigGenerator
	config            *config.Config
}

// NewReconciler creates a new Reconciler.
func NewReconciler(
	serverProvisioner hcloud.ServerProvisioner,
	talosGenerator *talos.ConfigGenerator,
	cfg *config.Config,
) *Reconciler {
	return &Reconciler{
		serverProvisioner: serverProvisioner,
		talosGenerator:    talosGenerator,
		config:            cfg,
	}
}

// ReconcileServers ensures that the desired servers exist.
// This is a simplified version for the current iteration, focusing on server creation.
// It assumes networks and firewalls are already set up (or ignored for now).
func (r *Reconciler) ReconcileServers(ctx context.Context) error {
	// 1. Reconcile Control Plane
	if err := r.reconcileControlPlane(ctx); err != nil {
		return fmt.Errorf("failed to reconcile control plane: %w", err)
	}

	// 2. Reconcile Workers
	if err := r.reconcileWorkers(ctx); err != nil {
		return fmt.Errorf("failed to reconcile workers: %w", err)
	}

	return nil
}

func (r *Reconciler) reconcileControlPlane(ctx context.Context) error {
	// For this iteration, we just loop through the desired count and create if missing.
	// In a full implementation, we would query existing servers first.
	// But since ServerProvisioner.CreateServer is idempotent (as per interface doc),
	// we can rely on that or simple checking.

	// Since CreateServer idempotency depends on the implementation (hcloud client),
	// and we want to be robust, let's assume we need to check existence or rely on the provisioner.
	// The interface says "It should be idempotent", so we will trust it for now.

	// However, we need to generate different names for each node.
	// Standard naming: <cluster-name>-control-plane-<index>

	// TODO: Get actual count from config. For now assume it is in the config struct (which we need to update).
	// Let's assume Config has ControlPlane.Count.

	// We need to update internal/config/config.go to support this structure first.
	// For now, I will assume a count of 3 if not present (or fail if config is missing).

	count := 3 // Default
	// if r.config.ControlPlane.Count > 0 { count = r.config.ControlPlane.Count }

	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("%s-control-plane-%d", r.config.ClusterName, i)

		// Check if server exists
		_, err := r.serverProvisioner.GetServerID(ctx, name)
		if err == nil {
			log.Printf("Server %s already exists, skipping creation", name)
			continue
		}

		// Generate Talos Config
		// TODO: We need the Load Balancer IP or VIP for the SANs.
		// For now, we will use a placeholder or the endpoint from config.
		// In Phase 2/4 we would have the LB IP.
		sans := []string{}

		cfgBytes, err := r.talosGenerator.GenerateControlPlaneConfig(sans)
		if err != nil {
			return fmt.Errorf("failed to generate config for %s: %w", name, err)
		}

		// Create Server
		// We need to pass the config as UserData.
		// The CreateServer interface in internal/hcloud/client.go takes labels map[string]string.
		// It DOES NOT currently accept UserData. We need to update the interface.

		log.Printf("Creating server %s...", name)

		// Labels for the server
		labels := map[string]string{
			"cluster": r.config.ClusterName,
			"role":    "control-plane",
		}

		// Create Server
		// TODO: serverType and imageType should come from config
		_, err = r.serverProvisioner.CreateServer(ctx, name, "talos", "cpx21", nil, labels, string(cfgBytes))
		if err != nil {
			return fmt.Errorf("failed to create server %s: %w", name, err)
		}
	}
	return nil
}

func (r *Reconciler) reconcileWorkers(ctx context.Context) error {
	// Similar logic for workers
	return nil
}
