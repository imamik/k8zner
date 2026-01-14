package addons

import (
	"context"
	"fmt"

	"hcloud-k8s/internal/addons/helm"
)

// applyCCM installs the Hetzner Cloud Controller Manager.
// See: terraform/hcloud.tf (hcloud_ccm)
func applyCCM(ctx context.Context, kubeconfigPath, token string, networkID int64) error {
	// Build CCM values matching terraform configuration
	values := buildCCMValues(token, networkID)

	// Render helm chart with values
	manifestBytes, err := helm.RenderChart("hcloud-ccm", "kube-system", values)
	if err != nil {
		return fmt.Errorf("failed to render CCM chart: %w", err)
	}

	// Apply manifests to cluster
	if err := applyWithKubectl(ctx, kubeconfigPath, "hcloud-ccm", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply CCM manifests: %w", err)
	}

	return nil
}

// buildCCMValues creates helm values matching terraform configuration.
// See: terraform/hcloud.tf lines 31-57
func buildCCMValues(_ string, _ int64) helm.Values {
	return helm.Values{
		"kind": "DaemonSet",
		"nodeSelector": helm.Values{
			"node-role.kubernetes.io/control-plane": "",
		},
		"networking": helm.Values{
			"enabled": true,
			// Note: clusterCIDR left at default 10.244.0.0/16 (Flannel default)
			"network": helm.Values{
				"valueFrom": helm.Values{
					"secretKeyRef": helm.Values{
						"name": "hcloud",
						"key":  "network",
					},
				},
			},
		},
		// HCLOUD_TOKEN is already configured in values.yaml defaults
		// Load balancer support is enabled by default in the chart
	}
}
