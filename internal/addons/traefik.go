package addons

import (
	"context"
	"fmt"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/util/naming"
)

// applyTraefik installs the Traefik Proxy ingress controller.
// Traefik uses a Deployment with a LoadBalancer service. CCM creates
// a Hetzner LB automatically via annotations, and external-dns
// auto-discovers the LB IP from the Ingress status for DNS records.
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
// Always uses Deployment with LoadBalancer service. CCM creates a Hetzner LB
// via annotations on the Service object.
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

	// External traffic policy - default to "Cluster" for reliability.
	// "Local" preserves client IP but introduces health check node port complexity
	// with Hetzner LBs. "Cluster" is what kube-hetzner and other projects use.
	externalTrafficPolicy := traefikCfg.ExternalTrafficPolicy
	if externalTrafficPolicy == "" {
		externalTrafficPolicy = "Cluster"
	}

	// Ingress class name
	ingressClassName := traefikCfg.IngressClass
	if ingressClassName == "" {
		ingressClassName = "traefik"
	}

	// Determine location for load balancer
	location := cfg.Location
	if location == "" {
		location = "nbg1"
	}

	values := helm.Values{
		"deployment":   deployment,
		"ingressClass": buildTraefikIngressClass(ingressClassName),
		"ingressRoute": buildTraefikIngressRoute(),
		"providers":    buildTraefikProviders(),
		"ports":        buildTraefikPorts(),
		"service":      buildTraefikService(cfg.ClusterName, externalTrafficPolicy, location),
		"tolerations": []helm.Values{helm.CCMUninitializedToleration()},
		"topologySpreadConstraints": func() []helm.Values {
			hostnamePolicy := "ScheduleAnyway"
			if workerCount > 1 {
				hostnamePolicy = "DoNotSchedule"
			}
			return helm.TopologySpread("traefik", "traefik", hostnamePolicy)
		}(),
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
// Disabled because we use standard Kubernetes Ingress, not Traefik's CRD-based IngressRoute.
func buildTraefikIngressRoute() helm.Values {
	return helm.Values{
		"dashboard": helm.Values{
			"enabled": false,
		},
	}
}

// buildTraefikProviders creates the providers configuration.
// Only kubernetesIngress is enabled - we use standard Kubernetes Ingress resources,
// not Traefik's IngressRoute CRDs. The kubernetesIngress provider discovers TLS
// secrets referenced in Ingress spec.tls[].secretName (created by cert-manager).
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
// Uses standard container ports (8000/8443) mapped to service ports (80/443).
// TLS is explicitly enabled on websecure so Traefik terminates TLS using certs
// from Kubernetes secrets (created by cert-manager via Ingress annotations).
// Note: Traefik chart v39+ uses new schema where 'expose' is an object with 'default' key.
func buildTraefikPorts() helm.Values {
	return helm.Values{
		"web": helm.Values{
			"port":        8000,
			"expose":      helm.Values{"default": true},
			"exposedPort": 80,
			"protocol":    "TCP",
		},
		"websecure": helm.Values{
			"port":        8443,
			"expose":      helm.Values{"default": true},
			"exposedPort": 443,
			"protocol":    "TCP",
			"http": helm.Values{
				"tls": helm.Values{
					"enabled": true,
				},
			},
		},
		"traefik": helm.Values{
			"port":   9000,
			"expose": helm.Values{"default": false},
		},
	}
}

// buildTraefikService creates the service configuration.
// Always uses LoadBalancer with Hetzner annotations. CCM creates the LB automatically.
func buildTraefikService(clusterName, externalTrafficPolicy, location string) helm.Values {
	// Use proper naming convention for the load balancer
	lbName := naming.IngressLoadBalancer(clusterName)

	return helm.Values{
		"enabled": true,
		"type":    "LoadBalancer",
		"spec": helm.Values{
			"externalTrafficPolicy": externalTrafficPolicy,
		},
		// Hetzner LB annotations - CCM creates the LB automatically
		"annotations": helm.Values{
			"load-balancer.hetzner.cloud/name":                    lbName,
			"load-balancer.hetzner.cloud/use-private-ip":          "true",
			"load-balancer.hetzner.cloud/disable-private-ingress": "true",
			"load-balancer.hetzner.cloud/location":                location,
		},
	}
}

// createTraefikNamespace returns the traefik namespace manifest.
func createTraefikNamespace() string {
	return helm.NamespaceManifest("traefik", map[string]string{
		"pod-security.kubernetes.io/enforce": "baseline",
		"pod-security.kubernetes.io/audit":   "baseline",
		"pod-security.kubernetes.io/warn":    "baseline",
	})
}
