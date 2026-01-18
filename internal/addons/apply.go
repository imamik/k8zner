// Package addons provides functionality for installing cluster addons.
//
// This package handles the application of Kubernetes manifests for various
// addons like Cloud Controller Manager (CCM), CSI drivers, and monitoring tools.
// Manifests are embedded at build time and can use Go templates for configuration.
package addons

import (
	"context"
	"fmt"
	"os"

	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/provisioning"
)

// Apply installs configured addons to the Kubernetes cluster.
//
// This function checks the addon configuration and applies the appropriate
// manifests to the cluster using kubectl. Currently supports:
//   - Gateway API CRDs
//   - Prometheus Operator CRDs
//   - Cilium CNI
//   - Hetzner Cloud Controller Manager (CCM)
//   - Hetzner Cloud CSI Driver
//   - Metrics Server
//   - Cert Manager
//   - Ingress NGINX
//   - Longhorn
//   - Cluster Autoscaler
//   - RBAC
//   - OIDC RBAC
//   - Talos Backup
//
// The kubeconfig must be valid and the cluster must be accessible.
// Addon manifests are embedded in the binary and processed as templates
// with cluster-specific configuration injected at runtime.
func Apply(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64, sshKeyName string, firewallID int64, talosGen provisioning.TalosConfigProducer) error {
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

	// Install CRDs first (before addons that depend on them)
	if cfg.Addons.GatewayAPICRDs.Enabled {
		if err := applyGatewayAPICRDs(ctx, tmpKubeconfig, cfg); err != nil {
			return fmt.Errorf("failed to install Gateway API CRDs: %w", err)
		}
	}

	if cfg.Addons.PrometheusOperatorCRDs.Enabled {
		if err := applyPrometheusOperatorCRDs(ctx, tmpKubeconfig, cfg); err != nil {
			return fmt.Errorf("failed to install Prometheus Operator CRDs: %w", err)
		}
	}

	// Install Cilium CNI first (network foundation)
	if cfg.Addons.Cilium.Enabled {
		if err := applyCilium(ctx, tmpKubeconfig, cfg); err != nil {
			return fmt.Errorf("failed to install Cilium: %w", err)
		}
	}

	// Install Cluster Autoscaler (if enabled)
	if cfg.Addons.ClusterAutoscaler.Enabled && len(cfg.Autoscaler.NodePools) > 0 {
		if err := applyClusterAutoscaler(ctx, tmpKubeconfig, cfg, networkID, sshKeyName, firewallID, talosGen); err != nil {
			return fmt.Errorf("failed to install Cluster Autoscaler: %w", err)
		}
	}

	// Create hcloud secret if CCM or CSI are enabled
	if cfg.Addons.CCM.Enabled || cfg.Addons.CSI.Enabled {
		if err := createHCloudSecret(ctx, tmpKubeconfig, cfg.HCloudToken, networkID); err != nil {
			return fmt.Errorf("failed to create hcloud secret: %w", err)
		}
	}

	if cfg.Addons.CCM.Enabled {
		if err := applyCCM(ctx, tmpKubeconfig, cfg, networkID); err != nil {
			return fmt.Errorf("failed to install CCM: %w", err)
		}
	}

	if cfg.Addons.CSI.Enabled {
		if err := applyCSI(ctx, tmpKubeconfig, cfg); err != nil {
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

	if cfg.Addons.Longhorn.Enabled {
		if err := applyLonghorn(ctx, tmpKubeconfig, cfg); err != nil {
			return fmt.Errorf("failed to install Longhorn: %w", err)
		}
	}

	if cfg.Addons.IngressNginx.Enabled {
		if err := applyIngressNginx(ctx, tmpKubeconfig, cfg); err != nil {
			return fmt.Errorf("failed to install Ingress NGINX: %w", err)
		}
	}

	if cfg.Addons.RBAC.Enabled {
		if err := applyRBAC(ctx, tmpKubeconfig, cfg); err != nil {
			return fmt.Errorf("failed to install RBAC: %w", err)
		}
	}

	if cfg.Addons.OIDCRBAC.Enabled {
		if err := applyOIDC(ctx, tmpKubeconfig, cfg); err != nil {
			return fmt.Errorf("failed to install OIDC RBAC: %w", err)
		}
	}

	if cfg.Addons.TalosBackup.Enabled {
		if err := applyTalosBackup(ctx, tmpKubeconfig, cfg); err != nil {
			return fmt.Errorf("failed to install Talos Backup: %w", err)
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
