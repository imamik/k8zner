package addons

import (
	"context"
	"fmt"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyArgoCD installs ArgoCD, a declarative GitOps continuous delivery tool.
// ArgoCD continuously monitors running applications and compares their live state
// against the desired state specified in Git, automatically syncing any deviations.
//
// Features:
//   - Web UI for application visualization and management
//   - Multi-cluster deployment support
//   - SSO integration (OIDC, OAuth2, LDAP, SAML)
//   - Webhook triggers for automated sync
//   - Health assessment and resource tracking
//
// See: https://argo-cd.readthedocs.io/
func applyArgoCD(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	// Create namespace first
	namespaceYAML := createArgoCDNamespace()
	if err := applyManifests(ctx, client, "argocd-namespace", []byte(namespaceYAML)); err != nil {
		return fmt.Errorf("failed to create argocd namespace: %w", err)
	}

	// Build values based on configuration
	values := buildArgoCDValues(cfg)

	// Get chart spec with any config overrides
	spec := helm.GetChartSpec("argo-cd", cfg.Addons.ArgoCD.Helm)

	// Render helm chart
	manifestBytes, err := helm.RenderFromSpec(ctx, spec, "argocd", values)
	if err != nil {
		return fmt.Errorf("failed to render argocd chart: %w", err)
	}

	// Apply all manifests
	if err := applyManifests(ctx, client, "argocd", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply argocd manifests: %w", err)
	}

	return nil
}

// buildArgoCDValues creates helm values for ArgoCD configuration.
func buildArgoCDValues(cfg *config.Config) helm.Values {
	argoCDCfg := cfg.Addons.ArgoCD

	values := helm.Values{
		// Global settings
		"global": helm.Values{
			"domain": argoCDCfg.IngressHost,
		},
		// Install CRDs
		"crds": helm.Values{
			"install": true,
			"keep":    true,
		},
		// Disable the redis secret init job - we don't use password auth
		// This is a TOP-LEVEL key, not nested under redis
		// See: https://github.com/argoproj/argo-helm/issues/3057
		"redisSecretInit": helm.Values{
			"enabled": false,
		},
		// Controller configuration
		"controller": buildArgoCDController(argoCDCfg),
		// Server configuration - pass full config for ingress annotations
		"server": buildArgoCDServer(cfg),
		// Repo server configuration
		"repoServer": buildArgoCDRepoServer(argoCDCfg),
		// Redis configuration
		"redis": buildArgoCDRedis(argoCDCfg),
		// Dex (OIDC provider) - disabled by default, users can enable via custom values
		"dex": helm.Values{
			"enabled": false,
		},
		// ApplicationSet controller
		"applicationSet": helm.Values{
			"enabled": true,
		},
		// Notifications controller
		"notifications": helm.Values{
			"enabled": true,
		},
	}

	// Configure HA mode
	if argoCDCfg.HA {
		values["redis-ha"] = helm.Values{
			"enabled": true,
			// Disable redis-ha auth to avoid secret dependency issues
			// See: https://github.com/argoproj/argo-helm/issues/3057
			"auth": false,
		}
		// Disable standalone redis when using HA
		values["redis"] = helm.Values{
			"enabled": false,
		}
	} else {
		// Explicitly disable redis-ha for non-HA mode
		values["redis-ha"] = helm.Values{
			"enabled": false,
		}
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, argoCDCfg.Helm.Values)
}

// buildArgoCDController creates the application controller configuration.
func buildArgoCDController(cfg config.ArgoCDConfig) helm.Values {
	replicas := 1
	if cfg.HA && cfg.ControllerReplicas != nil {
		replicas = *cfg.ControllerReplicas
	}

	return helm.Values{
		"replicas": replicas,
		// Add tolerations for CCM uninitialized taint
		"tolerations": []helm.Values{
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
		},
		// Resource defaults for production
		"resources": helm.Values{
			"requests": helm.Values{
				"cpu":    "100m",
				"memory": "256Mi",
			},
			"limits": helm.Values{
				"memory": "512Mi",
			},
		},
	}
}

// buildArgoCDServer creates the ArgoCD server configuration.
func buildArgoCDServer(cfg *config.Config) helm.Values {
	argoCDCfg := cfg.Addons.ArgoCD
	replicas := 1
	if argoCDCfg.HA {
		replicas = 2
		if argoCDCfg.ServerReplicas != nil {
			replicas = *argoCDCfg.ServerReplicas
		}
	}

	server := helm.Values{
		"replicas": replicas,
		// Add tolerations for CCM uninitialized taint
		"tolerations": []helm.Values{
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
		},
		// Resource defaults for production
		"resources": helm.Values{
			"requests": helm.Values{
				"cpu":    "50m",
				"memory": "128Mi",
			},
			"limits": helm.Values{
				"memory": "256Mi",
			},
		},
	}

	// Configure ingress if enabled
	if argoCDCfg.IngressEnabled && argoCDCfg.IngressHost != "" {
		server["ingress"] = buildArgoCDIngress(cfg)
	}

	return server
}

// buildArgoCDRepoServer creates the repo server configuration.
func buildArgoCDRepoServer(cfg config.ArgoCDConfig) helm.Values {
	replicas := 1
	if cfg.HA {
		replicas = 2
		if cfg.RepoServerReplicas != nil {
			replicas = *cfg.RepoServerReplicas
		}
	}

	return helm.Values{
		"replicas": replicas,
		// Add tolerations for CCM uninitialized taint
		"tolerations": []helm.Values{
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
		},
		// Resource defaults for production
		"resources": helm.Values{
			"requests": helm.Values{
				"cpu":    "50m",
				"memory": "128Mi",
			},
			"limits": helm.Values{
				"memory": "512Mi",
			},
		},
	}
}

// buildArgoCDRedis creates the Redis configuration.
func buildArgoCDRedis(cfg config.ArgoCDConfig) helm.Values {
	// Disable standalone redis if HA is enabled (uses redis-ha instead)
	if cfg.HA {
		return helm.Values{
			"enabled": false,
		}
	}

	return helm.Values{
		"enabled": true,
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
	}
}

// buildArgoCDIngress creates the ingress configuration for ArgoCD server.
func buildArgoCDIngress(cfg *config.Config) helm.Values {
	argoCDCfg := cfg.Addons.ArgoCD

	ingress := helm.Values{
		"enabled": true,
		"hosts": []string{
			argoCDCfg.IngressHost,
		},
		// ArgoCD server handles TLS termination internally
		"https": true,
	}

	// Set ingress class if specified
	if argoCDCfg.IngressClassName != "" {
		ingress["ingressClassName"] = argoCDCfg.IngressClassName
	}

	// Configure TLS if enabled
	if argoCDCfg.IngressTLS {
		ingress["tls"] = []helm.Values{
			{
				"hosts": []string{
					argoCDCfg.IngressHost,
				},
				"secretName": "argocd-server-tls",
			},
		}

		// Build annotations for TLS and DNS
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
			annotations["external-dns.alpha.kubernetes.io/hostname"] = argoCDCfg.IngressHost
		}

		ingress["annotations"] = annotations
	}

	return ingress
}

// createArgoCDNamespace returns the argocd namespace manifest.
func createArgoCDNamespace() string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: argocd
  labels:
    name: argocd
`
}
