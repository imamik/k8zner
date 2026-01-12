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
//   - Cilium CNI (via Helm)
//   - Hetzner Cloud Controller Manager (CCM)
//   - Hetzner Cloud CSI Driver
//
// The kubeconfig must be valid and the cluster must be accessible.
// Addon manifests are embedded in the binary and processed as templates
// with cluster-specific configuration injected at runtime.
//
// Installation order matters: Cilium (CNI) must be installed first as it
// provides pod networking required by other components.
func Apply(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	if len(kubeconfig) == 0 {
		return fmt.Errorf("kubeconfig is required for addon installation")
	}

	controlPlaneCount := getControlPlaneCount(cfg)

	// Install Cilium CNI first (required for cluster networking)
	if cfg.Kubernetes.CNI.Enabled {
		if err := applyCilium(ctx, cfg, kubeconfig, controlPlaneCount); err != nil {
			return fmt.Errorf("failed to install Cilium: %w", err)
		}
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
		defaultStorageClass := cfg.Addons.CSI.DefaultStorageClass
		if err := applyCSI(ctx, kubeconfig, controlPlaneCount, defaultStorageClass); err != nil {
			return fmt.Errorf("failed to install CSI: %w", err)
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
