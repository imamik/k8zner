// Package cluster provides the reconciliation logic for provisioning and managing Hetzner Cloud resources.
package cluster

import (
	"context"
	"fmt"
	"log"

	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/hcloud"
)

// Reconciler is responsible for reconciling the state of the cluster.
// TalosConfigProducer defines the interface for generating Talos configuration.
type TalosConfigProducer interface {
	GenerateControlPlaneConfig(san []string) ([]byte, error)
	GenerateWorkerConfig() ([]byte, error)
}

// Reconciler is responsible for reconciling the state of the cluster.
type Reconciler struct {
	serverProvisioner hcloud.ServerProvisioner
	talosGenerator    TalosConfigProducer
	config            *config.Config
}

// NewReconciler creates a new Reconciler.
func NewReconciler(
	serverProvisioner hcloud.ServerProvisioner,
	talosGenerator TalosConfigProducer,
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
	// Enforce SSH Keys
	if len(r.config.SSHKeys) == 0 {
		return fmt.Errorf("no ssh_keys provided in configuration; ssh keys are required to prevent password emails and ensure access")
	}

	count := r.config.ControlPlane.Count
	if count <= 0 {
		count = 1 // Default to 1 if not specified
	}

	imageName := r.config.ControlPlane.Image
	if imageName == "" {
		imageName = "talos" // Default if not specified, but usually should come from config
	}

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

		log.Printf("Creating server %s...", name)

		// Labels for the server
		labels := map[string]string{
			"cluster": r.config.ClusterName,
			"role":    "control-plane",
		}

		// Create Server
		serverType := r.config.ControlPlane.ServerType
		if serverType == "" {
			serverType = "cpx21" // Default
		}

		location := r.config.Location
		// If location is empty, we pass empty string, and the client will pass nil, relying on Hetzner project defaults.

		_, err = r.serverProvisioner.CreateServer(ctx, name, imageName, serverType, location, r.config.SSHKeys, labels, string(cfgBytes))
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
