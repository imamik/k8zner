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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/config"
	v2config "github.com/imamik/k8zner/internal/config/v2"
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

	// loadV2ConfigFile loads v2 config from file (for testing injection).
	loadV2ConfigFile = v2config.Load

	// expandV2Config expands v2 config to internal format (for testing injection).
	expandV2Config = v2config.Expand

	// findV2ConfigFile finds the v2 config file (for testing injection).
	findV2ConfigFile = v2config.FindConfigFile
)

// Apply provisions a Kubernetes cluster on Hetzner Cloud using Talos Linux.
//
// This function orchestrates the complete cluster provisioning workflow:
//  1. Loads and validates cluster configuration (auto-detects v2 or legacy format)
//  2. Checks if cluster is operator-managed (K8znerCluster CRD exists)
//  3. If operator-managed: updates CRD spec and lets operator reconcile
//  4. If not operator-managed: runs full CLI provisioning flow
//
// For CLI provisioning:
//  1. Initializes Hetzner Cloud client using HCLOUD_TOKEN environment variable
//  2. Generates Talos machine configurations and persists secrets immediately
//  3. Reconciles cluster infrastructure (networks, servers, load balancers, bootstrap)
//  4. Writes kubeconfig if cluster bootstrap completed successfully
//  5. Installs configured cluster addons (CCM, CSI, etc.) if bootstrap succeeded
//
// Secrets and Talos config are written before reconciliation to ensure they're
// preserved even if reconciliation fails, enabling retry without data loss.
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

	// Check if this cluster is operator-managed
	if isOperatorManaged, err := checkOperatorManaged(ctx, cfg.ClusterName); err == nil && isOperatorManaged {
		log.Printf("Cluster %s is operator-managed, updating CRD spec", cfg.ClusterName)
		return applyOperatorMode(ctx, cfg)
	}

	// Not operator-managed, run full CLI provisioning
	return applyCLIMode(ctx, cfg)
}

// applyCLIMode runs the traditional full CLI provisioning workflow.
func applyCLIMode(ctx context.Context, cfg *config.Config) error {
	infraClient := initializeClient()
	talosGen, err := initializeTalosGenerator(cfg)
	if err != nil {
		return err
	}

	if err := writeTalosFiles(talosGen); err != nil {
		return err
	}

	kubeconfig, err := reconcileInfrastructure(ctx, infraClient, talosGen, cfg)
	if err != nil {
		return err
	}

	if err := writeKubeconfig(kubeconfig); err != nil {
		return err
	}

	printApplySuccess(kubeconfig, cfg)
	return nil
}

// applyOperatorMode updates the K8znerCluster CRD spec for operator reconciliation.
func applyOperatorMode(ctx context.Context, cfg *config.Config) error {
	// Load kubeconfig to connect to the cluster
	kubecfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Create controller-runtime client
	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(kubecfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Get existing K8znerCluster CRD
	cluster := &k8znerv1alpha1.K8znerCluster{}
	key := client.ObjectKey{
		Namespace: k8znerNamespace,
		Name:      cfg.ClusterName,
	}

	if err := k8sClient.Get(ctx, key, cluster); err != nil {
		return fmt.Errorf("failed to get K8znerCluster: %w", err)
	}

	// Update spec from config
	updateClusterSpecFromConfig(cluster, cfg)

	// Apply the update
	if err := k8sClient.Update(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update K8znerCluster: %w", err)
	}

	log.Printf("Updated K8znerCluster %s spec", cfg.ClusterName)
	log.Printf("\nThe operator will now reconcile the changes.")
	log.Printf("Monitor progress with:")
	log.Printf("  kubectl logs -f -n %s deploy/k8zner-operator", k8znerNamespace)
	log.Printf("  kubectl get k8znerclusters -n %s -w", k8znerNamespace)

	return nil
}

// checkOperatorManaged checks if a cluster is managed by the operator.
// Returns true if K8znerCluster CRD exists for the cluster.
func checkOperatorManaged(ctx context.Context, clusterName string) (bool, error) {
	// Check if kubeconfig exists
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return false, nil
	}

	// Try to load kubeconfig
	kubecfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return false, err
	}

	// Create controller-runtime client
	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(kubecfg, client.Options{Scheme: scheme})
	if err != nil {
		return false, err
	}

	// Check if K8znerCluster CRD exists
	cluster := &k8znerv1alpha1.K8znerCluster{}
	key := client.ObjectKey{
		Namespace: k8znerNamespace,
		Name:      clusterName,
	}

	if err := k8sClient.Get(ctx, key, cluster); err != nil {
		return false, nil // CRD doesn't exist or not accessible
	}

	// Check if cluster has credentials reference (indicates operator management)
	return cluster.Spec.CredentialsRef.Name != "", nil
}

// updateClusterSpecFromConfig updates the K8znerCluster spec from config.Config.
func updateClusterSpecFromConfig(cluster *k8znerv1alpha1.K8znerCluster, cfg *config.Config) {
	// Update control plane settings
	if len(cfg.ControlPlane.NodePools) > 0 {
		pool := cfg.ControlPlane.NodePools[0]
		cluster.Spec.ControlPlanes.Count = pool.Count
		cluster.Spec.ControlPlanes.Size = pool.ServerType
	}

	// Update worker settings
	if len(cfg.Workers) > 0 {
		pool := cfg.Workers[0]
		cluster.Spec.Workers.Count = pool.Count
		cluster.Spec.Workers.Size = pool.ServerType
	}

	// Update Talos settings
	cluster.Spec.Talos.Version = cfg.Talos.Version
	cluster.Spec.Talos.SchematicID = cfg.Talos.SchematicID
	cluster.Spec.Talos.Extensions = cfg.Talos.Extensions

	// Update Kubernetes version
	cluster.Spec.Kubernetes.Version = cfg.Kubernetes.Version

	// Update network settings
	cluster.Spec.Network.IPv4CIDR = cfg.Network.IPv4CIDR
	cluster.Spec.Network.PodCIDR = cfg.Network.PodIPv4CIDR
	cluster.Spec.Network.ServiceCIDR = cfg.Network.ServiceIPv4CIDR

	// Update addons if specified
	if cluster.Spec.Addons == nil {
		cluster.Spec.Addons = &k8znerv1alpha1.AddonSpec{}
	}
	cluster.Spec.Addons.MetricsServer = cfg.Addons.MetricsServer.Enabled
	cluster.Spec.Addons.CertManager = cfg.Addons.CertManager.Enabled
	cluster.Spec.Addons.Traefik = cfg.Addons.Traefik.Enabled
	cluster.Spec.Addons.ArgoCD = cfg.Addons.ArgoCD.Enabled

	// Set last modified annotation
	if cluster.Annotations == nil {
		cluster.Annotations = make(map[string]string)
	}
	cluster.Annotations["k8zner.io/last-applied"] = metav1.Now().Format(metav1.RFC3339Micro)
}

// loadConfig loads and validates cluster configuration.
// If configPath is empty, it looks for k8zner.yaml in the current directory.
func loadConfig(configPath string) (*config.Config, error) {
	// If no path provided, try to find default config
	if configPath == "" {
		path, err := findV2ConfigFile()
		if err != nil {
			return nil, fmt.Errorf("no config file found: %w\nRun 'k8zner init' to create one", err)
		}
		configPath = path
	}

	// Load config and expand to internal format
	v2Cfg, err := loadV2ConfigFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config %s: %w", configPath, err)
	}

	log.Printf("Using config: %s", configPath)
	return expandV2Config(v2Cfg)
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
func reconcileInfrastructure(ctx context.Context, hclient hcloud.InfrastructureManager, talosGen provisioning.TalosConfigProducer, cfg *config.Config) ([]byte, error) {
	log.Println("Starting infrastructure reconciliation...")

	reconciler := newReconciler(hclient, talosGen, cfg)
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
