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
// Traefik uses LoadBalancer service type for Kubernetes-native external IP discovery,
// which allows external-dns to auto-discover the IP for DNS records.
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
// When hostNetwork is enabled (dev mode): uses DaemonSet with direct host port binding.
// When hostNetwork is disabled (HA mode): uses LoadBalancer service with Hetzner LB.
func buildTraefikValues(cfg *config.Config) helm.Values {
	traefikCfg := cfg.Addons.Traefik
	workerCount := getWorkerCount(cfg)

	// Check if hostNetwork mode is enabled (dev mode)
	hostNetwork := traefikCfg.HostNetwork != nil && *traefikCfg.HostNetwork

	// Determine replicas (not used for DaemonSet)
	replicas := 2
	if traefikCfg.Replicas != nil {
		replicas = *traefikCfg.Replicas
	} else if workerCount >= 3 {
		replicas = 3
	}

	// Determine kind (default: Deployment, or DaemonSet for hostNetwork)
	kind := traefikCfg.Kind
	if kind == "" {
		if hostNetwork {
			kind = "DaemonSet"
		} else {
			kind = "Deployment"
		}
	}

	// Build the deployment configuration
	deployment := buildTraefikDeployment(replicas, kind, hostNetwork)

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
		"ports":        buildTraefikPorts(hostNetwork),
		"service":      buildTraefikService(cfg.ClusterName, externalTrafficPolicy, location, hostNetwork),
		// Add tolerations for CCM uninitialized taint
		// This allows Traefik to schedule before CCM has fully initialized nodes
		"tolerations": []helm.Values{
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
		},
	}

	// Add proxy protocol args only for LoadBalancer mode (not hostNetwork)
	if !hostNetwork {
		values["additionalArguments"] = []string{
			// Enable proxy protocol for proper client IP preservation with Hetzner LBs
			"--entryPoints.web.proxyProtocol.trustedIPs=127.0.0.1/32,10.0.0.0/8",
			"--entryPoints.websecure.proxyProtocol.trustedIPs=127.0.0.1/32,10.0.0.0/8",
		}
		// Topology spread constraints for HA (not relevant for DaemonSet)
		values["topologySpreadConstraints"] = buildTraefikTopologySpread(workerCount)
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, traefikCfg.Helm.Values)
}

// buildTraefikDeployment creates the deployment configuration.
func buildTraefikDeployment(replicas int, kind string, hostNetwork bool) helm.Values {
	deployment := helm.Values{
		"enabled":  true,
		"kind":     kind,
		"replicas": replicas,
		"podDisruptionBudget": helm.Values{
			"enabled":        true,
			"maxUnavailable": 1,
		},
	}

	// Enable hostNetwork mode for direct port binding (dev mode)
	if hostNetwork {
		deployment["hostNetwork"] = true
		// When using hostNetwork, set dnsPolicy to ClusterFirstWithHostNet
		deployment["dnsPolicy"] = "ClusterFirstWithHostNet"
	}

	return deployment
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
// When hostNetwork is true: uses hostPort for direct binding to 80/443.
// When hostNetwork is false: uses standard service ports with proxy protocol.
// Note: Traefik chart v39+ uses new schema where 'expose' is an object with 'default' key.
func buildTraefikPorts(hostNetwork bool) helm.Values {
	webPort := helm.Values{
		"port":        8000,
		"expose":      helm.Values{"default": true},
		"exposedPort": 80,
		"protocol":    "TCP",
	}

	websecurePort := helm.Values{
		"port":        8443,
		"expose":      helm.Values{"default": true},
		"exposedPort": 443,
		"protocol":    "TCP",
	}

	if hostNetwork {
		// In hostNetwork mode, bind directly to host ports
		webPort["hostPort"] = 80
		websecurePort["hostPort"] = 443
	} else {
		// In LoadBalancer mode, enable proxy protocol for client IP preservation
		webPort["proxyProtocol"] = helm.Values{"enabled": true}
		websecurePort["proxyProtocol"] = helm.Values{"enabled": true}
	}

	return helm.Values{
		"web":       webPort,
		"websecure": websecurePort,
		"traefik": helm.Values{
			"port":   9000,
			"expose": helm.Values{"default": false},
		},
	}
}

// buildTraefikService creates the service configuration.
// When hostNetwork is true: uses ClusterIP (no external LB needed).
// When hostNetwork is false: uses LoadBalancer with Hetzner annotations.
func buildTraefikService(clusterName, externalTrafficPolicy, location string, hostNetwork bool) helm.Values {
	if hostNetwork {
		// In hostNetwork mode, use ClusterIP - traffic goes directly to host ports
		return helm.Values{
			"enabled": true,
			"type":    "ClusterIP",
		}
	}

	// Use proper naming convention for the load balancer
	lbName := naming.IngressLoadBalancer(clusterName)

	// In LoadBalancer mode, create Hetzner LB with proxy protocol
	return helm.Values{
		"enabled": true,
		"type":    "LoadBalancer",
		"spec": helm.Values{
			"externalTrafficPolicy": externalTrafficPolicy,
		},
		// Hetzner LB annotations for proxy protocol support
		"annotations": helm.Values{
			"load-balancer.hetzner.cloud/name":               lbName,
			"load-balancer.hetzner.cloud/use-private-ip":     "true",
			"load-balancer.hetzner.cloud/uses-proxyprotocol": "true",
			"load-balancer.hetzner.cloud/location":           location,
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
