package handlers

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/provisioning/destroy"
)

// Provisioner interface for testing - matches provisioning.Phase.
type Provisioner interface {
	Provision(ctx *provisioning.Context) error
}

// Factory function variables for destroy - can be replaced in tests.
var (
	// newDestroyProvisioner creates a new destroy provisioner.
	newDestroyProvisioner = func() Provisioner {
		return destroy.NewProvisioner()
	}

	// newProvisioningContext creates a new provisioning context.
	newProvisioningContext = provisioning.NewContext
)

// Destroy handles the destroy command.
//
// It loads the cluster configuration and deletes all associated resources
// from Hetzner Cloud. Resources are deleted in dependency order.
func Destroy(ctx context.Context, configPath string) error {
	// Load configuration using v2 loader
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	log.Printf("Destroying cluster: %s", cfg.ClusterName)

	// Initialize Hetzner Cloud client using environment variable
	token := os.Getenv("HCLOUD_TOKEN")
	infraClient := newInfraClient(token)

	// Create provisioning context (no Talos generator needed for destroy)
	pCtx := newProvisioningContext(ctx, cfg, infraClient, nil)

	// Create destroy provisioner
	destroyer := newDestroyProvisioner()

	// Execute destroy
	if err := destroyer.Provision(pCtx); err != nil {
		return fmt.Errorf("destroy failed: %w", err)
	}

	log.Printf("Cluster %s destroyed successfully", cfg.ClusterName)
	return nil
}
