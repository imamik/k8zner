package addons

import (
	"context"
	"fmt"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
)

// applyCertManager installs cert-manager for TLS certificate management.
// See: terraform/cert_manager.tf
func applyCertManager(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
	// Create namespace first
	namespaceYAML := createCertManagerNamespace()
	if err := applyWithKubectl(ctx, kubeconfigPath, "cert-manager-namespace", []byte(namespaceYAML)); err != nil {
		return fmt.Errorf("failed to create cert-manager namespace: %w", err)
	}

	// Build values matching terraform configuration
	values := buildCertManagerValues(cfg)

	// Render helm chart
	manifestBytes, err := helm.RenderChart("cert-manager", "cert-manager", values)
	if err != nil {
		return fmt.Errorf("failed to render cert-manager chart: %w", err)
	}

	// Apply manifests
	if err := applyWithKubectl(ctx, kubeconfigPath, "cert-manager", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply cert-manager manifests: %w", err)
	}

	return nil
}

// buildCertManagerValues creates helm values matching terraform configuration.
// See: terraform/cert_manager.tf lines 10-112
func buildCertManagerValues(cfg *config.Config) helm.Values {
	controlPlaneCount := getControlPlaneCount(cfg)
	replicas := 1
	if controlPlaneCount > 1 {
		replicas = 2
	}

	// Base configuration shared by all components
	baseConfig := helm.Values{
		"replicaCount": replicas,
		"podDisruptionBudget": helm.Values{
			"enabled":        true,
			"minAvailable":   nil,
			"maxUnavailable": 1,
		},
		"nodeSelector": helm.Values{
			"node-role.kubernetes.io/control-plane": "",
		},
		"tolerations": []helm.Values{
			{
				"key":      "node-role.kubernetes.io/control-plane",
				"effect":   "NoSchedule",
				"operator": "Exists",
			},
		},
	}

	// Build topology spread constraints for a component
	buildTopologySpread := func(component string) []helm.Values {
		return []helm.Values{
			{
				"topologyKey":       "kubernetes.io/hostname",
				"maxSkew":           1,
				"whenUnsatisfiable": "DoNotSchedule",
				"labelSelector": helm.Values{
					"matchLabels": helm.Values{
						"app.kubernetes.io/instance":  "cert-manager",
						"app.kubernetes.io/component": component,
					},
				},
				"matchLabelKeys": []string{"pod-template-hash"},
			},
		}
	}

	// Merge base config with topology spread for each component
	controllerConfig := helm.Merge(
		baseConfig,
		helm.Values{"topologySpreadConstraints": buildTopologySpread("controller")},
	)
	webhookConfig := helm.Merge(
		baseConfig,
		helm.Values{"topologySpreadConstraints": buildTopologySpread("webhook")},
	)
	cainjectorConfig := helm.Merge(
		baseConfig,
		helm.Values{"topologySpreadConstraints": buildTopologySpread("cainjector")},
	)

	return helm.Values{
		"crds": helm.Values{
			"enabled": true,
		},
		"startupapicheck": helm.Values{
			"enabled": false,
		},
		"config": helm.Values{
			"enableGatewayAPI": true,
			"featureGates": helm.Values{
				// Workaround for ingress-nginx bug: https://github.com/kubernetes/ingress-nginx/issues/11176
				"ACMEHTTP01IngressPathTypeExact": !cfg.Addons.IngressNginx.Enabled,
			},
		},
		// Apply configuration to all components
		"replicaCount":               controllerConfig["replicaCount"],
		"podDisruptionBudget":        controllerConfig["podDisruptionBudget"],
		"topologySpreadConstraints":  controllerConfig["topologySpreadConstraints"],
		"nodeSelector":               controllerConfig["nodeSelector"],
		"tolerations":                controllerConfig["tolerations"],
		"webhook":                    webhookConfig,
		"cainjector":                 cainjectorConfig,
	}
}

// createCertManagerNamespace returns the cert-manager namespace manifest.
func createCertManagerNamespace() string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: cert-manager
`
}
