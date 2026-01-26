package addons

import (
	"context"
	"fmt"
	"strconv"

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
	extraArgs := []string{
		"--cloudflare-proxied=" + strconv.FormatBool(cfCfg.Proxied),
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
		// Schedule on control plane nodes for reliability
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
