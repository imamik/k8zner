package handlers

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/talos"
	"github.com/imamik/k8zner/internal/provisioning/upgrade"
)

// Factory function variables for upgrade - can be replaced in tests.
var (
	// loadSecrets loads Talos secrets from file.
	loadSecrets = talos.LoadSecrets

	// newUpgradeProvisioner creates a new upgrade provisioner.
	newUpgradeProvisioner = func(opts upgrade.ProvisionerOptions) Provisioner {
		return upgrade.NewProvisioner(opts)
	}
)

// UpgradeOptions contains options for the upgrade command.
type UpgradeOptions struct {
	ConfigPath      string
	DryRun          bool
	SkipHealthCheck bool
	K8sVersion      string
}

// Upgrade handles the upgrade command.
//
// It loads the cluster configuration and upgrades Talos OS and/or Kubernetes
// to the versions specified in the configuration. The upgrade process is:
// 1. Validate configuration changes
// 2. Upgrade control plane nodes sequentially (maintains quorum)
// 3. Upgrade worker nodes in parallel
// 4. Upgrade Kubernetes version (if changed)
// 5. Health check after completion
func Upgrade(ctx context.Context, opts UpgradeOptions) error {
	// Load configuration using v2 loader
	cfg, err := loadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}

	// Override Kubernetes version if specified
	if opts.K8sVersion != "" {
		cfg.Kubernetes.Version = opts.K8sVersion
	}

	log.Printf("Upgrading cluster: %s", cfg.ClusterName)
	log.Printf("Target Talos version: %s", cfg.Talos.Version)
	log.Printf("Target Kubernetes version: %s", cfg.Kubernetes.Version)

	if opts.DryRun {
		log.Printf("[DRY RUN] Would upgrade cluster (no changes will be made)")
	}

	// Initialize Hetzner Cloud client using environment variable
	token := os.Getenv("HCLOUD_TOKEN")
	infraClient := newInfraClient(token)

	// Initialize Talos generator
	// For upgrade, we need to load existing secrets from disk
	sb, err := loadSecrets(secretsFile)
	if err != nil {
		return fmt.Errorf("failed to load Talos secrets from %s: %w (secrets must exist for upgrade)", secretsFile, err)
	}

	endpoint := fmt.Sprintf("https://%s-kube-api:%d", cfg.ClusterName, config.KubeAPIPort)
	talosGen := newTalosGenerator(
		cfg.ClusterName,
		cfg.Kubernetes.Version,
		cfg.Talos.Version,
		endpoint,
		sb,
	)

	// Set machine config options (CoreDNS, encryption, network settings, etc.)
	talosGen.SetMachineConfigOptions(talos.NewMachineConfigOptions(cfg))

	// Create provisioning context
	pCtx := newProvisioningContext(ctx, cfg, infraClient, talosGen)

	// Create upgrade provisioner
	upgrader := newUpgradeProvisioner(upgrade.ProvisionerOptions{
		DryRun:          opts.DryRun,
		SkipHealthCheck: opts.SkipHealthCheck,
	})

	// Execute upgrade
	if err := upgrader.Provision(pCtx); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}

	if opts.DryRun {
		log.Printf("[DRY RUN] Upgrade simulation completed successfully")
	} else {
		log.Printf("Cluster %s upgraded successfully", cfg.ClusterName)
	}

	return nil
}
