// Package addons provides functionality for installing cluster addons.
package addons

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyOpts controls which addons are included in the installation.
type applyOpts struct {
	includeCilium   bool
	includeOperator bool
}

// Apply installs configured addons to the Kubernetes cluster.
func Apply(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	if len(kubeconfig) == 0 {
		return fmt.Errorf("kubeconfig is required for addon installation")
	}

	if !hasEnabledAddons(cfg) {
		return nil
	}

	return applyAddons(ctx, cfg, kubeconfig, networkID, applyOpts{
		includeCilium:   true,
		includeOperator: true,
	})
}

// ApplyCilium installs only the Cilium CNI addon.
// This is used by the operator-centric flow to install CNI before other addons.
func ApplyCilium(ctx context.Context, cfg *config.Config, kubeconfig []byte) error {
	if len(kubeconfig) == 0 {
		return fmt.Errorf("kubeconfig is required for Cilium installation")
	}

	client, err := k8sclient.NewFromKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	if cfg.Addons.Cilium.Enabled {
		if err := applyCilium(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install Cilium: %w", err)
		}
	}

	return nil
}

// ApplyWithoutCilium installs all configured addons except Cilium and Operator.
// This is used by the operator-centric flow after CNI is ready.
func ApplyWithoutCilium(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	if len(kubeconfig) == 0 {
		return fmt.Errorf("kubeconfig is required for addon installation")
	}

	return applyAddons(ctx, cfg, kubeconfig, networkID, applyOpts{
		includeCilium:   false,
		includeOperator: false,
	})
}

// applyAddons is the shared implementation for Apply and ApplyWithoutCilium.
func applyAddons(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64, opts applyOpts) error {
	if err := validateAddonConfig(cfg); err != nil {
		return fmt.Errorf("addon configuration validation failed: %w", err)
	}

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

	// Install Cilium CNI (network foundation)
	if opts.includeCilium && cfg.Addons.Cilium.Enabled {
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
	if cfg.Addons.ExternalDNS.Enabled {
		if err := applyExternalDNS(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install External DNS: %w", err)
		}
	}

	if cfg.Addons.ArgoCD.Enabled {
		if err := applyArgoCD(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install ArgoCD: %w", err)
		}
	}

	if cfg.Addons.KubePrometheusStack.Enabled {
		if err := applyKubePrometheusStack(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install kube-prometheus-stack: %w", err)
		}
	}

	if cfg.Addons.TalosBackup.Enabled {
		if err := waitForTalosCRD(ctx, client); err != nil {
			log.Printf("[addons] WARNING: Talos Backup SKIPPED - %v", err)
		} else {
			if err := applyTalosBackup(ctx, client, cfg); err != nil {
				return fmt.Errorf("failed to install Talos Backup: %w", err)
			}
		}
	}

	// Install k8zner-operator (self-healing)
	if opts.includeOperator && cfg.Addons.Operator.Enabled {
		if err := applyOperator(ctx, client, cfg); err != nil {
			return fmt.Errorf("failed to install k8zner-operator: %w", err)
		}
	}

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

// validateAddonConfig checks that required configuration is set for enabled addons.
// Duplicates some v2 validations for defense-in-depth (fail at install time, not just load time).
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

// Talos CRD wait time constants
const (
	// DefaultTalosCRDWaitTime is the default time to wait for Talos API CRD registration.
	// Increased from 2 minutes to 5 minutes to handle slow cluster bootstraps.
	DefaultTalosCRDWaitTime = 5 * time.Minute

	// TalosCRDCheckInterval is how often to check for CRD availability.
	TalosCRDCheckInterval = 5 * time.Second
)

// waitForTalosCRD waits for the Talos API CRD (talos.dev/v1alpha1) to be available.
// The CRD is registered asynchronously after bootstrap when kubernetesTalosAPIAccess is enabled.
func waitForTalosCRD(ctx context.Context, client k8sclient.Client) error {
	return waitForTalosCRDWithTimeout(ctx, client, DefaultTalosCRDWaitTime)
}

// waitForTalosCRDWithTimeout waits for the Talos API CRD with a custom timeout.
func waitForTalosCRDWithTimeout(ctx context.Context, client k8sclient.Client, timeout time.Duration) error {
	const talosCRD = "talos.dev/v1alpha1/ServiceAccount"

	if timeout <= 0 {
		timeout = DefaultTalosCRDWaitTime
	}

	deadline := time.Now().Add(timeout)
	attempt := 0
	startTime := time.Now()

	log.Printf("[addons] Waiting up to %v for Talos API CRD to be registered...", timeout)

	for time.Now().Before(deadline) {
		attempt++

		// Check if the CRD exists
		exists, err := client.HasCRD(ctx, talosCRD)
		if err != nil {
			log.Printf("[addons] Error checking for Talos CRD (attempt %d): %v", attempt, err)
		} else if exists {
			elapsed := time.Since(startTime).Round(time.Second)
			log.Printf("[addons] Talos API CRD is available (after %v, %d attempts)", elapsed, attempt)
			return nil
		}

		// Log progress every 30 seconds
		if attempt%6 == 0 {
			elapsed := time.Since(startTime).Round(time.Second)
			remaining := timeout - elapsed
			log.Printf("[addons] Still waiting for Talos API CRD (elapsed: %v, remaining: %v)...", elapsed, remaining)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(TalosCRDCheckInterval):
			// Continue waiting
		}
	}

	elapsed := time.Since(startTime).Round(time.Second)
	return fmt.Errorf("timeout after %v waiting for Talos API CRD - ensure kubernetesTalosAPIAccess is enabled in machine config", elapsed)
}

// IngressClass wait time constants
const (
	// DefaultIngressClassWaitTime is the default time to wait for an IngressClass to be ready.
	// Increased to 5 minutes to allow time for Traefik Deployment to fully deploy.
	DefaultIngressClassWaitTime = 5 * time.Minute

	// IngressClassCheckInterval is how often to check for IngressClass availability.
	IngressClassCheckInterval = 5 * time.Second
)

// waitForIngressClass waits for an IngressClass to be available.
// This is useful for addons that create Ingress resources and need Traefik/nginx to be ready.
func waitForIngressClass(ctx context.Context, client k8sclient.Client, name string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultIngressClassWaitTime
	}

	deadline := time.Now().Add(timeout)
	attempt := 0
	startTime := time.Now()

	log.Printf("[addons] Waiting up to %v for IngressClass %q to be available...", timeout, name)

	for time.Now().Before(deadline) {
		attempt++

		exists, err := client.HasIngressClass(ctx, name)
		if err != nil {
			log.Printf("[addons] Error checking for IngressClass %q (attempt %d): %v", name, attempt, err)
		} else if exists {
			elapsed := time.Since(startTime).Round(time.Second)
			log.Printf("[addons] IngressClass %q is available (after %v)", name, elapsed)
			return nil
		}

		// Log progress every 30 seconds
		if attempt%6 == 0 {
			elapsed := time.Since(startTime).Round(time.Second)
			log.Printf("[addons] Still waiting for IngressClass %q (elapsed: %v)...", name, elapsed)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(IngressClassCheckInterval):
			// Continue waiting
		}
	}

	elapsed := time.Since(startTime).Round(time.Second)
	return fmt.Errorf("timeout after %v waiting for IngressClass %q - ensure Traefik/ingress controller is installed", elapsed, name)
}
