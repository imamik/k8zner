package addons

import (
	"context"
	"fmt"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyIngressNginx installs the NGINX Ingress Controller.
// See: terraform/ingress_nginx.tf
//
// Note: Admission webhooks are disabled in this configuration because:
// 1. Helm hooks (kube-webhook-certgen jobs) don't work with kubectl apply
// 2. Cert-manager integration has race conditions with certificate chain creation
// Admission webhooks are optional - they provide Ingress validation but the
// controller works fine without them.
func applyIngressNginx(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	// Create namespace first
	namespaceYAML := createIngressNginxNamespace()
	if err := applyManifests(ctx, client, "ingress-nginx-namespace", []byte(namespaceYAML)); err != nil {
		return fmt.Errorf("failed to create ingress-nginx namespace: %w", err)
	}

	// Build values matching terraform configuration
	values := buildIngressNginxValues(cfg)

	// Render helm chart
	manifestBytes, err := helm.RenderChart("ingress-nginx", "ingress-nginx", values)
	if err != nil {
		return fmt.Errorf("failed to render ingress-nginx chart: %w", err)
	}

	// Apply all manifests
	if err := applyManifests(ctx, client, "ingress-nginx", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply ingress-nginx manifests: %w", err)
	}

	return nil
}

// buildIngressNginxValues creates helm values matching terraform configuration.
// See: terraform/ingress_nginx.tf lines 39-129
func buildIngressNginxValues(cfg *config.Config) helm.Values {
	nginxCfg := cfg.Addons.IngressNginx
	workerCount := getWorkerCount(cfg)

	// Determine replicas
	replicas := 2
	if nginxCfg.Replicas != nil {
		replicas = *nginxCfg.Replicas
	} else if workerCount >= 3 {
		replicas = 3
	}

	// Determine kind (default: Deployment)
	kind := nginxCfg.Kind
	if kind == "" {
		kind = "Deployment"
	}

	controller := buildIngressNginxController(nginxCfg, workerCount, replicas, kind)

	values := helm.Values{
		"controller": controller,
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, nginxCfg.Helm.Values)
}

// buildIngressNginxController creates the controller configuration.
func buildIngressNginxController(nginxCfg config.IngressNginxConfig, workerCount, replicas int, kind string) helm.Values {
	// External traffic policy - default to "Local" (preserves client IP)
	externalTrafficPolicy := nginxCfg.ExternalTrafficPolicy
	if externalTrafficPolicy == "" {
		externalTrafficPolicy = "Local"
	}

	// Build config map values - start with defaults then merge user config
	configMap := helm.Values{
		"compute-full-forwarded-for": "true",
		"use-proxy-protocol":         "true",
	}
	for k, v := range nginxCfg.Config {
		configMap[k] = v
	}

	controller := helm.Values{
		// Disable admission webhooks for kubectl apply workflow.
		// The admission webhooks require either:
		// 1. Helm hooks (kube-webhook-certgen jobs) which don't work with kubectl apply
		// 2. Cert-manager integration which has race conditions with certificate chain creation
		// Admission webhooks are optional - they provide Ingress validation but the
		// controller works fine without them.
		"admissionWebhooks": helm.Values{
			"enabled": false,
		},
		"kind":                       kind,
		"replicaCount":               replicas,
		"minAvailable":               nil,
		"maxUnavailable":             1,
		"watchIngressWithoutClass":   true,
		"enableTopologyAwareRouting": nginxCfg.TopologyAwareRouting,
		"topologySpreadConstraints":  buildIngressNginxTopologySpread(workerCount),
		"metrics": helm.Values{
			"enabled": false,
		},
		"extraArgs": helm.Values{},
		"service": helm.Values{
			"type":                  "NodePort",
			"externalTrafficPolicy": externalTrafficPolicy,
			"nodePorts": helm.Values{
				"http":  30000,
				"https": 30001,
			},
		},
		"config": configMap,
		"networkPolicy": helm.Values{
			"enabled": true,
		},
		// Add tolerations for CCM uninitialized taint
		// This allows ingress-nginx to schedule before CCM has fully initialized nodes
		"tolerations": []helm.Values{
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
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
