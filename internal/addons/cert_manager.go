package addons

import (
	"context"
	"fmt"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyCertManager installs cert-manager for TLS certificate management.
// See: terraform/cert_manager.tf
func applyCertManager(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	// Create namespace first
	namespaceYAML := createCertManagerNamespace()
	if err := applyManifests(ctx, client, "cert-manager-namespace", []byte(namespaceYAML)); err != nil {
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
	if err := applyManifests(ctx, client, "cert-manager", manifestBytes); err != nil {
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

	baseConfig := buildCertManagerBaseConfig(replicas)

	values := helm.Values{
		"crds":                      helm.Values{"enabled": true},
		"startupapicheck":           helm.Values{"enabled": false},
		"config":                    buildCertManagerConfig(cfg),
		"replicaCount":              baseConfig["replicaCount"],
		"podDisruptionBudget":       baseConfig["podDisruptionBudget"],
		"topologySpreadConstraints": buildCertManagerTopologySpread("controller"),
		"nodeSelector":              baseConfig["nodeSelector"],
		"tolerations":               baseConfig["tolerations"],
		"webhook": helm.Values{
			"replicaCount":              baseConfig["replicaCount"],
			"podDisruptionBudget":       baseConfig["podDisruptionBudget"],
			"topologySpreadConstraints": buildCertManagerTopologySpread("webhook"),
			"nodeSelector":              baseConfig["nodeSelector"],
			"tolerations":               baseConfig["tolerations"],
		},
		"cainjector": helm.Values{
			"replicaCount":              baseConfig["replicaCount"],
			"podDisruptionBudget":       baseConfig["podDisruptionBudget"],
			"topologySpreadConstraints": buildCertManagerTopologySpread("cainjector"),
			"nodeSelector":              baseConfig["nodeSelector"],
			"tolerations":               baseConfig["tolerations"],
		},
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, cfg.Addons.CertManager.Helm.Values)
}

// buildCertManagerBaseConfig creates the base configuration shared by all components.
func buildCertManagerBaseConfig(replicas int) helm.Values {
	return helm.Values{
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
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
		},
	}
}

// buildCertManagerTopologySpread creates topology spread constraints for a component.
func buildCertManagerTopologySpread(component string) []helm.Values {
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

// buildCertManagerConfig creates the config section with feature gates.
func buildCertManagerConfig(cfg *config.Config) helm.Values {
	return helm.Values{
		"enableGatewayAPI": true,
		"featureGates": helm.Values{
			// Workaround for ingress-nginx bug: https://github.com/kubernetes/ingress-nginx/issues/11176
			"ACMEHTTP01IngressPathTypeExact": !cfg.Addons.IngressNginx.Enabled,
		},
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
