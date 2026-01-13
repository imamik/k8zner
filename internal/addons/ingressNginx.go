package addons

import (
	"context"
	"fmt"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
)

// applyIngressNginx installs the NGINX Ingress Controller.
// See: terraform/ingress_nginx.tf
func applyIngressNginx(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
	// Create namespace first
	namespaceYAML := createIngressNginxNamespace()
	if err := applyWithKubectl(ctx, kubeconfigPath, "ingress-nginx-namespace", []byte(namespaceYAML)); err != nil {
		return fmt.Errorf("failed to create ingress-nginx namespace: %w", err)
	}

	// Build values matching terraform configuration
	values := buildIngressNginxValues(cfg)

	// Render helm chart
	manifestBytes, err := helm.RenderChart("ingress-nginx", "ingress-nginx", values)
	if err != nil {
		return fmt.Errorf("failed to render ingress-nginx chart: %w", err)
	}

	// Apply manifests
	if err := applyWithKubectl(ctx, kubeconfigPath, "ingress-nginx", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply ingress-nginx manifests: %w", err)
	}

	return nil
}

// buildIngressNginxValues creates helm values matching terraform configuration.
// See: terraform/ingress_nginx.tf lines 39-129
func buildIngressNginxValues(cfg *config.Config) helm.Values {
	workerCount := getWorkerCount(cfg)
	replicas := 2
	if workerCount >= 3 {
		replicas = 3
	}

	controller := buildIngressNginxController(workerCount, replicas)

	return helm.Values{
		"controller": controller,
	}
}

// buildIngressNginxController creates the controller configuration.
func buildIngressNginxController(workerCount, replicas int) helm.Values {
	controller := helm.Values{
		"admissionWebhooks": helm.Values{
			"certManager": helm.Values{
				"enabled": true,
			},
		},
		"kind":                       "Deployment",
		"replicaCount":               replicas,
		"minAvailable":               nil,
		"maxUnavailable":             1,
		"watchIngressWithoutClass":   true,
		"enableTopologyAwareRouting": false,
		"topologySpreadConstraints":  buildIngressNginxTopologySpread(workerCount),
		"metrics": helm.Values{
			"enabled": false,
		},
		"extraArgs": helm.Values{},
		"service": helm.Values{
			"type":                  "NodePort",
			"externalTrafficPolicy": "Local",
			"nodePorts": helm.Values{
				"http":  30000,
				"https": 30001,
			},
		},
		"config": helm.Values{
			"compute-full-forwarded-for": true,
			"use-proxy-protocol":         true,
		},
		"networkPolicy": helm.Values{
			"enabled": true,
		},
	}

	return controller
}

// buildIngressNginxTopologySpread creates topology spread constraints for ingress-nginx.
// Two constraints: hostname (strict if multiple workers) and zone (soft).
func buildIngressNginxTopologySpread(workerCount int) []helm.Values {
	// Determine whenUnsatisfiable for hostname constraint
	hostnameUnsatisfiable := "ScheduleAnyway"
	if workerCount > 1 {
		hostnameUnsatisfiable = "DoNotSchedule"
	}

	labelSelector := helm.Values{
		"matchLabels": helm.Values{
			"app.kubernetes.io/instance":  "ingress-nginx",
			"app.kubernetes.io/name":      "ingress-nginx",
			"app.kubernetes.io/component": "controller",
		},
	}

	return []helm.Values{
		{
			"topologyKey":       "kubernetes.io/hostname",
			"maxSkew":           1,
			"whenUnsatisfiable": hostnameUnsatisfiable,
			"labelSelector":     labelSelector,
			"matchLabelKeys":    []string{"pod-template-hash"},
		},
		{
			"topologyKey":       "topology.kubernetes.io/zone",
			"maxSkew":           1,
			"whenUnsatisfiable": "ScheduleAnyway",
			"labelSelector":     labelSelector,
			"matchLabelKeys":    []string{"pod-template-hash"},
		},
	}
}

// createIngressNginxNamespace returns the ingress-nginx namespace manifest.
func createIngressNginxNamespace() string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: ingress-nginx
`
}
