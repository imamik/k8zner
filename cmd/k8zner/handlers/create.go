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

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
	configv2 "github.com/imamik/k8zner/internal/config/v2"
	hcloudInternal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/provisioning/cluster"
	"github.com/imamik/k8zner/internal/provisioning/compute"
	"github.com/imamik/k8zner/internal/provisioning/destroy"
	"github.com/imamik/k8zner/internal/provisioning/image"
	"github.com/imamik/k8zner/internal/provisioning/infrastructure"
	"github.com/imamik/k8zner/internal/util/naming"
)

const (
	// credentialsSecretName is the name of the secret containing Hetzner and Talos credentials.
	credentialsSecretName = "k8zner-credentials" //nolint:gosec // This is a secret name, not a credential value

	// operatorPollInterval is the interval between status checks when waiting for the operator.
	operatorPollInterval = 10 * time.Second

	// operatorWaitTimeout is the maximum time to wait for the operator to complete provisioning.
	operatorWaitTimeout = 30 * time.Minute
)

// Factory function variables for create - can be replaced in tests.
var (
	// newInfraProvisionerForCreate creates a new infrastructure provisioner.
	newInfraProvisionerForCreate = infrastructure.NewProvisioner

	// newImageProvisionerForCreate creates a new image provisioner.
	newImageProvisionerForCreate = image.NewProvisioner

	// newComputeProvisionerForCreate creates a new compute provisioner.
	newComputeProvisionerForCreate = compute.NewProvisioner

	// newClusterProvisionerForCreate creates a new cluster provisioner.
	newClusterProvisionerForCreate = cluster.NewProvisioner
)

// Create handles the create command.
//
// This function bootstraps a new cluster with operator management:
//  1. Loads and validates cluster configuration
//  2. Creates Talos snapshot (if needed)
//  3. Creates load balancer (stable API endpoint)
//  4. Creates network and subnets
//  5. Creates placement group
//  6. Creates firewall
//  7. Generates Talos secrets with LB IP as endpoint
//  8. Creates first control plane server
//  9. Applies Talos config and bootstraps etcd
//
// 10. Waits for Kubernetes API ready
// 11. Installs operator (with hostNetwork: true)
// 12. Creates K8znerCluster CRD
// 13. Shows progress and exits
func Create(ctx context.Context, configPath string, wait bool) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	// Run prerequisites check
	if err := checkPrerequisites(cfg); err != nil {
		return err
	}

	log.Printf("Creating cluster: %s", cfg.ClusterName)

	// Initialize clients
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		return fmt.Errorf("HCLOUD_TOKEN environment variable is required")
	}
	infraClient := newInfraClient(token)

	// Initialize Talos generator
	talosGen, err := initializeTalosGenerator(cfg)
	if err != nil {
		return err
	}

	// Write Talos config and secrets early
	if err := writeTalosFiles(talosGen); err != nil {
		return err
	}

	// Create provisioning context
	pCtx := newProvisioningContext(ctx, cfg, infraClient, talosGen)

	// Track whether cleanup is needed on failure
	var cleanupNeeded bool
	var createErr error
	defer func() {
		if createErr != nil && cleanupNeeded {
			log.Println("Create failed, cleaning up created resources...")
			cleanupCtx := context.Background() // Use fresh context for cleanup
			if cleanupErr := cleanupOnFailure(cleanupCtx, cfg, infraClient); cleanupErr != nil {
				log.Printf("Warning: cleanup failed: %v", cleanupErr)
			}
		}
	}()

	// Phase 1: Image
	log.Println("Phase 1: Ensuring Talos image snapshot...")
	imageProvisioner := newImageProvisionerForCreate()
	if err := imageProvisioner.Provision(pCtx); err != nil {
		createErr = fmt.Errorf("image provisioning failed: %w", err)
		return createErr
	}

	// Phase 2: Infrastructure
	log.Println("Phase 2: Creating infrastructure (network, firewall, LB, placement group)...")
	infraProvisioner := newInfraProvisionerForCreate()
	if err := infraProvisioner.Provision(pCtx); err != nil {
		createErr = fmt.Errorf("infrastructure provisioning failed: %w", err)
		return createErr
	}
	// Mark cleanup needed from here on - infrastructure has been created
	cleanupNeeded = true

	// Phase 3: First Control Plane
	log.Println("Phase 3: Creating first control plane...")
	// Set count to 1 for bootstrap (operator will create remaining)
	originalCPCount := cfg.ControlPlane.NodePools[0].Count
	cfg.ControlPlane.NodePools[0].Count = 1
	// Set worker count to 0 for bootstrap (operator will create workers)
	originalWorkerCount := 0
	if len(cfg.Workers) > 0 {
		originalWorkerCount = cfg.Workers[0].Count
		cfg.Workers[0].Count = 0
	}

	computeProvisioner := newComputeProvisionerForCreate()
	if err := computeProvisioner.Provision(pCtx); err != nil {
		createErr = fmt.Errorf("compute provisioning failed: %w", err)
		return createErr
	}

	// Restore original counts
	cfg.ControlPlane.NodePools[0].Count = originalCPCount
	if len(cfg.Workers) > 0 {
		cfg.Workers[0].Count = originalWorkerCount
	}

	// Phase 4: Bootstrap
	log.Println("Phase 4: Bootstrapping cluster...")
	clusterProvisioner := newClusterProvisionerForCreate()
	if err := clusterProvisioner.Provision(pCtx); err != nil {
		createErr = fmt.Errorf("cluster bootstrap failed: %w", err)
		return createErr
	}

	// Get kubeconfig from the state (populated by cluster provisioner)
	kubeconfig := pCtx.State.Kubeconfig
	if len(kubeconfig) == 0 {
		createErr = fmt.Errorf("kubeconfig not available after cluster bootstrap")
		return createErr
	}
	if err := writeKubeconfig(kubeconfig); err != nil {
		createErr = err
		return createErr
	}

	// Phase 5: Install Operator
	log.Println("Phase 5: Installing k8zner operator...")
	// Enable operator addon with hostNetwork (required before CNI is installed)
	cfg.Addons.Operator.Enabled = true
	cfg.Addons.Operator.HostNetwork = true

	// Install only the operator addon
	if err := installOperatorOnly(ctx, cfg, kubeconfig, pCtx.State.Network.ID); err != nil {
		createErr = fmt.Errorf("operator installation failed: %w", err)
		return createErr
	}

	// Get infrastructure details for CRD - prefer state LB, fallback to API lookup
	lb := pCtx.State.LoadBalancer
	log.Printf("[DEBUG] pCtx.State.LoadBalancer = %v", lb)
	if lb == nil {
		// Fallback to API lookup if state LB not available
		lbName := naming.KubeAPILoadBalancer(cfg.ClusterName)
		log.Printf("[DEBUG] State LB is nil, looking up LB by name: %s", lbName)
		var err error
		lb, err = infraClient.GetLoadBalancer(ctx, lbName)
		if err != nil { //nolint:gocritic // if-else chain is clearer for distinct error cases
			log.Printf("Warning: failed to get load balancer info: %v", err)
		} else if lb == nil {
			log.Printf("Warning: load balancer %s not found", lbName)
		} else {
			log.Printf("[DEBUG] Found LB via API: ID=%d, Name=%s, PublicNet=%+v, PrivateNet=%+v",
				lb.ID, lb.Name, lb.PublicNet, lb.PrivateNet)
		}
	} else {
		log.Printf("[DEBUG] Using state LB: ID=%d, Name=%s, PublicNet=%+v, PrivateNet=%+v",
			lb.ID, lb.Name, lb.PublicNet, lb.PrivateNet)
	}

	infraInfo := &InfrastructureInfo{
		NetworkID:   pCtx.State.Network.ID,
		NetworkName: pCtx.State.Network.Name,
		SSHKeyID:    pCtx.State.SSHKeyID,
	}
	if pCtx.State.Firewall != nil {
		infraInfo.FirewallID = pCtx.State.Firewall.ID
		infraInfo.FirewallName = pCtx.State.Firewall.Name
	}
	if lb != nil {
		infraInfo.LoadBalancerID = lb.ID
		infraInfo.LoadBalancerName = lb.Name
		infraInfo.LoadBalancerIP = hcloudInternal.LoadBalancerIPv4(lb)
		infraInfo.LoadBalancerPrivateIP = hcloudInternal.LoadBalancerPrivateIP(lb)
	}

	log.Printf("[DEBUG] infraInfo: NetworkID=%d, FirewallID=%d, LoadBalancerID=%d, LoadBalancerIP=%s, LoadBalancerPrivateIP=%s, SSHKeyID=%d",
		infraInfo.NetworkID, infraInfo.FirewallID, infraInfo.LoadBalancerID, infraInfo.LoadBalancerIP, infraInfo.LoadBalancerPrivateIP, infraInfo.SSHKeyID)

	// Phase 6: Create CRD
	log.Println("Phase 6: Creating K8znerCluster CRD...")
	if err := createClusterCRDForCreate(ctx, cfg, pCtx, infraInfo, kubeconfig, token); err != nil {
		return fmt.Errorf("CRD creation failed: %w", err)
	}

	printCreateSuccess(cfg, wait)

	// Optionally wait for operator to complete
	if wait {
		return waitForOperatorComplete(ctx, cfg.ClusterName, kubeconfig)
	}

	return nil
}

// installOperatorOnly installs just the k8zner-operator addon.
// Includes retry logic for transient API server errors.
func installOperatorOnly(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	// Disable all other addons temporarily
	savedAddons := cfg.Addons
	cfg.Addons = config.AddonsConfig{
		Operator: savedAddons.Operator,
	}
	defer func() {
		cfg.Addons = savedAddons
	}()

	// Retry with backoff for transient errors (API server just started)
	const maxRetries = 5
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			delay := time.Duration(i*10) * time.Second
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

		// Check if it's a transient error worth retrying
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

// createClusterCRDForCreate creates the K8znerCluster CRD and credentials Secret.
func createClusterCRDForCreate(ctx context.Context, cfg *config.Config, pCtx *provisioning.Context, infraInfo *InfrastructureInfo, kubeconfig []byte, hcloudToken string) error {
	// Load kubeconfig to connect to the cluster
	kubecfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	// Create controller-runtime client
	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(kubecfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create namespace if it doesn't exist
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8znerNamespace,
		},
	}
	if err := k8sClient.Create(ctx, ns); err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Read secrets.yaml and talosconfig
	secretsData, err := os.ReadFile(secretsFile)
	if err != nil {
		return fmt.Errorf("failed to read secrets.yaml: %w", err)
	}
	talosConfigData, err := os.ReadFile(talosConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read talosconfig: %w", err)
	}

	// Create credentials Secret
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
	// Add Cloudflare API token if configured (for DNS/TLS integration)
	if cfg.Addons.Cloudflare.APIToken != "" {
		credSecret.Data[k8znerv1alpha1.CredentialsKeyCloudflareAPIToken] = []byte(cfg.Addons.Cloudflare.APIToken)
	}
	if err := k8sClient.Create(ctx, credSecret); err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("failed to create credentials secret: %w", err)
	}

	// Create backup S3 Secret if backup is enabled with credentials
	if backupSecret := createBackupS3Secret(cfg, cfg.ClusterName); backupSecret != nil {
		if err := k8sClient.Create(ctx, backupSecret); err != nil && !isAlreadyExists(err) {
			return fmt.Errorf("failed to create backup S3 secret: %w", err)
		}
		log.Printf("Created backup S3 secret: %s", backupSecret.Name)
	}

	// Get bootstrap node info - use deterministic selection by sorting names
	bootstrapName, bootstrapID, bootstrapIP := getBootstrapNode(pCtx)

	// Create K8znerCluster CRD
	k8znerCluster := buildK8znerClusterForCreate(cfg, pCtx, infraInfo, bootstrapName, bootstrapID, bootstrapIP)
	if err := k8sClient.Create(ctx, k8znerCluster); err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("failed to create K8znerCluster: %w", err)
	}

	// Kubernetes ignores status during create (it's a subresource), so we need to update it separately
	// Fetch the created resource to get the latest resourceVersion
	createdCluster := &k8znerv1alpha1.K8znerCluster{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: k8znerCluster.Name, Namespace: k8znerCluster.Namespace}, createdCluster); err != nil {
		return fmt.Errorf("failed to get created K8znerCluster: %w", err)
	}

	// Set the status fields that were prepared in buildK8znerClusterForCreate
	createdCluster.Status = k8znerCluster.Status
	if err := k8sClient.Status().Update(ctx, createdCluster); err != nil {
		return fmt.Errorf("failed to update K8znerCluster status: %w", err)
	}

	return nil
}

// buildK8znerClusterForCreate builds the K8znerCluster CRD from config.
func buildK8znerClusterForCreate(cfg *config.Config, _ *provisioning.Context, infraInfo *InfrastructureInfo, bootstrapName string, bootstrapID int64, bootstrapIP string) *k8znerv1alpha1.K8znerCluster {
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
			ProvisioningPhase: k8znerv1alpha1.PhaseCNI, // Start with CNI phase
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Desired: cfg.ControlPlane.NodePools[0].Count,
				Ready:   1, // Bootstrap CP is ready
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

	// Set placement group (enabled by default for HA)
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

	// Extract subdomain overrides from IngressHost if domain is set
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
// Returns nil if backup is not enabled or S3 credentials are missing.
func buildBackupSpec(cfg *config.Config, clusterName string) *k8znerv1alpha1.BackupSpec {
	if !cfg.Addons.TalosBackup.Enabled {
		return nil
	}

	// Require S3 credentials for backup to work
	if cfg.Addons.TalosBackup.S3AccessKey == "" || cfg.Addons.TalosBackup.S3SecretKey == "" {
		return nil
	}

	return &k8znerv1alpha1.BackupSpec{
		Enabled:   true,
		Schedule:  cfg.Addons.TalosBackup.Schedule,
		Retention: "168h", // Default 7 days
		S3SecretRef: &k8znerv1alpha1.SecretReference{
			Name: backupS3SecretName(clusterName),
		},
	}
}

// backupS3SecretName returns the name of the Secret containing backup S3 credentials.
func backupS3SecretName(clusterName string) string {
	return clusterName + "-backup-s3"
}

// createBackupS3Secret creates the Secret containing S3 credentials for backup.
// Returns nil if backup is not enabled.
func createBackupS3Secret(cfg *config.Config, clusterName string) *corev1.Secret {
	if !cfg.Addons.TalosBackup.Enabled {
		return nil
	}

	// Require S3 credentials
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

// getWorkerCount returns the total worker count from config.
func getWorkerCount(cfg *config.Config) int {
	if len(cfg.Workers) == 0 {
		return 0
	}
	return cfg.Workers[0].Count
}

// getWorkerSize returns the worker server type from config.
func getWorkerSize(cfg *config.Config) string {
	if len(cfg.Workers) == 0 {
		return configv2.DefaultWorkerServerType
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
		// Check context first
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check timeout
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for operator to complete after %v", operatorWaitTimeout)
		}

		// Wait for next tick or context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		// Check context again after waking up
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Use a timeout context for the API call
		getCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		k8zCluster := &k8znerv1alpha1.K8znerCluster{}
		key := client.ObjectKey{
			Namespace: k8znerNamespace,
			Name:      clusterName,
		}

		err := k8sClient.Get(getCtx, key, k8zCluster)
		cancel()

		if err != nil {
			// Log but continue on transient errors
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

// isAlreadyExists checks if an error indicates the resource already exists.
func isAlreadyExists(err error) bool {
	return apierrors.IsAlreadyExists(err)
}

// printCreateSuccess outputs completion message and next steps.
func printCreateSuccess(cfg *config.Config, wait bool) {
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
		fmt.Printf("  k8zner health --watch\n")
		fmt.Printf("  kubectl get k8znerclusters -n %s -w\n", k8znerNamespace)
	}

	fmt.Printf("\nAccess your cluster:\n")
	fmt.Printf("  export KUBECONFIG=%s\n", kubeconfigPath)
	fmt.Printf("  kubectl get nodes\n")
}

// getBootstrapNode returns the bootstrap node info from the provisioning state.
// It uses sorted iteration to ensure deterministic selection.
func getBootstrapNode(pCtx *provisioning.Context) (name string, serverID int64, ip string) {
	if len(pCtx.State.ControlPlaneIPs) == 0 {
		return "", 0, ""
	}

	// Sort names for deterministic selection
	names := make([]string, 0, len(pCtx.State.ControlPlaneIPs))
	for n := range pCtx.State.ControlPlaneIPs {
		names = append(names, n)
	}
	sort.Strings(names)

	// Return the first (alphabetically) control plane
	name = names[0]
	ip = pCtx.State.ControlPlaneIPs[name]
	serverID = pCtx.State.ControlPlaneServerIDs[name]
	return name, serverID, ip
}

// cleanupOnFailure destroys all resources created during a failed create operation.
// This uses the destroy provisioner to clean up infrastructure by cluster label.
func cleanupOnFailure(ctx context.Context, cfg *config.Config, infraClient hcloudInternal.InfrastructureManager) error {
	log.Printf("Cleaning up resources for cluster: %s", cfg.ClusterName)

	// Create a minimal provisioning context for destroy
	pCtx := provisioning.NewContext(ctx, cfg, infraClient, nil)

	// Use the destroy provisioner
	destroyer := destroy.NewProvisioner()
	if err := destroyer.Provision(pCtx); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	log.Printf("Cleanup complete for cluster: %s", cfg.ClusterName)
	return nil
}
