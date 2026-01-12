// Package addons provides functionality for installing cluster addons.
//
// This package handles the application of Kubernetes manifests for various
// addons like Cloud Controller Manager (CCM), CSI drivers, and monitoring tools.
// Manifests are embedded at build time and can use Go templates for configuration.
package addons

import (
	"context"
	"embed"
	"fmt"
	"os"

	"hcloud-k8s/internal/config"
)

//go:embed manifests/*
var manifestsFS embed.FS

// Apply installs configured addons to the Kubernetes cluster.
//
// This function checks the addon configuration and applies the appropriate
// manifests to the cluster using kubectl. Currently supports:
//   - Hetzner Cloud Controller Manager (CCM)
//   - Hetzner Cloud CSI Driver
//
// The kubeconfig must be valid and the cluster must be accessible.
// Addon manifests are embedded in the binary and processed as templates
// with cluster-specific configuration injected at runtime.
func Apply(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	if len(kubeconfig) == 0 {
		return fmt.Errorf("kubeconfig is required for addon installation")
	}

	tmpKubeconfig, err := writeTempKubeconfig(kubeconfig)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmpKubeconfig)
	}()

	if cfg.Addons.CCM.Enabled {
		if err := applyCCM(ctx, tmpKubeconfig, cfg.HCloudToken, networkID); err != nil {
			return fmt.Errorf("failed to install CCM: %w", err)
		}
	}

	if cfg.Addons.CSI.Enabled {
		controlPlaneCount := getControlPlaneCount(cfg)
		defaultStorageClass := cfg.Addons.CSI.DefaultStorageClass
		if err := applyCSI(ctx, tmpKubeconfig, cfg.HCloudToken, controlPlaneCount, defaultStorageClass); err != nil {
			return fmt.Errorf("failed to install CSI: %w", err)
		}
	}

	if cfg.Addons.MetricsServer.Enabled {
		if err := applyMetricsServer(ctx, tmpKubeconfig, cfg); err != nil {
			return fmt.Errorf("failed to install Metrics Server: %w", err)
		}
	}

	if cfg.Addons.CertManager.Enabled {
		if err := applyCertManager(ctx, tmpKubeconfig, cfg); err != nil {
			return fmt.Errorf("failed to install Cert Manager: %w", err)
		}
	}

	return nil
}

// getControlPlaneCount returns the total number of control plane nodes.
func getControlPlaneCount(cfg *config.Config) int {
	count := 0
	for _, pool := range cfg.ControlPlane.NodePools {
		count += pool.Count
	}
	return count
}
