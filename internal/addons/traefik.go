package addons

import (
	"context"
	"fmt"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyTraefik installs the Traefik Proxy ingress controller.
// Traefik is configured to use NodePort services on ports 30000/30001
// for compatibility with Hetzner Load Balancers, matching the ingress-nginx setup.
func applyTraefik(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	// Create namespace first
	namespaceYAML := createTraefikNamespace()
	if err := applyManifests(ctx, client, "traefik-namespace", []byte(namespaceYAML)); err != nil {
		return fmt.Errorf("failed to create traefik namespace: %w", err)
	}

	// Build values matching the ingress-nginx configuration style
	values := buildTraefikValues(cfg)

	// Get chart spec with any config overrides
	spec := helm.GetChartSpec("traefik", cfg.Addons.Traefik.Helm)

	// Render helm chart
	manifestBytes, err := helm.RenderFromSpec(ctx, spec, "traefik", values)
	if err != nil {
		return fmt.Errorf("failed to render traefik chart: %w", err)
	}

	// Apply all manifests
	if err := applyManifests(ctx, client, "traefik", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply traefik manifests: %w", err)
	}

	return nil
}

// buildTraefikValues creates helm values for Traefik configuration.
// This mirrors the ingress-nginx setup for consistency with Hetzner Load Balancers.
func buildTraefikValues(cfg *config.Config) helm.Values {
	traefikCfg := cfg.Addons.Traefik
	workerCount := getWorkerCount(cfg)

	// Determine replicas
	replicas := 2
	if traefikCfg.Replicas != nil {
		replicas = *traefikCfg.Replicas
	} else if workerCount >= 3 {
		replicas = 3
	}

	// Determine kind (default: Deployment)
	kind := traefikCfg.Kind
	if kind == "" {
		kind = "Deployment"
	}

	// Build the deployment configuration
	deployment := buildTraefikDeployment(replicas, kind)

	// External traffic policy - default to "Local" (preserves client IP)
	externalTrafficPolicy := traefikCfg.ExternalTrafficPolicy
	if externalTrafficPolicy == "" {
		externalTrafficPolicy = "Local"
	}

	// Ingress class name
	ingressClassName := traefikCfg.IngressClass
	if ingressClassName == "" {
		ingressClassName = "traefik"
	}

	values := helm.Values{
		"deployment":   deployment,
		"ingressClass": buildTraefikIngressClass(ingressClassName),
		"ingressRoute": buildTraefikIngressRoute(),
		"providers":    buildTraefikProviders(),
		"ports":        buildTraefikPorts(externalTrafficPolicy),
		"service":      buildTraefikService(externalTrafficPolicy),
		"additionalArguments": []string{
			// Enable proxy protocol for proper client IP preservation with Hetzner LBs
			"--entryPoints.web.proxyProtocol.trustedIPs=127.0.0.1/32,10.0.0.0/8",
			"--entryPoints.websecure.proxyProtocol.trustedIPs=127.0.0.1/32,10.0.0.0/8",
		},
		// Add tolerations for CCM uninitialized taint
		// This allows Traefik to schedule before CCM has fully initialized nodes
		"tolerations": []helm.Values{
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
		},
		// Topology spread constraints for HA
		"topologySpreadConstraints": buildTraefikTopologySpread(workerCount),
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, traefikCfg.Helm.Values)
}

// buildTraefikDeployment creates the deployment configuration.
func buildTraefikDeployment(replicas int, kind string) helm.Values {
	return helm.Values{
		"enabled":  true,
		"kind":     kind,
		"replicas": replicas,
		"podDisruptionBudget": helm.Values{
			"enabled":        true,
			"maxUnavailable": 1,
		},
	}
}

// buildTraefikIngressClass creates the IngressClass configuration.
func buildTraefikIngressClass(name string) helm.Values {
	return helm.Values{
		"enabled":        true,
		"isDefaultClass": true,
		"name":           name,
	}
}

// buildTraefikIngressRoute creates the IngressRoute configuration for the dashboard.
// IngressRoute requires Traefik CRDs which we don't install, so always disabled.
func buildTraefikIngressRoute() helm.Values {
	return helm.Values{
		"dashboard": helm.Values{
			"enabled": false,
		},
	}
}

// buildTraefikProviders creates the providers configuration.
// We disable kubernetesCRD and only use standard Kubernetes Ingress resources
// to avoid requiring Traefik CRDs installation.
func buildTraefikProviders() helm.Values {
	return helm.Values{
		"kubernetesCRD": helm.Values{
			"enabled": false,
		},
		"kubernetesIngress": helm.Values{
			"enabled":                   true,
			"allowExternalNameServices": true,
			"publishedService": helm.Values{
				"enabled": true,
			},
		},
	}
}

// buildTraefikPorts creates the ports configuration.
// Uses NodePort on 30000/30001 for Hetzner LB compatibility (same as nginx).
func buildTraefikPorts(externalTrafficPolicy string) helm.Values {
	return helm.Values{
		"web": helm.Values{
			"port":        8000,
			"expose":      true,
			"exposedPort": 80,
			"nodePort":    30000,
			"protocol":    "TCP",
			// Enable proxy protocol for client IP preservation
			"proxyProtocol": helm.Values{
				"enabled": true,
			},
		},
		"websecure": helm.Values{
			"port":        8443,
			"expose":      true,
			"exposedPort": 443,
			"nodePort":    30001,
			"protocol":    "TCP",
			// Enable proxy protocol for client IP preservation
			"proxyProtocol": helm.Values{
				"enabled": true,
			},
			// TLS configuration
			"tls": helm.Values{
				"enabled": true,
			},
		},
		"traefik": helm.Values{
			"port":   9000,
			"expose": false,
		},
	}
}

// buildTraefikService creates the service configuration.
func buildTraefikService(externalTrafficPolicy string) helm.Values {
	return helm.Values{
		"enabled": true,
		"type":    "NodePort",
		"spec": helm.Values{
			"externalTrafficPolicy": externalTrafficPolicy,
		},
	}
}

// buildTraefikTopologySpread creates topology spread constraints for Traefik.
// Two constraints: hostname (strict if multiple workers) and zone (soft).
func buildTraefikTopologySpread(workerCount int) []helm.Values {
	// Determine whenUnsatisfiable for hostname constraint
	hostnameUnsatisfiable := "ScheduleAnyway"
	if workerCount > 1 {
		hostnameUnsatisfiable = "DoNotSchedule"
	}

	labelSelector := helm.Values{
		"matchLabels": helm.Values{
			"app.kubernetes.io/instance": "traefik",
			"app.kubernetes.io/name":     "traefik",
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

// createTraefikNamespace returns the traefik namespace manifest.
func createTraefikNamespace() string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: traefik
`
}
