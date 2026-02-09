package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
	hcloudInternal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/provisioning/destroy"
)

// provisionImage ensures the Talos image snapshot exists.
func provisionImage(pCtx *provisioning.Context) error {
	log.Println("Phase 1/6: Ensuring Talos image snapshot...")
	imgProvisioner := newImageProvisioner()
	if err := imgProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("image provisioning failed: %w", err)
	}
	return nil
}

// provisionInfrastructure creates network, firewall, LB, and placement group.
func provisionInfrastructure(pCtx *provisioning.Context) error {
	log.Println("Phase 2/6: Creating infrastructure (network, firewall, LB, placement group)...")
	infraProvisioner := newInfraProvisioner()
	if err := infraProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("infrastructure provisioning failed: %w", err)
	}
	return nil
}

// provisionFirstControlPlane creates the initial control plane node.
func provisionFirstControlPlane(cfg *config.Config, pCtx *provisioning.Context) error {
	log.Println("Phase 3/6: Creating first control plane...")
	originalCPCount := cfg.ControlPlane.NodePools[0].Count
	cfg.ControlPlane.NodePools[0].Count = 1
	originalWorkerCount := 0
	if len(cfg.Workers) > 0 {
		originalWorkerCount = cfg.Workers[0].Count
		cfg.Workers[0].Count = 0
	}

	computeProvisioner := newComputeProvisioner()
	if err := computeProvisioner.Provision(pCtx); err != nil {
		cfg.ControlPlane.NodePools[0].Count = originalCPCount
		if len(cfg.Workers) > 0 {
			cfg.Workers[0].Count = originalWorkerCount
		}
		return fmt.Errorf("compute provisioning failed: %w", err)
	}

	cfg.ControlPlane.NodePools[0].Count = originalCPCount
	if len(cfg.Workers) > 0 {
		cfg.Workers[0].Count = originalWorkerCount
	}

	return nil
}

// bootstrapCluster bootstraps Talos on the first control plane node.
func bootstrapCluster(pCtx *provisioning.Context) error {
	log.Println("Phase 4/6: Bootstrapping cluster...")
	clstrProvisioner := newClusterProvisioner()
	if err := clstrProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("cluster bootstrap failed: %w", err)
	}
	return nil
}

// installOperator installs the k8zner-operator addon with retry logic.
func installOperator(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	log.Println("Phase 5/6: Installing k8zner operator...")
	cfg.Addons.Operator.Enabled = true
	cfg.Addons.Operator.HostNetwork = true

	if err := installOperatorOnly(ctx, cfg, kubeconfig, networkID); err != nil {
		return fmt.Errorf("operator installation failed: %w", err)
	}
	return nil
}

// installOperatorOnly installs just the k8zner-operator addon with retry logic.
func installOperatorOnly(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	savedAddons := cfg.Addons
	cfg.Addons = config.AddonsConfig{
		Operator: savedAddons.Operator,
	}
	defer func() {
		cfg.Addons = savedAddons
	}()

	const maxRetries = 10
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			delay := time.Duration(15) * time.Second
			log.Printf("Retrying operator installation in %v (attempt %d/%d)...", delay, i+1, maxRetries)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		lastErr = addons.Apply(ctx, cfg, kubeconfig, networkID)
		if lastErr == nil {
			return nil
		}

		errStr := lastErr.Error()
		if !isTransientError(errStr) {
			return lastErr
		}
		log.Printf("Transient error during operator installation: %v", lastErr)
	}

	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// isTransientError checks if an error is likely transient and worth retrying.
func isTransientError(errStr string) bool {
	transientPatterns := []string{
		"EOF",
		"connection refused",
		"connection reset",
		"i/o timeout",
		"no such host",
		"TLS handshake timeout",
		"context deadline exceeded",
	}
	for _, pattern := range transientPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// initializeTalosGenerator creates a Talos configuration generator.
func initializeTalosGenerator(cfg *config.Config) (provisioning.TalosConfigProducer, error) {
	endpoint := fmt.Sprintf("https://%s-kube-api:%d", cfg.ClusterName, config.KubeAPIPort)

	sb, err := getOrGenerateSecrets(secretsFile, cfg.Talos.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize secrets: %w", err)
	}

	return newTalosGenerator(
		cfg.ClusterName,
		cfg.Kubernetes.Version,
		cfg.Talos.Version,
		endpoint,
		sb,
	), nil
}

// writeTalosFiles persists Talos secrets and client config to disk.
func writeTalosFiles(talosGen provisioning.TalosConfigProducer) error {
	clientCfgBytes, err := talosGen.GetClientConfig()
	if err != nil {
		return fmt.Errorf("failed to generate talosconfig: %w", err)
	}

	if err := writeFile(talosConfigPath, clientCfgBytes, 0600); err != nil {
		return fmt.Errorf("failed to write talosconfig: %w", err)
	}

	return nil
}

// writeKubeconfig persists the Kubernetes client config to disk.
func writeKubeconfig(kubeconfig []byte) error {
	if len(kubeconfig) == 0 {
		return nil
	}

	if err := writeFile(kubeconfigPath, kubeconfig, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	return nil
}

// cleanupOnFailure destroys all resources created during a failed apply.
func cleanupOnFailure(ctx context.Context, cfg *config.Config, infraClient hcloudInternal.InfrastructureManager) error {
	log.Printf("Cleaning up resources for cluster: %s", cfg.ClusterName)

	pCtx := provisioning.NewContext(ctx, cfg, infraClient, nil)

	destroyer := destroy.NewProvisioner()
	if err := destroyer.Provision(pCtx); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	log.Printf("Cleanup complete for cluster: %s", cfg.ClusterName)
	return nil
}

// printApplySuccess outputs completion message and next steps.
func printApplySuccess(cfg *config.Config, wait bool) {
	fmt.Printf("\nBootstrap complete!\n")
	fmt.Printf("Secrets saved to: %s\n", secretsFile)
	fmt.Printf("Talos config saved to: %s\n", talosConfigPath)
	fmt.Printf("Kubeconfig saved to: %s\n", kubeconfigPath)

	if !wait {
		fmt.Printf("\nThe operator is now provisioning:\n")
		fmt.Printf("  - Cilium CNI\n")
		fmt.Printf("  - Additional control planes\n")
		fmt.Printf("  - Worker nodes\n")
		fmt.Printf("  - Cluster addons\n")
		fmt.Printf("\nMonitor progress with:\n")
		fmt.Printf("  k8zner doctor --watch\n")
	}

	fmt.Printf("\nAccess your cluster:\n")
	fmt.Printf("  export KUBECONFIG=%s\n", kubeconfigPath)
	fmt.Printf("  kubectl get nodes\n")
}
