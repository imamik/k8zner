package handlers

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
	hcloudInternal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/provisioning/cluster"
	"github.com/imamik/k8zner/internal/provisioning/compute"
	"github.com/imamik/k8zner/internal/provisioning/image"
	"github.com/imamik/k8zner/internal/provisioning/infrastructure"
	"github.com/imamik/k8zner/internal/util/naming"
)

const (
	// credentialsSecretName is the name of the secret containing Hetzner and Talos credentials.
	credentialsSecretName = "k8zner-credentials" //nolint:gosec // This is a secret name, not a credential value
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

	// Phase 1: Image
	log.Println("Phase 1: Ensuring Talos image snapshot...")
	imageProvisioner := newImageProvisionerForCreate()
	if err := imageProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("image provisioning failed: %w", err)
	}

	// Phase 2: Infrastructure
	log.Println("Phase 2: Creating infrastructure (network, firewall, LB, placement group)...")
	infraProvisioner := newInfraProvisionerForCreate()
	if err := infraProvisioner.Provision(pCtx); err != nil {
		return fmt.Errorf("infrastructure provisioning failed: %w", err)
	}

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
		return fmt.Errorf("compute provisioning failed: %w", err)
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
		return fmt.Errorf("cluster bootstrap failed: %w", err)
	}

	// Get kubeconfig from the state (populated by cluster provisioner)
	kubeconfig := pCtx.State.Kubeconfig
	if len(kubeconfig) == 0 {
		return fmt.Errorf("kubeconfig not available after cluster bootstrap")
	}
	if err := writeKubeconfig(kubeconfig); err != nil {
		return err
	}

	// Phase 5: Install Operator
	log.Println("Phase 5: Installing k8zner operator...")
	// Enable operator addon with hostNetwork (required before CNI is installed)
	cfg.Addons.Operator.Enabled = true
	cfg.Addons.Operator.HostNetwork = true

	// Install only the operator addon
	if err := installOperatorOnly(ctx, cfg, kubeconfig, pCtx.State.Network.ID); err != nil {
		return fmt.Errorf("operator installation failed: %w", err)
	}

	// Get infrastructure details for CRD
	lbName := naming.KubeAPILoadBalancer(cfg.ClusterName)
	lb, err := infraClient.GetLoadBalancer(ctx, lbName)
	if err != nil {
		log.Printf("Warning: failed to get load balancer info: %v", err)
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
	}

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
func installOperatorOnly(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	// Disable all other addons temporarily
	savedAddons := cfg.Addons
	cfg.Addons = config.AddonsConfig{
		Operator: savedAddons.Operator,
	}

	err := addons.Apply(ctx, cfg, kubeconfig, networkID)

	// Restore addons
	cfg.Addons = savedAddons

	return err
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
	if err := k8sClient.Create(ctx, credSecret); err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("failed to create credentials secret: %w", err)
	}

	// Get bootstrap node info
	var bootstrapName string
	var bootstrapID int64
	var bootstrapIP string
	for name, ip := range pCtx.State.ControlPlaneIPs {
		bootstrapName = name
		bootstrapIP = ip
		bootstrapID = pCtx.State.ControlPlaneServerIDs[name]
		break
	}

	// Create K8znerCluster CRD
	k8znerCluster := buildK8znerClusterForCreate(cfg, pCtx, infraInfo, bootstrapName, bootstrapID, bootstrapIP)
	if err := k8sClient.Create(ctx, k8znerCluster); err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("failed to create K8znerCluster: %w", err)
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
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
				Count: cfg.ControlPlane.NodePools[0].Count,
				Size:  cfg.ControlPlane.NodePools[0].ServerType,
			},
			Workers: k8znerv1alpha1.WorkerSpec{
				Count: getWorkerCount(cfg),
				Size:  getWorkerSize(cfg),
			},
			Network: k8znerv1alpha1.NetworkSpec{
				IPv4CIDR:    cfg.Network.IPv4CIDR,
				PodCIDR:     cfg.Network.PodIPv4CIDR,
				ServiceCIDR: cfg.Network.ServiceIPv4CIDR,
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
				NetworkID:      infraInfo.NetworkID,
				FirewallID:     infraInfo.FirewallID,
				LoadBalancerID: infraInfo.LoadBalancerID,
				LoadBalancerIP: infraInfo.LoadBalancerIP,
				SSHKeyID:       infraInfo.SSHKeyID,
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
	return &k8znerv1alpha1.AddonSpec{
		Traefik:       cfg.Addons.Traefik.Enabled,
		CertManager:   cfg.Addons.CertManager.Enabled,
		ExternalDNS:   cfg.Addons.ExternalDNS.Enabled,
		ArgoCD:        cfg.Addons.ArgoCD.Enabled,
		MetricsServer: cfg.Addons.MetricsServer.Enabled,
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
		return "cx23"
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

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeout := time.After(30 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for operator to complete")
		case <-ticker.C:
			k8zCluster := &k8znerv1alpha1.K8znerCluster{}
			key := client.ObjectKey{
				Namespace: k8znerNamespace,
				Name:      clusterName,
			}

			if err := k8sClient.Get(ctx, key, k8zCluster); err != nil {
				continue // Retry
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
