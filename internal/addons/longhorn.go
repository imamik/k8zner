package addons

import (
	"context"
	"fmt"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
)

// applyLonghorn installs Longhorn distributed block storage.
// See: terraform/longhorn.tf
func applyLonghorn(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
	// Create namespace with pod security labels first
	namespaceYAML := createLonghornNamespace()
	if err := applyWithKubectl(ctx, kubeconfigPath, "longhorn-namespace", []byte(namespaceYAML)); err != nil {
		return fmt.Errorf("failed to create longhorn namespace: %w", err)
	}

	// Build values matching terraform configuration
	values := buildLonghornValues(cfg)

	// Render helm chart
	manifestBytes, err := helm.RenderChart("longhorn", "longhorn-system", values)
	if err != nil {
		return fmt.Errorf("failed to render longhorn chart: %w", err)
	}

	// Apply manifests
	if err := applyWithKubectl(ctx, kubeconfigPath, "longhorn", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply longhorn manifests: %w", err)
	}

	return nil
}

// buildLonghornValues creates helm values matching terraform configuration.
// See: terraform/longhorn.tf lines 29-56
func buildLonghornValues(cfg *config.Config) helm.Values {
	clusterAutoscalerEnabled := hasClusterAutoscaler(cfg)

	return helm.Values{
		// Hotfix for https://github.com/longhorn/longhorn/issues/12259
		"image": helm.Values{
			"longhorn": helm.Values{
				"manager": helm.Values{
					"tag": "v1.10.1-hotfix-1",
				},
			},
		},
		"preUpgradeChecker": helm.Values{
			"upgradeVersionCheck": false,
		},
		"defaultSettings": helm.Values{
			"allowCollectingLonghornUsageMetrics": false,
			"kubernetesClusterAutoscalerEnabled":  clusterAutoscalerEnabled,
			"upgradeChecker":                      false,
		},
		"networkPolicies": helm.Values{
			"enabled": true,
			"type":    "rke1", // rke1 = ingress-nginx compatible
		},
		"persistence": helm.Values{
			"defaultClass": cfg.Addons.Longhorn.DefaultStorageClass,
			"backingImage": helm.Values{
				"enable": false,
			},
			"recurringJobSelector": helm.Values{
				"enable": false,
			},
		},
	}
}

// hasClusterAutoscaler checks if cluster autoscaler is configured.
// In terraform this is: local.cluster_autoscaler_enabled
// which checks if length(local.cluster_autoscaler_nodepools) > 0
func hasClusterAutoscaler(_ *config.Config) bool {
	// Check if any worker pools have autoscaling configured
	// This would need to be extended once autoscaling config is added
	// For now, return false to match typical non-autoscaling setups
	return false
}

// createLonghornNamespace returns the longhorn-system namespace with pod security labels.
func createLonghornNamespace() string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: longhorn-system
  labels:
    pod-security.kubernetes.io/enforce: privileged
    pod-security.kubernetes.io/audit: privileged
    pod-security.kubernetes.io/warn: privileged
`
}
