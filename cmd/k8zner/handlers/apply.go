// Package handlers implements the business logic for CLI commands.
//
// This package contains handler functions that are called by command definitions
// in the commands package. Handlers are framework-agnostic and can be tested
// independently of the CLI framework.
package handlers

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"

	"github.com/imamik/k8zner/internal/config"
	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/orchestration"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/platform/talos"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/prerequisites"
)

const (
	secretsFile     = "secrets.yaml"
	talosConfigPath = "talosconfig"
	kubeconfigPath  = "kubeconfig"
)

// Reconciler interface for testing - matches orchestration.Reconciler.
type Reconciler interface {
	Reconcile(ctx context.Context) ([]byte, error)
}

// Factory function variables - can be replaced in tests for dependency injection.
var (
	// newInfraClient creates a new infrastructure client.
	newInfraClient = func(token string) hcloud.InfrastructureManager {
		return hcloud.NewRealClient(token)
	}

	// getOrGenerateSecrets loads or generates Talos secrets.
	getOrGenerateSecrets = talos.GetOrGenerateSecrets

	// newTalosGenerator creates a new Talos configuration generator.
	newTalosGenerator = func(clusterName, kubernetesVersion, talosVersion, endpoint string, sb *secrets.Bundle) provisioning.TalosConfigProducer {
		return talos.NewGenerator(clusterName, kubernetesVersion, talosVersion, endpoint, sb)
	}

	// newReconciler creates a new infrastructure reconciler.
	newReconciler = func(infra hcloud.InfrastructureManager, talosGen provisioning.TalosConfigProducer, cfg *config.Config) Reconciler {
		return orchestration.NewReconciler(infra, talosGen, cfg)
	}

	// checkDefaultPrereqs runs prerequisite checks.
	checkDefaultPrereqs = prerequisites.CheckDefault

	// writeFile writes data to a file (for testing injection).
	writeFile = os.WriteFile

	// loadConfigFile loads config from file (for testing injection).
	loadConfigFile = config.LoadFile

	// loadV2ConfigFile loads v2 config from file (for testing injection).
	loadV2ConfigFile = v2.Load

	// expandV2Config expands v2 config to internal format (for testing injection).
	expandV2Config = v2.Expand

	// findV2ConfigFile finds the v2 config file (for testing injection).
	findV2ConfigFile = v2.FindConfigFile
)

// Apply provisions a Kubernetes cluster on Hetzner Cloud using Talos Linux.
//
// This function orchestrates the complete cluster provisioning workflow:
//  1. Loads and validates cluster configuration (auto-detects v2 or legacy format)
//  2. Initializes Hetzner Cloud client using HCLOUD_TOKEN environment variable
//  3. Generates Talos machine configurations and persists secrets immediately
//  4. Reconciles cluster infrastructure (networks, servers, load balancers, bootstrap)
//  5. Writes kubeconfig if cluster bootstrap completed successfully
//  6. Installs configured cluster addons (CCM, CSI, etc.) if bootstrap succeeded
//
// Secrets and Talos config are written before reconciliation to ensure they're
// preserved even if reconciliation fails, enabling retry without data loss.
//
// Addon installation is performed separately after infrastructure provisioning
// to maintain clean separation between infrastructure and cluster components.
//
// The function expects HCLOUD_TOKEN to be set in the environment and will
// delegate validation to the Hetzner Cloud client.
func Apply(ctx context.Context, configPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	// Run prerequisites check if enabled (default: true)
	if err := checkPrerequisites(cfg); err != nil {
		return err
	}

	log.Printf("Applying configuration for cluster: %s", cfg.ClusterName)

	client := initializeClient()
	talosGen, err := initializeTalosGenerator(cfg)
	if err != nil {
		return err
	}

	if err := writeTalosFiles(talosGen); err != nil {
		return err
	}

	kubeconfig, err := reconcileInfrastructure(ctx, client, talosGen, cfg)
	if err != nil {
		return err
	}

	if err := writeKubeconfig(kubeconfig); err != nil {
		return err
	}

	printApplySuccess(kubeconfig, cfg)
	return nil
}

// loadConfig loads and validates cluster configuration.
// It auto-detects v2 config format (simple 5-field config) vs legacy format.
// If configPath is empty, it looks for k8zner.yaml in the current directory.
func loadConfig(configPath string) (*config.Config, error) {
	// If no path provided, try to find default v2 config
	if configPath == "" {
		path, err := findV2ConfigFile()
		if err != nil {
			return nil, fmt.Errorf("no config file found: %w\nRun 'k8zner init' to create one", err)
		}
		configPath = path
	}

	// Try loading as v2 config first (the new simplified format)
	v2Cfg, err := loadV2ConfigFile(configPath)
	if err == nil {
		// Successfully loaded as v2, expand to internal format
		log.Printf("Using v2 config: %s", configPath)
		return expandV2Config(v2Cfg)
	}

	// Fall back to legacy config format
	cfg, err := loadConfigFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	log.Printf("Using legacy config: %s", configPath)
	return cfg, nil
}

// initializeClient creates a Hetzner Cloud client using HCLOUD_TOKEN from environment.
// Token validation is delegated to the client.
func initializeClient() hcloud.InfrastructureManager {
	token := os.Getenv("HCLOUD_TOKEN")
	return newInfraClient(token)
}

// initializeTalosGenerator creates a Talos configuration generator for the orchestration.
// Generates machine configs, certificates, and client secrets for cluster access.
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

// reconcileInfrastructure provisions infrastructure and bootstraps the Kubernetes orchestration.
// Returns kubeconfig bytes if bootstrap completed.
// Kubeconfig will be empty if cluster was already bootstrapped.
func reconcileInfrastructure(ctx context.Context, client hcloud.InfrastructureManager, talosGen provisioning.TalosConfigProducer, cfg *config.Config) ([]byte, error) {
	log.Println("Starting infrastructure reconciliation...")

	reconciler := newReconciler(client, talosGen, cfg)
	kubeconfig, err := reconciler.Reconcile(ctx)
	if err != nil {
		return nil, fmt.Errorf("reconciliation failed: %w", err)
	}

	log.Println("Infrastructure reconciliation completed")
	return kubeconfig, nil
}

// writeTalosFiles persists Talos secrets and client config to disk.
// Must be called before reconciliation to ensure secrets survive failures.
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
// Only writes if kubeconfig is non-empty (i.e., cluster bootstrap succeeded).
func writeKubeconfig(kubeconfig []byte) error {
	if len(kubeconfig) == 0 {
		return nil
	}

	if err := writeFile(kubeconfigPath, kubeconfig, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	return nil
}

// printApplySuccess outputs completion message and next steps for the user.
// Message varies depending on whether this was initial bootstrap or re-apply.
func printApplySuccess(kubeconfig []byte, cfg *config.Config) {
	fmt.Printf("\nReconciliation complete!\n")
	fmt.Printf("Secrets saved to: %s\n", secretsFile)
	fmt.Printf("Talos config saved to: %s\n", talosConfigPath)

	if len(kubeconfig) > 0 {
		fmt.Printf("Kubeconfig saved to: %s\n", kubeconfigPath)
		fmt.Printf("\nYou can now access your cluster with:\n")
		fmt.Printf("  export KUBECONFIG=%s\n", kubeconfigPath)
		fmt.Printf("  kubectl get nodes\n")
	} else {
		fmt.Printf("\nNote: Cluster was already bootstrapped. To retrieve kubeconfig, use talosctl:\n")
		fmt.Printf("  talosctl --talosconfig %s kubeconfig\n", talosConfigPath)
	}

	// Print Cilium encryption info if Cilium is enabled
	printCiliumEncryptionInfo(cfg)
}

// printCiliumEncryptionInfo outputs Cilium encryption settings.
// Matches terraform/outputs.tf cilium_encryption_info output.
func printCiliumEncryptionInfo(cfg *config.Config) {
	if !cfg.Addons.Cilium.Enabled {
		return
	}

	cilium := cfg.Addons.Cilium
	if !cilium.EncryptionEnabled {
		fmt.Printf("\nCilium encryption: disabled\n")
		return
	}

	fmt.Printf("\nCilium encryption info:\n")
	fmt.Printf("  Enabled: %t\n", cilium.EncryptionEnabled)
	fmt.Printf("  Type: %s\n", cilium.EncryptionType)

	if cilium.EncryptionType == "ipsec" {
		fmt.Printf("  IPsec settings:\n")
		fmt.Printf("    Algorithm: %s\n", cilium.IPSecAlgorithm)
		fmt.Printf("    Key size (bits): %d\n", cilium.IPSecKeySize)
		fmt.Printf("    Key ID: %d\n", cilium.IPSecKeyID)
		fmt.Printf("    Secret name: cilium-ipsec-keys\n")
		fmt.Printf("    Namespace: kube-system\n")
	}
}

// checkPrerequisites verifies required client tools are available.
// Enabled by default, can be disabled via prerequisites_check_enabled: false.
func checkPrerequisites(cfg *config.Config) error {
	// Default to enabled if not explicitly set
	enabled := cfg.PrerequisitesCheckEnabled == nil || *cfg.PrerequisitesCheckEnabled

	if !enabled {
		return nil
	}

	log.Println("Checking prerequisites...")
	results := checkDefaultPrereqs()

	// Log found tools
	for _, r := range results.Results {
		if r.Found {
			version := r.Version
			if version == "" {
				version = "unknown version"
			}
			log.Printf("  Found %s (%s)", r.Tool.Name, version)
		}
	}

	// Return error if required tools are missing
	if err := results.Error(); err != nil {
		return fmt.Errorf("prerequisites check failed: %w", err)
	}

	return nil
}
