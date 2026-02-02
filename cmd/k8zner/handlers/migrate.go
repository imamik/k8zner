package handlers

import (
	"context"
	"fmt"
	"log"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
	hcloudInternal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/util/naming"
)

// Migrate handles the migrate command.
//
// This function converts a legacy CLI-provisioned cluster to operator management:
//  1. Detects existing resources by cluster label
//  2. Reads local secrets.yaml and talosconfig files
//  3. Creates a credentials Secret in the cluster
//  4. Creates a K8znerCluster CRD with detected state
//  5. Installs the operator (if not present)
func Migrate(ctx context.Context, configPath string, dryRun bool) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	log.Printf("Migrating cluster: %s", cfg.ClusterName)

	// Check prerequisites
	if err := checkMigrationPrereqs(); err != nil {
		return err
	}

	// Load kubeconfig
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return fmt.Errorf("kubeconfig not found at %s - is the cluster running?", kubeconfigPath)
	}

	kubecfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Create kubernetes client
	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(kubecfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Detect existing infrastructure
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		return fmt.Errorf("HCLOUD_TOKEN environment variable is required")
	}
	infraClient := newInfraClient(token)

	// Get infrastructure IDs from Hetzner Cloud
	log.Println("Detecting existing infrastructure...")
	infra, err := detectInfrastructure(ctx, infraClient, cfg.ClusterName)
	if err != nil {
		return fmt.Errorf("failed to detect infrastructure: %w", err)
	}

	// Get node information from kubernetes
	log.Println("Detecting existing nodes...")
	cpNodes, workerNodes, err := detectNodes(ctx, k8sClient)
	if err != nil {
		return fmt.Errorf("failed to detect nodes: %w", err)
	}

	if dryRun {
		printMigrationPlan(cfg, infra, cpNodes, workerNodes)
		return nil
	}

	// Step 1: Create namespace
	log.Println("Creating k8zner-system namespace...")
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8znerNamespace,
		},
	}
	if err := k8sClient.Create(ctx, ns); err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Step 2: Create credentials Secret
	log.Println("Creating credentials secret...")
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
			k8znerv1alpha1.CredentialsKeyHCloudToken:  []byte(token),
			k8znerv1alpha1.CredentialsKeyTalosSecrets: secretsData,
			k8znerv1alpha1.CredentialsKeyTalosConfig:  talosConfigData,
		},
	}
	if err := k8sClient.Create(ctx, credSecret); err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create credentials secret: %w", err)
	}

	// Step 3: Create K8znerCluster CRD
	log.Println("Creating K8znerCluster CRD...")
	cluster := buildMigrationCRD(cfg, infra, cpNodes, workerNodes)
	if err := k8sClient.Create(ctx, cluster); err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create K8znerCluster: %w", err)
	}

	// Step 4: Install operator if not present
	log.Println("Checking operator installation...")
	if err := ensureOperatorInstalled(ctx, cfg, k8sClient, infra.NetworkID); err != nil {
		return fmt.Errorf("failed to ensure operator installed: %w", err)
	}

	printMigrationSuccess(cfg.ClusterName)
	return nil
}

// detectInfrastructure finds existing Hetzner resources by cluster name.
// Returns infrastructure info populated from Hetzner Cloud API.
func detectInfrastructure(ctx context.Context, infraClient hcloudInternal.InfrastructureManager, clusterName string) (*InfrastructureInfo, error) {
	info := &InfrastructureInfo{}

	// Get network
	networkName := naming.Network(clusterName)
	network, err := infraClient.GetNetwork(ctx, networkName)
	if err == nil && network != nil {
		info.NetworkID = network.ID
		info.NetworkName = network.Name
	}

	// Get firewall
	firewallName := naming.Firewall(clusterName)
	firewall, err := infraClient.GetFirewall(ctx, firewallName)
	if err == nil && firewall != nil {
		info.FirewallID = firewall.ID
		info.FirewallName = firewall.Name
	}

	// Get load balancer
	lbName := naming.KubeAPILoadBalancer(clusterName)
	lb, err := infraClient.GetLoadBalancer(ctx, lbName)
	if err == nil && lb != nil {
		info.LoadBalancerID = lb.ID
		info.LoadBalancerName = lb.Name
		info.LoadBalancerIP = hcloudInternal.LoadBalancerIPv4(lb)
	}

	return info, nil
}

// detectNodes gets control plane and worker node info from kubernetes.
func detectNodes(ctx context.Context, k8sClient client.Client) ([]k8znerv1alpha1.NodeStatus, []k8znerv1alpha1.NodeStatus, error) {
	nodeList := &corev1.NodeList{}
	if err := k8sClient.List(ctx, nodeList); err != nil {
		return nil, nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	var cpNodes, workerNodes []k8znerv1alpha1.NodeStatus

	for _, node := range nodeList.Items {
		nodeStatus := k8znerv1alpha1.NodeStatus{
			Name:    node.Name,
			Healthy: isNodeHealthy(&node),
		}

		// Extract server ID from provider ID
		if node.Spec.ProviderID != "" {
			var serverID int64
			if _, err := fmt.Sscanf(node.Spec.ProviderID, "hcloud://%d", &serverID); err == nil {
				nodeStatus.ServerID = serverID
			}
		}

		// Get IPs
		for _, addr := range node.Status.Addresses {
			switch addr.Type {
			case corev1.NodeInternalIP:
				nodeStatus.PrivateIP = addr.Address
			case corev1.NodeExternalIP:
				nodeStatus.PublicIP = addr.Address
			}
		}

		// Categorize by role
		if _, isCP := node.Labels["node-role.kubernetes.io/control-plane"]; isCP {
			cpNodes = append(cpNodes, nodeStatus)
		} else {
			workerNodes = append(workerNodes, nodeStatus)
		}
	}

	return cpNodes, workerNodes, nil
}

// isNodeHealthy checks if a node is ready.
func isNodeHealthy(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// buildMigrationCRD creates a K8znerCluster CRD from detected state.
func buildMigrationCRD(cfg *config.Config, infra *InfrastructureInfo, cpNodes, workerNodes []k8znerv1alpha1.NodeStatus) *k8znerv1alpha1.K8znerCluster {
	now := metav1.Now()

	// Find bootstrap node (first CP)
	var bootstrapName string
	var bootstrapID int64
	var bootstrapIP string
	if len(cpNodes) > 0 {
		bootstrapName = cpNodes[0].Name
		bootstrapID = cpNodes[0].ServerID
		bootstrapIP = cpNodes[0].PublicIP
	}

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.ClusterName,
			Namespace: k8znerNamespace,
			Labels: map[string]string{
				"cluster": cfg.ClusterName,
			},
			Annotations: map[string]string{
				"k8zner.io/migrated-at": now.Format(metav1.RFC3339Micro),
			},
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region: cfg.Location,
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
				Count: len(cpNodes),
				Size:  cfg.ControlPlane.NodePools[0].ServerType,
			},
			Workers: k8znerv1alpha1.WorkerSpec{
				Count: len(workerNodes),
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
			Phase:             k8znerv1alpha1.ClusterPhaseRunning,
			ProvisioningPhase: k8znerv1alpha1.PhaseComplete,
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Desired: len(cpNodes),
				Ready:   countHealthy(cpNodes),
				Nodes:   cpNodes,
			},
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Desired: len(workerNodes),
				Ready:   countHealthy(workerNodes),
				Nodes:   workerNodes,
			},
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID:      infra.NetworkID,
				FirewallID:     infra.FirewallID,
				LoadBalancerID: infra.LoadBalancerID,
				LoadBalancerIP: infra.LoadBalancerIP,
				SSHKeyID:       infra.SSHKeyID,
			},
		},
	}

	return cluster
}

// countHealthy counts healthy nodes in a list.
func countHealthy(nodes []k8znerv1alpha1.NodeStatus) int {
	count := 0
	for _, n := range nodes {
		if n.Healthy {
			count++
		}
	}
	return count
}

// ensureOperatorInstalled checks if operator is installed and installs if not.
func ensureOperatorInstalled(ctx context.Context, cfg *config.Config, k8sClient client.Client, networkID int64) error {
	// Check if operator deployment exists
	deployment := &corev1.Pod{}
	key := client.ObjectKey{
		Namespace: k8znerNamespace,
		Name:      "k8zner-operator",
	}

	if err := k8sClient.Get(ctx, key, deployment); err == nil {
		log.Println("Operator already installed")
		return nil
	}

	// Install operator
	log.Println("Installing operator...")

	// Read kubeconfig
	kubeconfig, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to read kubeconfig: %w", err)
	}

	// Enable operator addon
	cfg.Addons.Operator.Enabled = true

	// Install only operator
	savedAddons := cfg.Addons
	cfg.Addons = config.AddonsConfig{
		Operator: savedAddons.Operator,
	}

	if err := addons.Apply(ctx, cfg, kubeconfig, networkID); err != nil {
		return fmt.Errorf("failed to install operator: %w", err)
	}

	cfg.Addons = savedAddons

	return nil
}

// checkMigrationPrereqs verifies migration prerequisites.
func checkMigrationPrereqs() error {
	// Check secrets.yaml exists
	if _, err := os.Stat(secretsFile); os.IsNotExist(err) {
		return fmt.Errorf("secrets.yaml not found - this is required for migration")
	}

	// Check talosconfig exists
	if _, err := os.Stat(talosConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("talosconfig not found - this is required for migration")
	}

	return nil
}

// printMigrationPlan outputs what would be migrated without making changes.
func printMigrationPlan(cfg *config.Config, infra *InfrastructureInfo, cpNodes, workerNodes []k8znerv1alpha1.NodeStatus) {
	fmt.Println("\nMigration Plan (dry run):")
	fmt.Println("─────────────────────────────────────")
	fmt.Printf("\nCluster: %s\n", cfg.ClusterName)

	fmt.Println("\nInfrastructure to detect:")
	fmt.Printf("  Network ID: %d\n", infra.NetworkID)
	fmt.Printf("  Firewall ID: %d\n", infra.FirewallID)
	fmt.Printf("  Load Balancer ID: %d\n", infra.LoadBalancerID)

	fmt.Printf("\nControl Planes: %d\n", len(cpNodes))
	for _, node := range cpNodes {
		healthy := "unhealthy"
		if node.Healthy {
			healthy = "healthy"
		}
		fmt.Printf("  - %s (ID: %d, %s)\n", node.Name, node.ServerID, healthy)
	}

	fmt.Printf("\nWorkers: %d\n", len(workerNodes))
	for _, node := range workerNodes {
		healthy := "unhealthy"
		if node.Healthy {
			healthy = "healthy"
		}
		fmt.Printf("  - %s (ID: %d, %s)\n", node.Name, node.ServerID, healthy)
	}

	fmt.Println("\nActions to perform:")
	fmt.Println("  1. Create namespace: k8zner-system")
	fmt.Println("  2. Create credentials Secret")
	fmt.Println("  3. Create K8znerCluster CRD")
	fmt.Println("  4. Install operator (if not present)")

	fmt.Println("\nRun without --dry-run to execute migration")
}

// printMigrationSuccess outputs success message.
func printMigrationSuccess(clusterName string) {
	fmt.Printf("\nMigration complete!\n")
	fmt.Printf("\nCluster %s is now operator-managed.\n", clusterName)
	fmt.Printf("\nThe operator will now handle:\n")
	fmt.Printf("  - Health monitoring\n")
	fmt.Printf("  - Self-healing (node replacement)\n")
	fmt.Printf("  - Scaling (via 'k8zner apply')\n")
	fmt.Printf("\nMonitor status with:\n")
	fmt.Printf("  k8zner health --watch\n")
	fmt.Printf("  kubectl get k8znerclusters -n %s -w\n", k8znerNamespace)
}

// isAlreadyExistsError checks if an error indicates the resource already exists.
func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return len(errStr) >= 14 && errStr[:14] == "already exists"
}
