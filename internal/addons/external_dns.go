package addons

import (
	"context"
	"fmt"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyExternalDNS installs external-dns for automatic DNS record management.
// External-dns watches Kubernetes Ingress resources and creates DNS records
// in Cloudflare based on annotations.
func applyExternalDNS(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	// Build values for external-dns
	values := buildExternalDNSValues(cfg)

	// Get chart spec with any config overrides
	spec := helm.GetChartSpec("external-dns", cfg.Addons.ExternalDNS.Helm)

	// Render helm chart
	manifestBytes, err := helm.RenderFromSpec(ctx, spec, "external-dns", values)
	if err != nil {
		return fmt.Errorf("failed to render external-dns chart: %w", err)
	}

	// Apply manifests
	if err := applyManifests(ctx, client, "external-dns", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply external-dns manifests: %w", err)
	}

	return nil
}

// buildExternalDNSValues creates helm values for external-dns configuration.
func buildExternalDNSValues(cfg *config.Config) helm.Values {
	extDNSCfg := cfg.Addons.ExternalDNS
	cfCfg := cfg.Addons.Cloudflare

	// Determine TXT owner ID (default to cluster name)
	ownerID := extDNSCfg.TXTOwnerID
	if ownerID == "" {
		ownerID = cfg.ClusterName
	}

	// Determine sync policy
	policy := extDNSCfg.Policy
	if policy == "" {
		policy = "sync"
	}

	// Determine sources to watch
	sources := extDNSCfg.Sources
	if len(sources) == 0 {
		sources = []string{"ingress"}
	}

	// Build domain filters if domain is configured
	domainFilters := []string{}
	if cfCfg.Domain != "" {
		domainFilters = []string{cfCfg.Domain}
	}

	// Build extra args for Cloudflare-specific settings
	// Note: --cloudflare-proxied is a boolean flag that defaults to false.
	// Only pass it when proxied=true (no =value needed, just the flag itself).
	extraArgs := []string{}
	if cfCfg.Proxied {
		extraArgs = append(extraArgs, "--cloudflare-proxied")
	}

	// Add zone ID if specified (avoids API calls to list zones)
	if cfCfg.ZoneID != "" {
		extraArgs = append(extraArgs, "--zone-id-filter="+cfCfg.ZoneID)
	}

	values := helm.Values{
		"provider": helm.Values{
			"name": "cloudflare",
		},
		"txtOwnerId":    ownerID,
		"policy":        policy,
		"sources":       sources,
		"domainFilters": domainFilters,
		// Cloudflare API token - inject directly from secret
		"env": []helm.Values{
			{
				"name": "CF_API_TOKEN",
				"valueFrom": helm.Values{
					"secretKeyRef": helm.Values{
						"name": cloudflareSecretName,
						"key":  "api-token",
					},
				},
			},
		},
		"extraArgs": extraArgs,
		// Deployment configuration
		"replicaCount": 1,
		"podDisruptionBudget": helm.Values{
			"enabled":      true,
			"minAvailable": 1,
		},
		// Run on worker nodes - control plane nodes have Cilium network restrictions
		// that prevent outbound DNS/HTTPS connections needed for Cloudflare API
		"affinity": helm.Values{
			"nodeAffinity": helm.Values{
				"requiredDuringSchedulingIgnoredDuringExecution": helm.Values{
					"nodeSelectorTerms": []helm.Values{
						{
							"matchExpressions": []helm.Values{
								{
									"key":      "node-role.kubernetes.io/control-plane",
									"operator": "DoesNotExist",
								},
							},
						},
					},
				},
			},
		},
		"tolerations": []helm.Values{
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
		},
		// Service account configuration
		"serviceAccount": helm.Values{
			"create": true,
			"name":   "external-dns",
		},
		// RBAC configuration
		"rbac": helm.Values{
			"create": true,
		},
		// Logging
		"logLevel":  "info",
		"logFormat": "text",
		// Interval between DNS sync cycles
		"interval": "1m",
		// Registry for TXT records to track ownership
		"registry": "txt",
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, extDNSCfg.Helm.Values)
}
