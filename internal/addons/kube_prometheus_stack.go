package addons

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyKubePrometheusStack installs the kube-prometheus-stack, providing a complete
// monitoring solution including Prometheus, Grafana, Alertmanager, and various exporters.
//
// Features:
//   - Prometheus for metrics collection and alerting
//   - Grafana for visualization with pre-built dashboards
//   - Alertmanager for alert routing and notification
//   - Node Exporter for hardware/OS metrics
//   - Kube State Metrics for Kubernetes object metrics
//   - Pre-configured alerting rules
//
// See: https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack
func applyKubePrometheusStack(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	// Create namespace first
	namespaceYAML := createMonitoringNamespace()
	if err := applyManifests(ctx, client, "monitoring-namespace", []byte(namespaceYAML)); err != nil {
		return fmt.Errorf("failed to create monitoring namespace: %w", err)
	}

	// Get worker node external IPs for DNS targeting in hostNetwork mode
	var workerIPs []string
	if cfg.Addons.Traefik.HostNetwork != nil && *cfg.Addons.Traefik.HostNetwork {
		ips, err := client.GetWorkerExternalIPs(ctx)
		if err != nil {
			log.Printf("[KubePrometheusStack] Warning: failed to get worker IPs for DNS target: %v", err)
		} else if len(ips) > 0 {
			workerIPs = ips
			log.Printf("[KubePrometheusStack] Using worker IPs for DNS target: %v", workerIPs)
		}
	}

	// Build values based on configuration
	values := buildKubePrometheusStackValues(cfg, workerIPs)

	// Get chart spec with any config overrides
	spec := helm.GetChartSpec("kube-prometheus-stack", cfg.Addons.KubePrometheusStack.Helm)

	// Render helm chart
	manifestBytes, err := helm.RenderFromSpec(ctx, spec, "monitoring", values)
	if err != nil {
		return fmt.Errorf("failed to render kube-prometheus-stack chart: %w", err)
	}

	// Apply all manifests
	if err := applyManifests(ctx, client, "kube-prometheus-stack", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply kube-prometheus-stack manifests: %w", err)
	}

	log.Printf("[KubePrometheusStack] Monitoring stack installed successfully")
	return nil
}

// buildKubePrometheusStackValues creates helm values for kube-prometheus-stack configuration.
func buildKubePrometheusStackValues(cfg *config.Config, workerIPs []string) helm.Values {
	promCfg := cfg.Addons.KubePrometheusStack

	values := helm.Values{
		// Default alerting rules
		"defaultRules": helm.Values{
			"create": getBoolWithDefault(promCfg.DefaultRules, true),
		},
		// Prometheus configuration
		"prometheus": buildPrometheusValues(cfg, workerIPs),
		// Grafana configuration
		"grafana": buildGrafanaValues(cfg, workerIPs),
		// Alertmanager configuration
		"alertmanager": buildAlertmanagerValues(cfg, workerIPs),
		// Node Exporter
		"nodeExporter": helm.Values{
			"enabled": getBoolWithDefault(promCfg.NodeExporter, true),
		},
		// Kube State Metrics
		"kubeStateMetrics": helm.Values{
			"enabled": getBoolWithDefault(promCfg.KubeStateMetrics, true),
		},
		// Prometheus Operator configuration
		"prometheusOperator": buildPrometheusOperatorValues(),
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, promCfg.Helm.Values)
}

// buildPrometheusValues creates the Prometheus server configuration.
func buildPrometheusValues(cfg *config.Config, workerIPs []string) helm.Values {
	promCfg := cfg.Addons.KubePrometheusStack.Prometheus

	values := helm.Values{
		"enabled": getBoolWithDefault(promCfg.Enabled, true),
		"prometheusSpec": helm.Values{
			// Retention period
			"retention": fmt.Sprintf("%dd", getIntWithDefault(promCfg.RetentionDays, 15)),
			// Add tolerations for CCM uninitialized taint
			"tolerations": []helm.Values{
				{
					"key":      "node.cloudprovider.kubernetes.io/uninitialized",
					"operator": "Exists",
				},
			},
			// Resource defaults
			"resources": buildResourceValues(promCfg.Resources, "500m", "512Mi", "2", "2Gi"),
			// Service monitor selector - scrape all ServiceMonitors
			"serviceMonitorSelectorNilUsesHelmValues": false,
			"podMonitorSelectorNilUsesHelmValues":     false,
			"ruleSelectorNilUsesHelmValues":           false,
		},
	}

	// Storage configuration
	if promCfg.Persistence.Enabled {
		values["prometheusSpec"].(helm.Values)["storageSpec"] = helm.Values{
			"volumeClaimTemplate": helm.Values{
				"spec": helm.Values{
					"accessModes": []string{"ReadWriteOnce"},
					"resources": helm.Values{
						"requests": helm.Values{
							"storage": getStringWithDefault(promCfg.Persistence.Size, "50Gi"),
						},
					},
					"storageClassName": promCfg.Persistence.StorageClass,
				},
			},
		}
	}

	// Ingress configuration
	if promCfg.IngressEnabled && promCfg.IngressHost != "" {
		values["ingress"] = buildPrometheusIngress(cfg, workerIPs)
	}

	return values
}

// buildPrometheusIngress creates the ingress configuration for Prometheus.
func buildPrometheusIngress(cfg *config.Config, workerIPs []string) helm.Values {
	promCfg := cfg.Addons.KubePrometheusStack.Prometheus

	ingress := helm.Values{
		"enabled": true,
		"hosts": []string{
			promCfg.IngressHost,
		},
		"paths": []string{"/"},
	}

	// Set ingress class
	ingressClass := promCfg.IngressClassName
	if ingressClass == "" {
		ingressClass = "traefik"
	}
	ingress["ingressClassName"] = ingressClass

	// Configure TLS if enabled
	if promCfg.IngressTLS {
		ingress["tls"] = []helm.Values{
			{
				"hosts":      []string{promCfg.IngressHost},
				"secretName": "prometheus-tls",
			},
		}

		// Build annotations for TLS and DNS
		annotations := buildIngressAnnotations(cfg, promCfg.IngressHost, workerIPs)
		ingress["annotations"] = annotations
	}

	return ingress
}

// buildGrafanaValues creates the Grafana configuration.
func buildGrafanaValues(cfg *config.Config, workerIPs []string) helm.Values {
	grafanaCfg := cfg.Addons.KubePrometheusStack.Grafana

	values := helm.Values{
		"enabled": getBoolWithDefault(grafanaCfg.Enabled, true),
		// Run Grafana in insecure mode - Traefik handles TLS termination
		"grafana.ini": helm.Values{
			"server": helm.Values{
				"root_url":            fmt.Sprintf("https://%s", grafanaCfg.IngressHost),
				"serve_from_sub_path": false,
			},
		},
		// Disable test framework to avoid service account race conditions
		"testFramework": helm.Values{
			"enabled": false,
		},
		// Add tolerations for CCM uninitialized taint
		"tolerations": []helm.Values{
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
		},
		// Resource defaults
		"resources": helm.Values{
			"requests": helm.Values{
				"cpu":    "100m",
				"memory": "128Mi",
			},
			"limits": helm.Values{
				"memory": "256Mi",
			},
		},
		// Default data sources
		"sidecar": helm.Values{
			"dashboards": helm.Values{
				"enabled": true,
			},
			"datasources": helm.Values{
				"enabled": true,
			},
		},
	}

	// Admin password
	if grafanaCfg.AdminPassword != "" {
		values["adminPassword"] = grafanaCfg.AdminPassword
	}

	// Persistence configuration
	if grafanaCfg.Persistence.Enabled {
		values["persistence"] = helm.Values{
			"enabled":          true,
			"size":             getStringWithDefault(grafanaCfg.Persistence.Size, "10Gi"),
			"storageClassName": grafanaCfg.Persistence.StorageClass,
		}
	}

	// Ingress configuration
	if grafanaCfg.IngressEnabled && grafanaCfg.IngressHost != "" {
		values["ingress"] = buildGrafanaIngress(cfg, workerIPs)
	}

	return values
}

// buildGrafanaIngress creates the ingress configuration for Grafana.
func buildGrafanaIngress(cfg *config.Config, workerIPs []string) helm.Values {
	grafanaCfg := cfg.Addons.KubePrometheusStack.Grafana

	ingress := helm.Values{
		"enabled": true,
		"hosts": []string{
			grafanaCfg.IngressHost,
		},
		"path": "/",
	}

	// Set ingress class
	ingressClass := grafanaCfg.IngressClassName
	if ingressClass == "" {
		ingressClass = "traefik"
	}
	ingress["ingressClassName"] = ingressClass

	// Configure TLS if enabled
	if grafanaCfg.IngressTLS {
		ingress["tls"] = []helm.Values{
			{
				"hosts":      []string{grafanaCfg.IngressHost},
				"secretName": "grafana-tls",
			},
		}

		// Build annotations for TLS and DNS
		annotations := buildIngressAnnotations(cfg, grafanaCfg.IngressHost, workerIPs)
		ingress["annotations"] = annotations
	}

	return ingress
}

// buildAlertmanagerValues creates the Alertmanager configuration.
func buildAlertmanagerValues(cfg *config.Config, workerIPs []string) helm.Values {
	alertCfg := cfg.Addons.KubePrometheusStack.Alertmanager

	values := helm.Values{
		"enabled": getBoolWithDefault(alertCfg.Enabled, true),
		"alertmanagerSpec": helm.Values{
			// Add tolerations for CCM uninitialized taint
			"tolerations": []helm.Values{
				{
					"key":      "node.cloudprovider.kubernetes.io/uninitialized",
					"operator": "Exists",
				},
			},
			// Resource defaults
			"resources": helm.Values{
				"requests": helm.Values{
					"cpu":    "50m",
					"memory": "64Mi",
				},
				"limits": helm.Values{
					"memory": "128Mi",
				},
			},
		},
	}

	// Ingress configuration
	if alertCfg.IngressEnabled && alertCfg.IngressHost != "" {
		values["ingress"] = buildAlertmanagerIngress(cfg, workerIPs)
	}

	return values
}

// buildAlertmanagerIngress creates the ingress configuration for Alertmanager.
func buildAlertmanagerIngress(cfg *config.Config, workerIPs []string) helm.Values {
	alertCfg := cfg.Addons.KubePrometheusStack.Alertmanager

	ingress := helm.Values{
		"enabled": true,
		"hosts": []string{
			alertCfg.IngressHost,
		},
		"paths": []string{"/"},
	}

	// Set ingress class
	ingressClass := alertCfg.IngressClassName
	if ingressClass == "" {
		ingressClass = "traefik"
	}
	ingress["ingressClassName"] = ingressClass

	// Configure TLS if enabled
	if alertCfg.IngressTLS {
		ingress["tls"] = []helm.Values{
			{
				"hosts":      []string{alertCfg.IngressHost},
				"secretName": "alertmanager-tls",
			},
		}

		// Build annotations for TLS and DNS
		annotations := buildIngressAnnotations(cfg, alertCfg.IngressHost, workerIPs)
		ingress["annotations"] = annotations
	}

	return ingress
}

// buildPrometheusOperatorValues creates the Prometheus Operator configuration.
func buildPrometheusOperatorValues() helm.Values {
	return helm.Values{
		// Add tolerations for CCM uninitialized taint
		"tolerations": []helm.Values{
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
		},
		// Resource defaults
		"resources": helm.Values{
			"requests": helm.Values{
				"cpu":    "100m",
				"memory": "128Mi",
			},
			"limits": helm.Values{
				"memory": "256Mi",
			},
		},
	}
}

// buildIngressAnnotations builds common ingress annotations for TLS and DNS.
func buildIngressAnnotations(cfg *config.Config, host string, workerIPs []string) helm.Values {
	annotations := helm.Values{}

	// Determine which ClusterIssuer to use based on cert-manager Cloudflare config
	clusterIssuer := "letsencrypt-prod" // Default fallback
	if cfg.Addons.CertManager.Cloudflare.Enabled {
		if cfg.Addons.CertManager.Cloudflare.Production {
			clusterIssuer = "letsencrypt-cloudflare-production"
		} else {
			clusterIssuer = "letsencrypt-cloudflare-staging"
		}
	}
	annotations["cert-manager.io/cluster-issuer"] = clusterIssuer

	// Add external-dns annotations if Cloudflare/external-dns is enabled
	if cfg.Addons.Cloudflare.Enabled && cfg.Addons.ExternalDNS.Enabled {
		annotations["external-dns.alpha.kubernetes.io/hostname"] = host

		// When using hostNetwork mode, external-dns can't determine the target IP
		// from the Ingress status. We need to provide it explicitly via annotation.
		if len(workerIPs) > 0 {
			annotations["external-dns.alpha.kubernetes.io/target"] = strings.Join(workerIPs, ",")
		}
	}

	return annotations
}

// buildResourceValues creates resource specifications with defaults.
func buildResourceValues(resources config.KubePrometheusResourcesConfig, defaultReqCPU, defaultReqMem, defaultLimitCPU, defaultLimitMem string) helm.Values {
	reqCPU := resources.Requests.CPU
	if reqCPU == "" {
		reqCPU = defaultReqCPU
	}
	reqMem := resources.Requests.Memory
	if reqMem == "" {
		reqMem = defaultReqMem
	}
	limitCPU := resources.Limits.CPU
	if limitCPU == "" {
		limitCPU = defaultLimitCPU
	}
	limitMem := resources.Limits.Memory
	if limitMem == "" {
		limitMem = defaultLimitMem
	}

	return helm.Values{
		"requests": helm.Values{
			"cpu":    reqCPU,
			"memory": reqMem,
		},
		"limits": helm.Values{
			"cpu":    limitCPU,
			"memory": limitMem,
		},
	}
}

// createMonitoringNamespace returns the monitoring namespace manifest.
func createMonitoringNamespace() string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: monitoring
  labels:
    name: monitoring
`
}

// Helper functions

func getBoolWithDefault(ptr *bool, defaultVal bool) bool {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}

func getIntWithDefault(ptr *int, defaultVal int) int {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}

func getStringWithDefault(s string, defaultVal string) string {
	if s == "" {
		return defaultVal
	}
	return s
}
