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
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
	v2config "github.com/imamik/k8zner/internal/config/v2"
	hcloudInternal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/platform/talos"
	"github.com/imamik/k8zner/internal/provisioning"
	clusterProv "github.com/imamik/k8zner/internal/provisioning/cluster"
	"github.com/imamik/k8zner/internal/provisioning/compute"
	"github.com/imamik/k8zner/internal/provisioning/destroy"
	"github.com/imamik/k8zner/internal/provisioning/image"
	"github.com/imamik/k8zner/internal/provisioning/infrastructure"
	"github.com/imamik/k8zner/internal/util/naming"
)

const (
	secretsFile     = "secrets.yaml"
	talosConfigPath = "talosconfig"
	kubeconfigPath  = "kubeconfig"

	// k8znerNamespace is the Kubernetes namespace for k8zner resources.
	k8znerNamespace = "k8zner-system"

	// credentialsSecretName is the name of the secret containing Hetzner and Talos credentials.
	credentialsSecretName = "k8zner-credentials" //nolint:gosec // This is a secret name, not a credential value

	// operatorPollInterval is the interval between status checks when waiting for the operator.
	operatorPollInterval = 10 * time.Second

	// operatorWaitTimeout is the maximum time to wait for the operator to complete provisioning.
	operatorWaitTimeout = 30 * time.Minute
)

// InfrastructureInfo contains information about created infrastructure.
type InfrastructureInfo struct {
	NetworkID             int64
	NetworkName           string
	FirewallID            int64
	FirewallName          string
	LoadBalancerID        int64
	LoadBalancerName      string
	LoadBalancerIP        string
	LoadBalancerPrivateIP string
	SSHKeyID              int64
}

// Provisioner interface for testing - matches provisioning.Phase.
type Provisioner interface {
	Provision(ctx *provisioning.Context) error
}

// Factory function variables - can be replaced in tests for dependency injection.
var (
	// newInfraClient creates a new infrastructure client.
	newInfraClient = func(token string) hcloudInternal.InfrastructureManager {
		return hcloudInternal.NewRealClient(token)
	}

	// getOrGenerateSecrets loads or generates Talos secrets.
	getOrGenerateSecrets = talos.GetOrGenerateSecrets

	// newTalosGenerator creates a new Talos configuration generator.
	newTalosGenerator = func(clusterName, kubernetesVersion, talosVersion, endpoint string, sb *secrets.Bundle) provisioning.TalosConfigProducer {
		return talos.NewGenerator(clusterName, kubernetesVersion, talosVersion, endpoint, sb)
	}

	// writeFile writes data to a file (for testing injection).
	writeFile = os.WriteFile

	// loadV2ConfigFile loads v2 config from file (for testing injection).
	loadV2ConfigFile = v2config.Load

	// expandV2Config expands v2 config to internal format (for testing injection).
	expandV2Config = v2config.Expand

	// findV2ConfigFile finds the v2 config file (for testing injection).
	findV2ConfigFile = v2config.FindConfigFile

	// Factory functions for provisioners - can be replaced in tests.
	newInfraProvisioner    = infrastructure.NewProvisioner
	newImageProvisioner    = image.NewProvisioner
	newComputeProvisioner  = compute.NewProvisioner
	newClusterProvisioner  = clusterProv.NewProvisioner
	newDestroyProvisioner  = func() Provisioner { return destroy.NewProvisioner() }
	newProvisioningContext = provisioning.NewContext
)

// Apply creates or updates a Kubernetes cluster on Hetzner Cloud using Talos Linux.
//
// This function implements a single idempotent code path:
//  1. Loads and validates cluster configuration
//  2. Checks if cluster already exists (kubeconfig + CRD present)
//  3. If CRD exists: updates CRD spec, operator reconciles
//  4. If no cluster: bootstraps (infra -> 1 CP -> operator -> CRD)
//  5. Optionally waits for operator to reach Running state
func Apply(ctx context.Context, configPath string, wait bool) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	log.Printf("Applying configuration for cluster: %s", cfg.ClusterName)

	// Check if cluster already exists with operator management
	if isOperatorManaged, err := checkOperatorManaged(ctx, cfg.ClusterName); err == nil && isOperatorManaged {
		log.Printf("Cluster %s is operator-managed, updating CRD spec", cfg.ClusterName)
		return applyUpdate(ctx, cfg)
	}

	// No existing cluster — bootstrap from scratch
	return applyBootstrap(ctx, cfg, wait)
}

// applyUpdate updates an existing operator-managed cluster's CRD spec.
func applyUpdate(ctx context.Context, cfg *config.Config) error {
	kubecfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(kubecfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	k8zCluster := &k8znerv1alpha1.K8znerCluster{}
	key := client.ObjectKey{
		Namespace: k8znerNamespace,
		Name:      cfg.ClusterName,
	}

	if err := k8sClient.Get(ctx, key, k8zCluster); err != nil {
		return fmt.Errorf("failed to get K8znerCluster: %w", err)
	}

	updateClusterSpecFromConfig(k8zCluster, cfg)

	if err := k8sClient.Update(ctx, k8zCluster); err != nil {
		return fmt.Errorf("failed to update K8znerCluster: %w", err)
	}

	log.Printf("Updated K8znerCluster %s spec", cfg.ClusterName)
	log.Printf("\nThe operator will now reconcile the changes.")
	log.Printf("Monitor progress with:")
	log.Printf("  k8zner doctor --watch")

	return nil
}

// applyBootstrap creates a new cluster from scratch.
//
// Flow: Image -> Infrastructure -> 1 CP -> Bootstrap -> Install operator -> Create CRD
func applyBootstrap(ctx context.Context, cfg *config.Config, wait bool) error {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		return fmt.Errorf("HCLOUD_TOKEN environment variable is required")
	}
	infraClient := newInfraClient(token)

	talosGen, err := initializeTalosGenerator(cfg)
	if err != nil {
		return err
	}

	talosGen.SetMachineConfigOptions(talos.NewMachineConfigOptions(cfg))

	if err := writeTalosFiles(talosGen); err != nil {
		return err
	}

	pCtx := newProvisioningContext(ctx, cfg, infraClient, talosGen)

	// Track whether cleanup is needed on failure
	var cleanupNeeded bool
	var applyErr error
	defer func() {
		if applyErr != nil && cleanupNeeded {
			log.Println("Apply failed, cleaning up created resources...")
			cleanupCtx := context.Background()
			if cleanupErr := cleanupOnFailure(cleanupCtx, cfg, infraClient); cleanupErr != nil {
				log.Printf("Warning: cleanup failed: %v", cleanupErr)
			}
		}
	}()

	// Phase 1: Image
	log.Println("Phase 1/6: Ensuring Talos image snapshot...")
	imgProvisioner := newImageProvisioner()
	if err := imgProvisioner.Provision(pCtx); err != nil {
		applyErr = fmt.Errorf("image provisioning failed: %w", err)
		return applyErr
	}

	// Phase 2: Infrastructure
	log.Println("Phase 2/6: Creating infrastructure (network, firewall, LB, placement group)...")
	infraProvisioner := newInfraProvisioner()
	if err := infraProvisioner.Provision(pCtx); err != nil {
		applyErr = fmt.Errorf("infrastructure provisioning failed: %w", err)
		return applyErr
	}
	cleanupNeeded = true

	// Phase 3: First Control Plane
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
		applyErr = fmt.Errorf("compute provisioning failed: %w", err)
		return applyErr
	}

	cfg.ControlPlane.NodePools[0].Count = originalCPCount
	if len(cfg.Workers) > 0 {
		cfg.Workers[0].Count = originalWorkerCount
	}

	// Phase 4: Bootstrap
	log.Println("Phase 4/6: Bootstrapping cluster...")
	clstrProvisioner := newClusterProvisioner()
	if err := clstrProvisioner.Provision(pCtx); err != nil {
		applyErr = fmt.Errorf("cluster bootstrap failed: %w", err)
		return applyErr
	}

	kubeconfig := pCtx.State.Kubeconfig
	if len(kubeconfig) == 0 {
		applyErr = fmt.Errorf("kubeconfig not available after cluster bootstrap")
		return applyErr
	}
	if err := writeKubeconfig(kubeconfig); err != nil {
		applyErr = err
		return applyErr
	}

	// Phase 5: Install Operator
	log.Println("Phase 5/6: Installing k8zner operator...")
	cfg.Addons.Operator.Enabled = true
	cfg.Addons.Operator.HostNetwork = true

	if err := installOperatorOnly(ctx, cfg, kubeconfig, pCtx.State.Network.ID); err != nil {
		applyErr = fmt.Errorf("operator installation failed: %w", err)
		return applyErr
	}

	// Gather infrastructure info for CRD
	infraInfo := buildInfraInfo(ctx, pCtx, infraClient, cfg)

	// Phase 6: Create CRD
	log.Println("Phase 6/6: Creating K8znerCluster CRD...")
	if err := createClusterCRD(ctx, cfg, pCtx, infraInfo, kubeconfig, token); err != nil {
		return fmt.Errorf("CRD creation failed: %w", err)
	}

	printApplySuccess(cfg, wait)

	if wait {
		return waitForOperatorComplete(ctx, cfg.ClusterName, kubeconfig)
	}

	return nil
}

// checkOperatorManaged checks if a cluster is managed by the operator.
func checkOperatorManaged(ctx context.Context, clusterName string) (bool, error) {
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return false, nil
	}

	kubecfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return false, err
	}

	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(kubecfg, client.Options{Scheme: scheme})
	if err != nil {
		return false, err
	}

	k8zCluster := &k8znerv1alpha1.K8znerCluster{}
	key := client.ObjectKey{
		Namespace: k8znerNamespace,
		Name:      clusterName,
	}

	if err := k8sClient.Get(ctx, key, k8zCluster); err != nil {
		return false, nil
	}

	return k8zCluster.Spec.CredentialsRef.Name != "", nil
}

// updateClusterSpecFromConfig updates the K8znerCluster spec from config.Config.
func updateClusterSpecFromConfig(k8zCluster *k8znerv1alpha1.K8znerCluster, cfg *config.Config) {
	if len(cfg.ControlPlane.NodePools) > 0 {
		pool := cfg.ControlPlane.NodePools[0]
		k8zCluster.Spec.ControlPlanes.Count = pool.Count
		k8zCluster.Spec.ControlPlanes.Size = pool.ServerType
	}

	if len(cfg.Workers) > 0 {
		pool := cfg.Workers[0]
		k8zCluster.Spec.Workers.Count = pool.Count
		k8zCluster.Spec.Workers.Size = pool.ServerType
	}

	k8zCluster.Spec.Talos.Version = cfg.Talos.Version
	k8zCluster.Spec.Talos.SchematicID = cfg.Talos.SchematicID
	k8zCluster.Spec.Talos.Extensions = cfg.Talos.Extensions
	k8zCluster.Spec.Kubernetes.Version = cfg.Kubernetes.Version

	k8zCluster.Spec.Network.IPv4CIDR = cfg.Network.IPv4CIDR
	k8zCluster.Spec.Network.PodCIDR = cfg.Network.PodIPv4CIDR
	k8zCluster.Spec.Network.ServiceCIDR = cfg.Network.ServiceIPv4CIDR

	if k8zCluster.Spec.Addons == nil {
		k8zCluster.Spec.Addons = &k8znerv1alpha1.AddonSpec{}
	}
	k8zCluster.Spec.Addons.MetricsServer = cfg.Addons.MetricsServer.Enabled
	k8zCluster.Spec.Addons.CertManager = cfg.Addons.CertManager.Enabled
	k8zCluster.Spec.Addons.Traefik = cfg.Addons.Traefik.Enabled
	k8zCluster.Spec.Addons.ArgoCD = cfg.Addons.ArgoCD.Enabled
	k8zCluster.Spec.Addons.Monitoring = cfg.Addons.KubePrometheusStack.Enabled

	if cfg.Addons.TalosBackup.Enabled && cfg.Addons.TalosBackup.S3AccessKey != "" {
		if k8zCluster.Spec.Backup == nil {
			k8zCluster.Spec.Backup = &k8znerv1alpha1.BackupSpec{}
		}
		k8zCluster.Spec.Backup.Enabled = true
		k8zCluster.Spec.Backup.Schedule = cfg.Addons.TalosBackup.Schedule
		if k8zCluster.Spec.Backup.S3SecretRef == nil {
			k8zCluster.Spec.Backup.S3SecretRef = &k8znerv1alpha1.SecretReference{
				Name: k8zCluster.Name + "-backup-s3",
			}
		}
	}

	if k8zCluster.Annotations == nil {
		k8zCluster.Annotations = make(map[string]string)
	}
	k8zCluster.Annotations["k8zner.io/last-applied"] = metav1.Now().Format(metav1.RFC3339Micro)
}

// loadConfig loads and validates cluster configuration.
func loadConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		path, err := findV2ConfigFile()
		if err != nil {
			return nil, fmt.Errorf("no config file found: %w\nRun 'k8zner init' to create one", err)
		}
		configPath = path
	}

	v2Cfg, err := loadV2ConfigFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config %s: %w", configPath, err)
	}

	log.Printf("Using config: %s", configPath)
	return expandV2Config(v2Cfg)
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

// buildInfraInfo gathers infrastructure details from provisioning state and API.
func buildInfraInfo(ctx context.Context, pCtx *provisioning.Context, infraClient hcloudInternal.InfrastructureManager, cfg *config.Config) *InfrastructureInfo {
	lb := pCtx.State.LoadBalancer
	if lb == nil {
		lbName := naming.KubeAPILoadBalancer(cfg.ClusterName)
		var err error
		lb, err = infraClient.GetLoadBalancer(ctx, lbName)
		if err != nil {
			log.Printf("Warning: failed to get load balancer info: %v", err)
		}
	}

	info := &InfrastructureInfo{
		NetworkID:   pCtx.State.Network.ID,
		NetworkName: pCtx.State.Network.Name,
		SSHKeyID:    pCtx.State.SSHKeyID,
	}
	if pCtx.State.Firewall != nil {
		info.FirewallID = pCtx.State.Firewall.ID
		info.FirewallName = pCtx.State.Firewall.Name
	}
	if lb != nil {
		info.LoadBalancerID = lb.ID
		info.LoadBalancerName = lb.Name
		info.LoadBalancerIP = hcloudInternal.LoadBalancerIPv4(lb)
		info.LoadBalancerPrivateIP = hcloudInternal.LoadBalancerPrivateIP(lb)
	}

	return info
}

// createClusterCRD creates the K8znerCluster CRD and credentials Secret.
func createClusterCRD(ctx context.Context, cfg *config.Config, pCtx *provisioning.Context, infraInfo *InfrastructureInfo, kubeconfig []byte, hcloudToken string) error {
	kubecfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(kubecfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8znerNamespace,
		},
	}
	if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	secretsData, err := os.ReadFile(secretsFile)
	if err != nil {
		return fmt.Errorf("failed to read secrets.yaml: %w", err)
	}
	talosConfigData, err := os.ReadFile(talosConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read talosconfig: %w", err)
	}

	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      credentialsSecretName,
			Namespace: k8znerNamespace,
			Labels: map[string]string{
				"cluster": cfg.ClusterName,
			},
		},
		Data: map[string][]byte{
			k8znerv1alpha1.CredentialsKeyHCloudToken:  []byte(hcloudToken),
			k8znerv1alpha1.CredentialsKeyTalosSecrets: secretsData,
			k8znerv1alpha1.CredentialsKeyTalosConfig:  talosConfigData,
		},
	}
	if cfg.Addons.Cloudflare.APIToken != "" {
		credSecret.Data[k8znerv1alpha1.CredentialsKeyCloudflareAPIToken] = []byte(cfg.Addons.Cloudflare.APIToken)
	}
	if err := k8sClient.Create(ctx, credSecret); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create credentials secret: %w", err)
	}

	if backupSecret := createBackupS3Secret(cfg, cfg.ClusterName); backupSecret != nil {
		if err := k8sClient.Create(ctx, backupSecret); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create backup S3 secret: %w", err)
		}
		log.Printf("Created backup S3 secret: %s", backupSecret.Name)
	}

	bootstrapName, bootstrapID, bootstrapIP := getBootstrapNode(pCtx)
	k8znerCluster := buildK8znerCluster(cfg, infraInfo, bootstrapName, bootstrapID, bootstrapIP)
	if err := k8sClient.Create(ctx, k8znerCluster); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create K8znerCluster: %w", err)
	}

	// Status is a subresource — update it separately
	createdCluster := &k8znerv1alpha1.K8znerCluster{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: k8znerCluster.Name, Namespace: k8znerCluster.Namespace}, createdCluster); err != nil {
		return fmt.Errorf("failed to get created K8znerCluster: %w", err)
	}

	createdCluster.Status = k8znerCluster.Status
	if err := k8sClient.Status().Update(ctx, createdCluster); err != nil {
		return fmt.Errorf("failed to update K8znerCluster status: %w", err)
	}

	return nil
}

// buildK8znerCluster builds the K8znerCluster CRD from config.
func buildK8znerCluster(cfg *config.Config, infraInfo *InfrastructureInfo, bootstrapName string, bootstrapID int64, bootstrapIP string) *k8znerv1alpha1.K8znerCluster {
	now := metav1.Now()

	k8znerCluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.ClusterName,
			Namespace: k8znerNamespace,
			Labels: map[string]string{
				"cluster": cfg.ClusterName,
			},
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region: cfg.Location,
			Domain: cfg.Addons.Cloudflare.Domain,
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
				Count: cfg.ControlPlane.NodePools[0].Count,
				Size:  cfg.ControlPlane.NodePools[0].ServerType,
			},
			Workers: k8znerv1alpha1.WorkerSpec{
				Count: getWorkerCount(cfg),
				Size:  getWorkerSize(cfg),
			},
			Network: k8znerv1alpha1.NetworkSpec{
				IPv4CIDR:     cfg.Network.IPv4CIDR,
				NodeIPv4CIDR: cfg.Network.NodeIPv4CIDR,
				PodCIDR:      cfg.Network.PodIPv4CIDR,
				ServiceCIDR:  cfg.Network.ServiceIPv4CIDR,
			},
			Firewall: k8znerv1alpha1.FirewallSpec{
				Enabled: true,
			},
			Kubernetes: k8znerv1alpha1.KubernetesSpec{
				Version: cfg.Kubernetes.Version,
			},
			Talos: k8znerv1alpha1.TalosSpec{
				Version:     cfg.Talos.Version,
				SchematicID: cfg.Talos.SchematicID,
				Extensions:  cfg.Talos.Extensions,
			},
			CredentialsRef: corev1.LocalObjectReference{
				Name: credentialsSecretName,
			},
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed:       true,
				CompletedAt:     &now,
				BootstrapNode:   bootstrapName,
				BootstrapNodeID: bootstrapID,
				PublicIP:        bootstrapIP,
			},
			Addons: buildAddonSpec(cfg),
			Backup: buildBackupSpec(cfg, cfg.ClusterName),
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Phase:             k8znerv1alpha1.ClusterPhaseProvisioning,
			ProvisioningPhase: k8znerv1alpha1.PhaseCNI,
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Desired: cfg.ControlPlane.NodePools[0].Count,
				Ready:   1,
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:     bootstrapName,
						ServerID: bootstrapID,
						PublicIP: bootstrapIP,
						Healthy:  true,
					},
				},
			},
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Desired: getWorkerCount(cfg),
			},
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID:             infraInfo.NetworkID,
				FirewallID:            infraInfo.FirewallID,
				LoadBalancerID:        infraInfo.LoadBalancerID,
				LoadBalancerIP:        infraInfo.LoadBalancerIP,
				LoadBalancerPrivateIP: infraInfo.LoadBalancerPrivateIP,
				SSHKeyID:              infraInfo.SSHKeyID,
			},
			ControlPlaneEndpoint: infraInfo.LoadBalancerIP,
		},
	}

	k8znerCluster.Spec.PlacementGroup = &k8znerv1alpha1.PlacementGroupSpec{
		Enabled: true,
		Type:    "spread",
	}

	return k8znerCluster
}

// buildAddonSpec creates the addon spec from config.
func buildAddonSpec(cfg *config.Config) *k8znerv1alpha1.AddonSpec {
	spec := &k8znerv1alpha1.AddonSpec{
		Traefik:       cfg.Addons.Traefik.Enabled,
		CertManager:   cfg.Addons.CertManager.Enabled,
		ExternalDNS:   cfg.Addons.ExternalDNS.Enabled,
		ArgoCD:        cfg.Addons.ArgoCD.Enabled,
		MetricsServer: cfg.Addons.MetricsServer.Enabled,
		Monitoring:    cfg.Addons.KubePrometheusStack.Enabled,
	}

	domain := cfg.Addons.Cloudflare.Domain
	if domain != "" {
		suffix := "." + domain
		if host := cfg.Addons.ArgoCD.IngressHost; host != "" && strings.HasSuffix(host, suffix) {
			sub := strings.TrimSuffix(host, suffix)
			if sub != "argo" {
				spec.ArgoSubdomain = sub
			}
		}
		if host := cfg.Addons.KubePrometheusStack.Grafana.IngressHost; host != "" && strings.HasSuffix(host, suffix) {
			sub := strings.TrimSuffix(host, suffix)
			if sub != "grafana" {
				spec.GrafanaSubdomain = sub
			}
		}
	}

	return spec
}

// buildBackupSpec creates the backup spec from config.
func buildBackupSpec(cfg *config.Config, clusterName string) *k8znerv1alpha1.BackupSpec {
	if !cfg.Addons.TalosBackup.Enabled {
		return nil
	}
	if cfg.Addons.TalosBackup.S3AccessKey == "" || cfg.Addons.TalosBackup.S3SecretKey == "" {
		return nil
	}

	return &k8znerv1alpha1.BackupSpec{
		Enabled:   true,
		Schedule:  cfg.Addons.TalosBackup.Schedule,
		Retention: "168h",
		S3SecretRef: &k8znerv1alpha1.SecretReference{
			Name: backupS3SecretName(clusterName),
		},
	}
}

func backupS3SecretName(clusterName string) string {
	return clusterName + "-backup-s3"
}

// createBackupS3Secret creates the Secret containing S3 credentials for backup.
func createBackupS3Secret(cfg *config.Config, clusterName string) *corev1.Secret {
	if !cfg.Addons.TalosBackup.Enabled {
		return nil
	}
	if cfg.Addons.TalosBackup.S3AccessKey == "" || cfg.Addons.TalosBackup.S3SecretKey == "" {
		return nil
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupS3SecretName(clusterName),
			Namespace: k8znerNamespace,
			Labels: map[string]string{
				"cluster": clusterName,
				"purpose": "backup",
			},
		},
		StringData: map[string]string{
			"access-key": cfg.Addons.TalosBackup.S3AccessKey,
			"secret-key": cfg.Addons.TalosBackup.S3SecretKey,
			"endpoint":   cfg.Addons.TalosBackup.S3Endpoint,
			"bucket":     cfg.Addons.TalosBackup.S3Bucket,
			"region":     cfg.Addons.TalosBackup.S3Region,
		},
	}
}

func getWorkerCount(cfg *config.Config) int {
	if len(cfg.Workers) == 0 {
		return 0
	}
	return cfg.Workers[0].Count
}

func getWorkerSize(cfg *config.Config) string {
	if len(cfg.Workers) == 0 {
		return v2config.DefaultWorkerServerType
	}
	return cfg.Workers[0].ServerType
}

// waitForOperatorComplete waits for the operator to complete provisioning.
func waitForOperatorComplete(ctx context.Context, clusterName string, kubeconfig []byte) error {
	log.Println("Waiting for operator to complete provisioning...")

	kubecfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(kubecfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ticker := time.NewTicker(operatorPollInterval)
	defer ticker.Stop()

	deadline := time.Now().Add(operatorWaitTimeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for operator to complete after %v", operatorWaitTimeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		getCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		k8zCluster := &k8znerv1alpha1.K8znerCluster{}
		key := client.ObjectKey{
			Namespace: k8znerNamespace,
			Name:      clusterName,
		}

		err := k8sClient.Get(getCtx, key, k8zCluster)
		cancel()

		if err != nil {
			log.Printf("Warning: failed to get cluster status: %v", err)
			continue
		}

		phase := k8zCluster.Status.ProvisioningPhase
		clusterPhase := k8zCluster.Status.Phase

		log.Printf("Status: %s (phase: %s)", clusterPhase, phase)

		if phase == k8znerv1alpha1.PhaseComplete && clusterPhase == k8znerv1alpha1.ClusterPhaseRunning {
			log.Println("Cluster provisioning complete!")
			return nil
		}
	}
}

// getBootstrapNode returns the bootstrap node info from the provisioning state.
func getBootstrapNode(pCtx *provisioning.Context) (name string, serverID int64, ip string) {
	if len(pCtx.State.ControlPlaneIPs) == 0 {
		return "", 0, ""
	}

	names := make([]string, 0, len(pCtx.State.ControlPlaneIPs))
	for n := range pCtx.State.ControlPlaneIPs {
		names = append(names, n)
	}
	sort.Strings(names)

	name = names[0]
	ip = pCtx.State.ControlPlaneIPs[name]
	serverID = pCtx.State.ControlPlaneServerIDs[name]
	return name, serverID, ip
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
