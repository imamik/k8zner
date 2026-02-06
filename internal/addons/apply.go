// Package addons provides functionality for installing cluster addons.
package addons

import (
	"context"
	"fmt"
	"log"

	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// Apply installs configured addons to the Kubernetes cluster.
func Apply(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	if len(kubeconfig) == 0 {
		return fmt.Errorf("kubeconfig is required for addon installation")
	}

	// Check if any addons are enabled
	if !hasEnabledAddons(cfg) {
		return nil
	}

	// Pre-flight validation: check addon configuration requirements
	if err := validateAddonConfig(cfg); err != nil {
		return fmt.Errorf("addon configuration validation failed: %w", err)
	}

	// Create k8s client from kubeconfig bytes
	client, err := k8sclient.NewFromKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Install CRDs first (before addons that depend on them)
	if cfg.Addons.GatewayAPICRDs.Enabled {
		if err := applyGatewayAPICRDs(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Gateway API CRDs: %w", err)
		}
	}

	if cfg.Addons.PrometheusOperatorCRDs.Enabled {
		if err := applyPrometheusOperatorCRDs(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Prometheus Operator CRDs: %w", err)
		}
	}

	// Install Talos CCM (node lifecycle management)
	if cfg.Addons.TalosCCM.Enabled {
		if err := applyTalosCCM(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Talos CCM: %w", err)
		}
	}

	// Install Cilium CNI first (network foundation)
	if cfg.Addons.Cilium.Enabled {
		if err := applyCilium(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Cilium: %w", err)
		}
	}

	// Create hcloud secret if CCM or CSI are enabled
	if cfg.Addons.CCM.Enabled || cfg.Addons.CSI.Enabled {
		if err := createHCloudSecret(ctx, client, cfg.HCloudToken, networkID); err != nil {
			return fmt.Errorf("failed to create hcloud secret: %w", err)
		}
	}

	if cfg.Addons.CCM.Enabled {
		if err := applyCCM(ctx, client, cfg, networkID); err != nil {
			return fmt.Errorf("failed to install CCM: %w", err)
		}
	}

	if cfg.Addons.CSI.Enabled {
		if err := applyCSI(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install CSI: %w", err)
		}
	}

	if cfg.Addons.MetricsServer.Enabled {
		if err := applyMetricsServer(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Metrics Server: %w", err)
		}
	}

	if cfg.Addons.CertManager.Enabled {
		if err := applyCertManager(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Cert Manager: %w", err)
		}
	}

	// Create Cloudflare secrets if any Cloudflare feature is enabled
	if cfg.Addons.Cloudflare.Enabled {
		if err := createCloudflareSecrets(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to create Cloudflare secrets: %w", err)
		}
	}

	// Cert-manager Cloudflare ClusterIssuer (after cert-manager and Cloudflare secrets)
	if cfg.Addons.CertManager.Enabled && cfg.Addons.CertManager.Cloudflare.Enabled {
		if err := applyCertManagerCloudflare(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to configure Cloudflare DNS01 issuer: %w", err)
		}
	}

	// Install Traefik before external-DNS (needs ingress controller)
	if cfg.Addons.Traefik.Enabled {
		if err := applyTraefik(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Traefik: %w", err)
		}
	}

	// External-DNS (requires Cloudflare secrets AND ingress controllers)
	// Must be installed after ingress controllers so Ingress status has external IP
	if cfg.Addons.ExternalDNS.Enabled {
		if err := applyExternalDNS(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install External DNS: %w", err)
		}
	}

	if cfg.Addons.ArgoCD.Enabled {
		log.Printf("[addons] Installing ArgoCD...")
		if err := applyArgoCD(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install ArgoCD: %w", err)
		}
		log.Printf("[addons] ArgoCD installed successfully")
	}

	// Install kube-prometheus-stack (full monitoring: Prometheus, Grafana, Alertmanager)
	// Must be installed after ingress controllers and external-dns for subdomain exposure
	if cfg.Addons.KubePrometheusStack.Enabled {
		if err := applyKubePrometheusStack(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install kube-prometheus-stack: %w", err)
		}
	}

	// Install Talos Backup (etcd backup to S3)
	if cfg.Addons.TalosBackup.Enabled {
		log.Printf("[addons] Installing Talos Backup (schedule=%s, bucket=%s)...",
			cfg.Addons.TalosBackup.Schedule, cfg.Addons.TalosBackup.S3Bucket)
		if err := applyTalosBackup(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Talos Backup: %w", err)
		}
		log.Printf("[addons] Talos Backup installed successfully")
	}

	// Install k8zner-operator (self-healing)
	if cfg.Addons.Operator.Enabled {
		if err := applyOperator(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install k8zner-operator: %w", err)
		}
	}

	return nil
}

// ApplyCilium installs only the Cilium CNI addon.
// This is used by the operator-centric flow to install CNI before other addons.
func ApplyCilium(ctx context.Context, cfg *config.Config, kubeconfig []byte) error {
	if len(kubeconfig) == 0 {
		return fmt.Errorf("kubeconfig is required for Cilium installation")
	}

	// Create k8s client from kubeconfig bytes
	client, err := k8sclient.NewFromKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Install Cilium CNI
	if cfg.Addons.Cilium.Enabled {
		if err := applyCilium(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Cilium: %w", err)
		}
	}

	return nil
}

// ApplyWithoutCilium installs all configured addons except Cilium.
// This is used by the operator-centric flow after CNI is ready.
func ApplyWithoutCilium(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	if len(kubeconfig) == 0 {
		return fmt.Errorf("kubeconfig is required for addon installation")
	}

	log.Printf("[addons] Starting addon installation (networkID=%d)", networkID)
	log.Printf("[addons] Enabled addons: CCM=%v, CSI=%v, MetricsServer=%v, CertManager=%v, Traefik=%v, ArgoCD=%v, TalosBackup=%v",
		cfg.Addons.CCM.Enabled,
		cfg.Addons.CSI.Enabled,
		cfg.Addons.MetricsServer.Enabled,
		cfg.Addons.CertManager.Enabled,
		cfg.Addons.Traefik.Enabled,
		cfg.Addons.ArgoCD.Enabled,
		cfg.Addons.TalosBackup.Enabled,
	)

	// Pre-flight validation: check addon configuration requirements
	if err := validateAddonConfig(cfg); err != nil {
		return fmt.Errorf("addon configuration validation failed: %w", err)
	}
	log.Printf("[addons] Pre-flight validation passed")

	// Create k8s client from kubeconfig bytes
	client, err := k8sclient.NewFromKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}
	log.Printf("[addons] Kubernetes client created successfully")

	// Install CRDs first (before addons that depend on them)
	if cfg.Addons.GatewayAPICRDs.Enabled {
		log.Printf("[addons] Installing Gateway API CRDs...")
		if err := applyGatewayAPICRDs(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Gateway API CRDs: %w", err)
		}
	}

	if cfg.Addons.PrometheusOperatorCRDs.Enabled {
		if err := applyPrometheusOperatorCRDs(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Prometheus Operator CRDs: %w", err)
		}
	}

	// Install Talos CCM (node lifecycle management)
	if cfg.Addons.TalosCCM.Enabled {
		if err := applyTalosCCM(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Talos CCM: %w", err)
		}
	}

	// Skip Cilium - it's already installed in CNI phase

	// Create hcloud secret if CCM or CSI are enabled
	if cfg.Addons.CCM.Enabled || cfg.Addons.CSI.Enabled {
		log.Printf("[addons] Creating hcloud secret for CCM/CSI (networkID=%d, tokenLength=%d)", networkID, len(cfg.HCloudToken))
		if err := createHCloudSecret(ctx, client, cfg.HCloudToken, networkID); err != nil {
			return fmt.Errorf("failed to create hcloud secret: %w", err)
		}
	}

	if cfg.Addons.CCM.Enabled {
		log.Printf("[addons] Installing Hetzner Cloud Controller Manager...")
		if err := applyCCM(ctx, client, cfg, networkID); err != nil {
			return fmt.Errorf("failed to install CCM: %w", err)
		}
		log.Printf("[addons] CCM installed successfully")
	}

	if cfg.Addons.CSI.Enabled {
		log.Printf("[addons] Installing Hetzner CSI Driver...")
		if err := applyCSI(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install CSI: %w", err)
		}
		log.Printf("[addons] CSI installed successfully")
	}

	if cfg.Addons.MetricsServer.Enabled {
		if err := applyMetricsServer(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Metrics Server: %w", err)
		}
	}

	if cfg.Addons.CertManager.Enabled {
		if err := applyCertManager(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Cert Manager: %w", err)
		}
	}

	// Create Cloudflare secrets if any Cloudflare feature is enabled
	if cfg.Addons.Cloudflare.Enabled {
		if err := createCloudflareSecrets(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to create Cloudflare secrets: %w", err)
		}
	}

	// Cert-manager Cloudflare ClusterIssuer (after cert-manager and Cloudflare secrets)
	if cfg.Addons.CertManager.Enabled && cfg.Addons.CertManager.Cloudflare.Enabled {
		if err := applyCertManagerCloudflare(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to configure Cloudflare DNS01 issuer: %w", err)
		}
	}

	// Install Traefik before external-DNS (needs ingress controller)
	if cfg.Addons.Traefik.Enabled {
		if err := applyTraefik(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Traefik: %w", err)
		}
	}

	// External-DNS (requires Cloudflare secrets AND ingress controllers)
	// Must be installed after ingress controllers so Ingress status has external IP
	if cfg.Addons.ExternalDNS.Enabled {
		if err := applyExternalDNS(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install External DNS: %w", err)
		}
	}

	if cfg.Addons.ArgoCD.Enabled {
		log.Printf("[addons] Installing ArgoCD...")
		if err := applyArgoCD(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install ArgoCD: %w", err)
		}
		log.Printf("[addons] ArgoCD installed successfully")
	}

	// Install kube-prometheus-stack (full monitoring: Prometheus, Grafana, Alertmanager)
	// Must be installed after ingress controllers and external-dns for subdomain exposure
	if cfg.Addons.KubePrometheusStack.Enabled {
		if err := applyKubePrometheusStack(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install kube-prometheus-stack: %w", err)
		}
	}

	// Install Talos Backup (etcd backup to S3)
	if cfg.Addons.TalosBackup.Enabled {
		log.Printf("[addons] Installing Talos Backup (schedule=%s, bucket=%s)...",
			cfg.Addons.TalosBackup.Schedule, cfg.Addons.TalosBackup.S3Bucket)
		if err := applyTalosBackup(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Talos Backup: %w", err)
		}
		log.Printf("[addons] Talos Backup installed successfully")
	}

	// Skip operator installation - it's already running if we're in this flow

	return nil
}

// getControlPlaneCount returns the total number of control plane nodes.
func getControlPlaneCount(cfg *config.Config) int {
	count := 0
	for _, pool := range cfg.ControlPlane.NodePools {
		count += pool.Count
	}
	return count
}

func hasEnabledAddons(cfg *config.Config) bool {
	a := &cfg.Addons
	return a.GatewayAPICRDs.Enabled || a.PrometheusOperatorCRDs.Enabled ||
		a.TalosCCM.Enabled || a.Cilium.Enabled || a.CCM.Enabled || a.CSI.Enabled ||
		a.MetricsServer.Enabled || a.CertManager.Enabled || a.Traefik.Enabled ||
		a.ArgoCD.Enabled || a.Cloudflare.Enabled || a.ExternalDNS.Enabled ||
		a.TalosBackup.Enabled || a.KubePrometheusStack.Enabled || a.Operator.Enabled
}

// validateAddonConfig performs pre-flight validation of addon configuration.
// Returns an error if required configuration is missing for enabled addons.
//
// Note: Some validations (e.g., S3 credentials) are intentionally duplicated
// from v2/types.go for defense-in-depth. The v2 layer validates at config load
// time (fail fast), while this validates at addon install time (runtime check).
func validateAddonConfig(cfg *config.Config) error {
	a := &cfg.Addons

	// CCM/CSI/Operator require HCloud token
	if (a.CCM.Enabled || a.CSI.Enabled || a.Operator.Enabled) && cfg.HCloudToken == "" {
		return fmt.Errorf("ccm/csi/operator addons require hcloud_token to be set")
	}

	// Cloudflare addons require API token
	if a.Cloudflare.Enabled && a.Cloudflare.APIToken == "" {
		return fmt.Errorf("cloudflare addon requires api_token to be set")
	}

	// ExternalDNS uses Cloudflare as the DNS provider
	if a.ExternalDNS.Enabled && !a.Cloudflare.Enabled {
		return fmt.Errorf("external-dns addon requires cloudflare addon to be enabled")
	}

	// CertManager Cloudflare integration requires Cloudflare addon
	if a.CertManager.Enabled && a.CertManager.Cloudflare.Enabled && !a.Cloudflare.Enabled {
		return fmt.Errorf("cert-manager cloudflare integration requires cloudflare addon to be enabled")
	}

	// TalosBackup requires S3 configuration
	if a.TalosBackup.Enabled {
		if a.TalosBackup.S3Bucket == "" {
			return fmt.Errorf("talos-backup addon requires s3_bucket to be set")
		}
		if a.TalosBackup.S3AccessKey == "" || a.TalosBackup.S3SecretKey == "" {
			return fmt.Errorf("talos-backup addon requires s3_access_key and s3_secret_key to be set")
		}
		if a.TalosBackup.S3Endpoint == "" {
			return fmt.Errorf("talos-backup addon requires s3_endpoint to be set")
		}
	}

	return nil
}
