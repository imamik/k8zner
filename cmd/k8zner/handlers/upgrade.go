package handlers

import (
	"context"
	"fmt"
	"log"

	"k8zner/internal/config"
	"k8zner/internal/platform/hcloud"
	"k8zner/internal/platform/talos"
	"k8zner/internal/provisioning"
	"k8zner/internal/provisioning/upgrade"
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
	// Load and validate configuration
	cfg, err := config.LoadFile(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
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

	// Initialize Hetzner Cloud client
	infraClient := hcloud.NewRealClient(cfg.HCloudToken)

	// Initialize Talos generator
	// For upgrade, we need to load existing secrets from disk
	sb, err := talos.LoadSecrets(secretsFile)
	if err != nil {
		return fmt.Errorf("failed to load Talos secrets from %s: %w (secrets must exist for upgrade)", secretsFile, err)
	}

	endpoint := fmt.Sprintf("https://%s-kube-api:%d", cfg.ClusterName, config.KubeAPIPort)
	talosGen := talos.NewGenerator(
		cfg.ClusterName,
		cfg.Kubernetes.Version,
		cfg.Talos.Version,
		endpoint,
		sb,
	)

	// Create provisioning context
	pCtx := provisioning.NewContext(ctx, cfg, infraClient, talosGen)

	// Create upgrade provisioner
	upgrader := upgrade.NewProvisioner(upgrade.ProvisionerOptions{
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
