package handlers

import (
	"context"
	"fmt"
	"log"

	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/provisioning/destroy"
)

// Destroy handles the destroy command.
//
// It loads the cluster configuration and deletes all associated resources
// from Hetzner Cloud. Resources are deleted in dependency order.
func Destroy(ctx context.Context, configPath string) error {
	// Load and validate configuration
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	log.Printf("Destroying cluster: %s", cfg.ClusterName)

	// Initialize Hetzner Cloud client
	infraClient, err := hcloud.NewRealClient(cfg.HCloudToken)
	if err != nil {
		return fmt.Errorf("failed to create Hetzner Cloud client: %w", err)
	}

	// Create provisioning context (no Talos generator needed for destroy)
	pCtx := provisioning.NewContext(ctx, cfg, infraClient, nil)

	// Create destroy provisioner
	destroyer := destroy.NewProvisioner()

	// Execute destroy
	if err := destroyer.Provision(pCtx); err != nil {
		return fmt.Errorf("destroy failed: %w", err)
	}

	log.Printf("Cluster %s destroyed successfully", cfg.ClusterName)
	return nil
}
