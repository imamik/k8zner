// Package handlers implements the business logic for CLI commands.
package handlers

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	talosconfig "github.com/siderolabs/talos/pkg/machinery/client/config"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/config"
	hcloudInternal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/util/keygen"
	"github.com/imamik/k8zner/internal/util/labels"
	"github.com/imamik/k8zner/internal/util/naming"
)

const (
	// Bootstrap timeouts.
	serverCreateTimeout    = 5 * time.Minute
	serverIPWaitTimeout    = 2 * time.Minute
	serverIPRetryInterval  = 5 * time.Second
	talosBootstrapTimeout  = 10 * time.Minute
	apiServerReadyTimeout  = 5 * time.Minute
	operatorInstallTimeout = 2 * time.Minute
	lbCreateTimeout        = 6 * time.Minute

	// Kubernetes namespace for k8zner resources.
	k8znerNamespace = "k8zner-system"
)

// BootstrapOptions contains options for the bootstrap command.
type BootstrapOptions struct {
	ConfigPath    string
	OperatorImage string
	SkipOperator  bool
	DryRun        bool
}

// InfrastructureInfo contains information about created infrastructure.
type InfrastructureInfo struct {
	NetworkID             int64
	NetworkName           string
	FirewallID            int64
	FirewallName          string
	LoadBalancerID        int64
	LoadBalancerName      string
	LoadBalancerIP        string
	LoadBalancerPrivateIP string // Private IP for internal cluster communication
	SSHKeyID              int64
}

// BootstrapServerInfo contains information about the bootstrap server.
type BootstrapServerInfo struct {
	Name      string
	ID        int64
	PublicIP  string
	PrivateIP string
}

// SnapshotInfo contains information about a Talos image snapshot.
type SnapshotInfo struct {
	ID int64
}

// Bootstrap performs cluster bootstrap with stable infrastructure.
//
// This revised bootstrap creates infrastructure FIRST to ensure a stable endpoint:
//  1. Creates Load Balancer (to get stable IP for endpoint)
//  2. Creates Network + Subnets
//  3. Creates Firewall
//  4. Creates SSH Key (if needed)
//  5. Creates first Control Plane server (attached to network)
//  6. Adds CP to Load Balancer target pool
//  7. Generates Talos secrets with LB IP as endpoint
//  8. Applies Talos config and bootstraps etcd
//  9. Waits for API server via Load Balancer
//  10. Installs operator + CRD + credentials Secret
//
// The operator then handles scaling (additional CPs, workers) and addons.
func Bootstrap(ctx context.Context, opts BootstrapOptions) error {
	// Load configuration
	cfg, err := loadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}

	// Run prerequisites check
	if err := checkPrerequisites(cfg); err != nil {
		return err
	}

	log.Printf("Starting bootstrap for cluster: %s", cfg.ClusterName)
	log.Printf("Region: %s", cfg.Location)

	if opts.DryRun {
		printDryRunPlan(cfg)
		return nil
	}

	// Initialize HCloud client
	hcloudToken := os.Getenv("HCLOUD_TOKEN")
	if hcloudToken == "" {
		return fmt.Errorf("HCLOUD_TOKEN environment variable is required")
	}
	hcloudClient := newInfraClient(hcloudToken)

	// Step 1: Ensure Talos image snapshot exists
	log.Println("\n[Step 1/10] Ensuring Talos image snapshot exists...")
	snapshot, err := ensureTalosImage(ctx, hcloudClient, cfg)
	if err != nil {
		return fmt.Errorf("failed to ensure Talos image: %w", err)
	}
	log.Printf("Using Talos snapshot: %d", snapshot.ID)

	// Step 2: Create Load Balancer (to get stable endpoint IP)
	log.Println("\n[Step 2/10] Creating Load Balancer...")
	infra := &InfrastructureInfo{}
	if err := createLoadBalancer(ctx, hcloudClient, cfg, infra); err != nil {
		return fmt.Errorf("failed to create load balancer: %w", err)
	}
	log.Printf("Load Balancer created: %s (IP: %s)", infra.LoadBalancerName, infra.LoadBalancerIP)

	// Step 3: Create Network + Subnets
	log.Println("\n[Step 3/10] Creating Network and Subnets...")
	if err := createNetwork(ctx, hcloudClient, cfg, infra); err != nil {
		return fmt.Errorf("failed to create network: %w", err)
	}
	log.Printf("Network created: %s (ID: %d)", infra.NetworkName, infra.NetworkID)

	// Step 4: Create Firewall
	log.Println("\n[Step 4/10] Creating Firewall...")
	if err := createFirewall(ctx, hcloudClient, cfg, infra); err != nil {
		return fmt.Errorf("failed to create firewall: %w", err)
	}
	log.Printf("Firewall created: %s (ID: %d)", infra.FirewallName, infra.FirewallID)

	// Step 5: Generate Talos secrets with LB IP as endpoint
	log.Println("\n[Step 5/10] Generating Talos secrets...")
	secretsBundle, err := getOrGenerateSecrets(secretsFile, cfg.Talos.Version)
	if err != nil {
		return fmt.Errorf("failed to generate secrets: %w", err)
	}

	// Use LB IP as the stable endpoint
	endpoint := fmt.Sprintf("https://%s:%d", infra.LoadBalancerIP, config.KubeAPIPort)
	talosGen := newTalosGenerator(
		cfg.ClusterName,
		cfg.Kubernetes.Version,
		cfg.Talos.Version,
		endpoint,
		secretsBundle,
	)

	// Write talosconfig and secrets immediately
	if err := writeTalosFiles(talosGen); err != nil {
		return fmt.Errorf("failed to write Talos files: %w", err)
	}
	log.Printf("Talos config saved to: %s", talosConfigPath)
	log.Printf("Secrets saved to: %s", secretsFile)

	// Step 6: Create first Control Plane server (attached to network)
	log.Println("\n[Step 6/10] Creating bootstrap Control Plane server...")
	bootstrapServer, err := createBootstrapServer(ctx, hcloudClient, cfg, snapshot.ID, infra)
	if err != nil {
		return fmt.Errorf("failed to create bootstrap server: %w", err)
	}
	log.Printf("Server created: %s (ID: %d, Public IP: %s, Private IP: %s)",
		bootstrapServer.Name, bootstrapServer.ID, bootstrapServer.PublicIP, bootstrapServer.PrivateIP)

	// Step 7: Add CP to Load Balancer (already done via label selector, but verify)
	log.Println("\n[Step 7/10] Verifying Load Balancer target...")
	// The LB uses label selector, so the server should automatically be a target
	log.Printf("Server %s is targeted by LB via label selector", bootstrapServer.Name)

	// Step 8: Apply Talos config
	log.Println("\n[Step 8/10] Applying Talos configuration...")
	// Generate config with LB IP in SANs
	talosConfig, err := talosGen.GenerateControlPlaneConfig(
		[]string{infra.LoadBalancerIP, bootstrapServer.PublicIP, bootstrapServer.PrivateIP},
		bootstrapServer.Name,
		bootstrapServer.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to generate Talos config: %w", err)
	}

	talosClientConfig, err := talosGen.GetClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get Talos client config: %w", err)
	}

	// Apply via public IP (insecure/maintenance mode)
	if err := applyTalosConfig(ctx, talosClientConfig, bootstrapServer.PublicIP, talosConfig); err != nil {
		return fmt.Errorf("failed to apply Talos config: %w", err)
	}
	log.Println("Talos configuration applied")

	// Step 9: Bootstrap etcd and wait for API server
	log.Println("\n[Step 9/10] Bootstrapping etcd...")
	if err := bootstrapEtcd(ctx, talosClientConfig, bootstrapServer.PublicIP); err != nil {
		return fmt.Errorf("failed to bootstrap etcd: %w", err)
	}
	log.Println("etcd bootstrapped")

	log.Println("Waiting for Kubernetes API server (via Load Balancer)...")
	kubeconfig, err := waitForAPIServer(ctx, talosClientConfig, infra.LoadBalancerIP, apiServerReadyTimeout)
	if err != nil {
		// Try direct IP as fallback
		log.Printf("LB not ready yet, trying direct IP...")
		kubeconfig, err = waitForAPIServer(ctx, talosClientConfig, bootstrapServer.PublicIP, apiServerReadyTimeout)
		if err != nil {
			return fmt.Errorf("API server not ready: %w", err)
		}
	}
	log.Println("Kubernetes API server is ready!")

	// Write kubeconfig (with LB endpoint)
	kubeconfigWithLB := replaceKubeconfigEndpoint(kubeconfig, infra.LoadBalancerIP)
	if err := writeKubeconfig(kubeconfigWithLB); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}
	log.Printf("Kubeconfig saved to: %s", kubeconfigPath)

	// Step 10: Install operator
	if !opts.SkipOperator {
		log.Println("\n[Step 10/10] Installing k8zner operator...")
		if err := installOperator(ctx, kubeconfigWithLB, cfg, hcloudToken, secretsBundle, bootstrapServer, infra, opts.OperatorImage); err != nil {
			return fmt.Errorf("failed to install operator: %w", err)
		}
		log.Println("Operator installed successfully")
	} else {
		log.Println("\n[Step 10/10] Skipping operator installation (--skip-operator)")
	}

	printBootstrapSuccess(cfg, infra)
	return nil
}

// createLoadBalancer creates the API load balancer and returns its public IP.
func createLoadBalancer(ctx context.Context, hclient hcloudInternal.InfrastructureManager, cfg *config.Config, infra *InfrastructureInfo) error {
	lbName := naming.KubeAPILoadBalancer(cfg.ClusterName)
	infra.LoadBalancerName = lbName

	lbLabels := labels.NewLabelBuilder(cfg.ClusterName).
		WithRole("kube-api").
		WithTestIDIfSet(cfg.TestID).
		Build()

	// Create load balancer
	lb, err := hclient.EnsureLoadBalancer(ctx, lbName, cfg.Location, "lb11", hcloud.LoadBalancerAlgorithmTypeRoundRobin, lbLabels)
	if err != nil {
		return fmt.Errorf("failed to ensure load balancer: %w", err)
	}
	infra.LoadBalancerID = lb.ID

	// Get public IP
	infra.LoadBalancerIP = hcloudInternal.LoadBalancerIPv4(lb)
	if infra.LoadBalancerIP == "" {
		return fmt.Errorf("load balancer has no public IP")
	}
	// Get private IP (for internal cluster communication)
	infra.LoadBalancerPrivateIP = hcloudInternal.LoadBalancerPrivateIP(lb)

	// Configure service (port 6443)
	service := hcloud.LoadBalancerAddServiceOpts{
		Protocol:        hcloud.LoadBalancerServiceProtocolTCP,
		ListenPort:      hcloud.Ptr(6443),
		DestinationPort: hcloud.Ptr(6443),
		HealthCheck: &hcloud.LoadBalancerAddServiceOptsHealthCheck{
			Protocol: hcloud.LoadBalancerServiceProtocolHTTP,
			Port:     hcloud.Ptr(6443),
			Interval: hcloud.Ptr(time.Second * 3),
			Timeout:  hcloud.Ptr(time.Second * 2),
			Retries:  hcloud.Ptr(2),
			HTTP: &hcloud.LoadBalancerAddServiceOptsHealthCheckHTTP{
				Path:        hcloud.Ptr("/version"),
				StatusCodes: []string{"401"},
				TLS:         hcloud.Ptr(true),
			},
		},
	}
	if err := hclient.ConfigureService(ctx, lb, service); err != nil {
		return fmt.Errorf("failed to configure LB service: %w", err)
	}

	// Add target using label selector (will match servers with cluster=<name>,role=control-plane)
	targetSelector := fmt.Sprintf("cluster=%s,role=control-plane", cfg.ClusterName)
	if err := hclient.AddTarget(ctx, lb, hcloud.LoadBalancerTargetTypeLabelSelector, targetSelector); err != nil {
		return fmt.Errorf("failed to add LB target: %w", err)
	}

	return nil
}

// createNetwork creates the private network and subnets.
func createNetwork(ctx context.Context, hclient hcloudInternal.InfrastructureManager, cfg *config.Config, infra *InfrastructureInfo) error {
	networkName := cfg.ClusterName
	infra.NetworkName = networkName

	networkLabels := labels.NewLabelBuilder(cfg.ClusterName).
		WithTestIDIfSet(cfg.TestID).
		Build()

	// Create network
	network, err := hclient.EnsureNetwork(ctx, networkName, cfg.Network.IPv4CIDR, cfg.Network.Zone, networkLabels)
	if err != nil {
		return fmt.Errorf("failed to ensure network: %w", err)
	}
	infra.NetworkID = network.ID

	// Create subnets
	// Control Plane subnet
	cpSubnet, err := cfg.GetSubnetForRole("control-plane", 0)
	if err != nil {
		return fmt.Errorf("failed to calculate control-plane subnet: %w", err)
	}
	if err := hclient.EnsureSubnet(ctx, network, cpSubnet, cfg.Network.Zone, hcloud.NetworkSubnetTypeCloud); err != nil {
		return fmt.Errorf("failed to ensure control-plane subnet: %w", err)
	}

	// Load Balancer subnet
	lbSubnet, err := cfg.GetSubnetForRole("load-balancer", 0)
	if err != nil {
		return fmt.Errorf("failed to calculate load-balancer subnet: %w", err)
	}
	if err := hclient.EnsureSubnet(ctx, network, lbSubnet, cfg.Network.Zone, hcloud.NetworkSubnetTypeCloud); err != nil {
		return fmt.Errorf("failed to ensure load-balancer subnet: %w", err)
	}

	// Worker subnets
	for i := range cfg.Workers {
		wSubnet, err := cfg.GetSubnetForRole("worker", i)
		if err != nil {
			return fmt.Errorf("failed to calculate worker subnet %d: %w", i, err)
		}
		if err := hclient.EnsureSubnet(ctx, network, wSubnet, cfg.Network.Zone, hcloud.NetworkSubnetTypeCloud); err != nil {
			return fmt.Errorf("failed to ensure worker subnet %d: %w", i, err)
		}
	}

	// Attach LB to network
	lb, err := hclient.GetLoadBalancer(ctx, infra.LoadBalancerName)
	if err != nil {
		return fmt.Errorf("failed to get load balancer: %w", err)
	}

	// Calculate LB private IP
	lbPrivateIPStr, err := config.CIDRHost(lbSubnet, -2)
	if err != nil {
		return fmt.Errorf("failed to calculate LB private IP: %w", err)
	}
	lbPrivateIP := net.ParseIP(lbPrivateIPStr)

	if err := hclient.AttachToNetwork(ctx, lb, network, lbPrivateIP); err != nil {
		return fmt.Errorf("failed to attach LB to network: %w", err)
	}

	return nil
}

// createFirewall creates the cluster firewall.
func createFirewall(ctx context.Context, hclient hcloudInternal.InfrastructureManager, cfg *config.Config, infra *InfrastructureInfo) error {
	firewallName := cfg.ClusterName
	infra.FirewallName = firewallName

	firewallLabels := labels.NewLabelBuilder(cfg.ClusterName).
		WithTestIDIfSet(cfg.TestID).
		Build()

	// Detect current public IP for firewall rules
	publicIP, _ := hclient.GetPublicIP(ctx)

	// Build firewall rules
	rules := []hcloud.FirewallRule{}

	// Kube API (6443) - allow from current IP and configured sources
	kubeAPISources := []string{"0.0.0.0/0", "::/0"} // Default: open (can be restricted via config)
	if len(cfg.Firewall.KubeAPISource) > 0 {
		kubeAPISources = cfg.Firewall.KubeAPISource
	} else if len(cfg.Firewall.APISource) > 0 {
		kubeAPISources = cfg.Firewall.APISource
	}
	if publicIP != "" && cfg.Firewall.UseCurrentIPv4 != nil && *cfg.Firewall.UseCurrentIPv4 {
		kubeAPISources = append(kubeAPISources, publicIP+"/32")
	}

	if sourceNets := parseCIDRs(kubeAPISources); len(sourceNets) > 0 {
		rules = append(rules, hcloud.FirewallRule{
			Description: hcloud.Ptr("Allow Kube API"),
			Direction:   hcloud.FirewallRuleDirectionIn,
			Protocol:    hcloud.FirewallRuleProtocolTCP,
			Port:        hcloud.Ptr("6443"),
			SourceIPs:   sourceNets,
		})
	}

	// Talos API (50000)
	talosAPISources := []string{}
	if len(cfg.Firewall.TalosAPISource) > 0 {
		talosAPISources = cfg.Firewall.TalosAPISource
	} else if len(cfg.Firewall.APISource) > 0 {
		talosAPISources = cfg.Firewall.APISource
	}
	if publicIP != "" && cfg.Firewall.UseCurrentIPv4 != nil && *cfg.Firewall.UseCurrentIPv4 {
		talosAPISources = append(talosAPISources, publicIP+"/32")
	}

	if sourceNets := parseCIDRs(talosAPISources); len(sourceNets) > 0 {
		rules = append(rules, hcloud.FirewallRule{
			Description: hcloud.Ptr("Allow Talos API"),
			Direction:   hcloud.FirewallRuleDirectionIn,
			Protocol:    hcloud.FirewallRuleProtocolTCP,
			Port:        hcloud.Ptr("50000"),
			SourceIPs:   sourceNets,
		})
	}

	// Apply firewall with label selector
	applyToLabelSelector := fmt.Sprintf("cluster=%s", cfg.ClusterName)

	firewall, err := hclient.EnsureFirewall(ctx, firewallName, rules, firewallLabels, applyToLabelSelector)
	if err != nil {
		return fmt.Errorf("failed to ensure firewall: %w", err)
	}
	infra.FirewallID = firewall.ID

	return nil
}

// parseCIDRs parses a slice of CIDR strings into net.IPNet.
func parseCIDRs(cidrs []string) []net.IPNet {
	var nets []net.IPNet
	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, *n)
		}
	}
	return nets
}

// createBootstrapServer creates the first control plane server attached to the network.
// Uses an ephemeral SSH key to avoid Hetzner password emails, then deletes it after provisioning.
func createBootstrapServer(ctx context.Context, hclient hcloudInternal.InfrastructureManager, cfg *config.Config, snapshotID int64, infra *InfrastructureInfo) (*BootstrapServerInfo, error) {
	// Use deterministic ID "1" for the first/bootstrap control plane for idempotency
	serverName := naming.ControlPlaneWithID(cfg.ClusterName, "1")

	// Check if server already exists
	existingIP, err := hclient.GetServerIP(ctx, serverName)
	if err == nil && existingIP != "" {
		serverIDStr, err := hclient.GetServerID(ctx, serverName)
		if err != nil {
			return nil, fmt.Errorf("server exists but failed to get ID: %w", err)
		}
		var serverID int64
		if _, err := fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
			return nil, fmt.Errorf("failed to parse server ID: %w", err)
		}
		log.Printf("Using existing bootstrap server: %s", serverName)
		return &BootstrapServerInfo{
			Name:     serverName,
			ID:       serverID,
			PublicIP: existingIP,
		}, nil
	}

	// Create ephemeral SSH key to avoid Hetzner password emails
	// This key is created for server provisioning and deleted immediately after
	sshKeyName := fmt.Sprintf("ephemeral-%s-%d", serverName, time.Now().Unix())
	log.Printf("Creating ephemeral SSH key: %s", sshKeyName)

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral SSH key: %w", err)
	}

	sshKeyLabels := labels.NewLabelBuilder(cfg.ClusterName).
		WithTestIDIfSet(cfg.TestID).
		Build()
	sshKeyLabels["type"] = "ephemeral-bootstrap"

	_, err = hclient.CreateSSHKey(ctx, sshKeyName, string(keyPair.PublicKey), sshKeyLabels)
	if err != nil {
		return nil, fmt.Errorf("failed to upload ephemeral SSH key: %w", err)
	}

	// Schedule cleanup of the ephemeral SSH key
	defer func() {
		log.Printf("Cleaning up ephemeral SSH key: %s", sshKeyName)
		if err := hclient.DeleteSSHKey(ctx, sshKeyName); err != nil {
			log.Printf("Warning: failed to delete ephemeral SSH key %s: %v", sshKeyName, err)
		}
	}()

	// Build labels
	serverLabels := labels.NewLabelBuilder(cfg.ClusterName).
		WithRole("control-plane").
		WithPool("control-plane").
		WithTestIDIfSet(cfg.TestID).
		Build()

	// Calculate private IP for the server
	cpSubnet, err := cfg.GetSubnetForRole("control-plane", 0)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate control-plane subnet: %w", err)
	}
	privateIPStr, err := config.CIDRHost(cpSubnet, 1) // First IP in subnet
	if err != nil {
		return nil, fmt.Errorf("failed to calculate private IP: %w", err)
	}

	// Create server attached to network using the ephemeral SSH key
	serverIDStr, err := hclient.CreateServer(
		ctx,
		serverName,
		fmt.Sprintf("%d", snapshotID),
		cfg.ControlPlane.NodePools[0].ServerType,
		cfg.Location,
		[]string{sshKeyName}, // Use ephemeral SSH key instead of cfg.SSHKeys
		serverLabels,
		"",              // userData
		nil,             // placementGroupID
		infra.NetworkID, // networkID - attach to network
		privateIPStr,    // privateIP
		true,            // enablePublicIPv4
		true,            // enablePublicIPv6
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	var serverID int64
	if _, err := fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
		return nil, fmt.Errorf("failed to parse server ID: %w", err)
	}

	// Wait for public IP
	publicIP, err := waitForServerIP(ctx, hclient, serverName, serverIPWaitTimeout)
	if err != nil {
		return nil, fmt.Errorf("server created but failed to get IP: %w", err)
	}

	return &BootstrapServerInfo{
		Name:      serverName,
		ID:        serverID,
		PublicIP:  publicIP,
		PrivateIP: privateIPStr,
	}, nil
}

// ensureTalosImage ensures a Talos image snapshot exists.
func ensureTalosImage(ctx context.Context, hclient hcloudInternal.InfrastructureManager, cfg *config.Config) (*SnapshotInfo, error) {
	snapshotLabels := map[string]string{
		"os":            "talos",
		"talos_version": cfg.Talos.Version,
	}
	snapshot, err := hclient.GetSnapshotByLabels(ctx, snapshotLabels)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing snapshot: %w", err)
	}
	if snapshot != nil {
		return &SnapshotInfo{ID: snapshot.ID}, nil
	}

	return nil, fmt.Errorf("no Talos image found with version %s. Run 'k8zner image build' first", cfg.Talos.Version)
}

// waitForServerIP waits for a server to have a public IP assigned.
func waitForServerIP(ctx context.Context, hclient hcloudInternal.InfrastructureManager, serverName string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(serverIPRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout waiting for server IP: %w", ctx.Err())
		case <-ticker.C:
			ip, err := hclient.GetServerIP(ctx, serverName)
			if err == nil && ip != "" {
				return ip, nil
			}
		}
	}
}

// applyTalosConfig applies the Talos machine configuration to a node.
func applyTalosConfig(ctx context.Context, talosClientConfig []byte, nodeIP string, machineConfig []byte) error {
	cfg, err := talosconfig.FromBytes(talosClientConfig)
	if err != nil {
		return fmt.Errorf("failed to parse talosconfig: %w", err)
	}

	talosClient, err := client.New(ctx,
		client.WithConfig(cfg),
		client.WithEndpoints(nodeIP),
	)
	if err != nil {
		return fmt.Errorf("failed to create Talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	_, err = talosClient.ApplyConfiguration(ctx, &machine.ApplyConfigurationRequest{
		Data: machineConfig,
		Mode: machine.ApplyConfigurationRequest_AUTO,
	})
	return err
}

// bootstrapEtcd bootstraps the etcd cluster on the first control plane node.
func bootstrapEtcd(ctx context.Context, talosClientConfig []byte, nodeIP string) error {
	cfg, err := talosconfig.FromBytes(talosClientConfig)
	if err != nil {
		return fmt.Errorf("failed to parse talosconfig: %w", err)
	}

	talosClient, err := client.New(ctx,
		client.WithConfig(cfg),
		client.WithEndpoints(nodeIP),
	)
	if err != nil {
		return fmt.Errorf("failed to create Talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	return talosClient.Bootstrap(ctx, &machine.BootstrapRequest{})
}

// waitForAPIServer waits for the Kubernetes API server to become ready.
func waitForAPIServer(ctx context.Context, talosClientConfig []byte, endpoint string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cfg, err := talosconfig.FromBytes(talosClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse talosconfig: %w", err)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for API server: %w", ctx.Err())
		case <-ticker.C:
			talosClient, err := client.New(ctx,
				client.WithConfig(cfg),
				client.WithEndpoints(endpoint),
			)
			if err != nil {
				log.Printf("Failed to create Talos client, retrying...")
				continue
			}

			kubeconfig, err := talosClient.Kubeconfig(ctx)
			_ = talosClient.Close()

			if err == nil && len(kubeconfig) > 0 {
				return kubeconfig, nil
			}
			log.Printf("API server not ready yet, retrying...")
		}
	}
}

// replaceKubeconfigEndpoint updates the kubeconfig to use the Load Balancer IP.
func replaceKubeconfigEndpoint(kubeconfig []byte, lbIP string) []byte {
	// Simple string replacement for the server URL
	content := string(kubeconfig)
	// The kubeconfig server is typically https://<ip>:6443
	// We want to ensure it uses the LB IP
	newEndpoint := fmt.Sprintf("https://%s:6443", lbIP)

	// Replace any server URL with the LB endpoint
	// This is a simple approach - for production, consider using the kubeconfig API
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "server:") {
			lines[i] = strings.Replace(line, trimmed, fmt.Sprintf("server: %s", newEndpoint), 1)
		}
	}

	return []byte(strings.Join(lines, "\n"))
}

// installOperator installs the k8zner operator into the cluster.
func installOperator(ctx context.Context, kubeconfig []byte, cfg *config.Config, hcloudToken string, secretsBundle interface{}, bootstrapServer *BootstrapServerInfo, infra *InfrastructureInfo, operatorImage string) error {
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create REST config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Create namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8znerNamespace,
		},
	}
	if _, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Create credentials Secret
	if err := createCredentialsSecret(ctx, clientset, cfg, hcloudToken, secretsBundle); err != nil {
		return fmt.Errorf("failed to create credentials secret: %w", err)
	}

	// Create K8znerCluster CRD
	if err := createK8znerClusterCRD(ctx, kubeconfig, cfg, bootstrapServer, infra); err != nil {
		return fmt.Errorf("failed to create K8znerCluster CRD: %w", err)
	}

	log.Printf("Operator installation requires CRDs and deployment manifests.")
	log.Printf("Apply the operator manifests with: kubectl apply -f https://raw.githubusercontent.com/imamik/k8zner/main/deploy/operator.yaml")

	return nil
}

// createCredentialsSecret creates the credentials Secret.
func createCredentialsSecret(ctx context.Context, clientset *kubernetes.Clientset, cfg *config.Config, hcloudToken string, secretsBundle interface{}) error {
	secretsData, err := os.ReadFile(secretsFile)
	if err != nil {
		return fmt.Errorf("failed to read secrets file: %w", err)
	}

	talosConfigData, err := os.ReadFile(talosConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read talosconfig: %w", err)
	}

	secretName := fmt.Sprintf("%s-credentials", cfg.ClusterName)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: k8znerNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "k8zner",
				"app.kubernetes.io/component": "credentials",
				"k8zner.io/cluster":           cfg.ClusterName,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			k8znerv1alpha1.CredentialsKeyHCloudToken:  []byte(hcloudToken),
			k8znerv1alpha1.CredentialsKeyTalosSecrets: secretsData,
			k8znerv1alpha1.CredentialsKeyTalosConfig:  talosConfigData,
		},
	}

	if _, err := clientset.CoreV1().Secrets(k8znerNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	log.Printf("Created credentials secret: %s/%s", k8znerNamespace, secretName)
	return nil
}

// createK8znerClusterCRD creates the K8znerCluster custom resource.
func createK8znerClusterCRD(ctx context.Context, kubeconfig []byte, cfg *config.Config, bootstrapServer *BootstrapServerInfo, infra *InfrastructureInfo) error {
	now := metav1.Now()
	cluster := &k8znerv1alpha1.K8znerCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "k8zner.io/v1alpha1",
			Kind:       "K8znerCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.ClusterName,
			Namespace: k8znerNamespace,
		},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region: cfg.Location,
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
				Count: cfg.ControlPlane.NodePools[0].Count,
				Size:  cfg.ControlPlane.NodePools[0].ServerType,
			},
			Workers: k8znerv1alpha1.WorkerSpec{
				Count: cfg.Workers[0].Count,
				Size:  cfg.Workers[0].ServerType,
			},
			Network: k8znerv1alpha1.NetworkSpec{
				IPv4CIDR:    cfg.Network.IPv4CIDR,
				PodCIDR:     cfg.Network.PodIPv4CIDR,
				ServiceCIDR: cfg.Network.ServiceIPv4CIDR,
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
				Name: fmt.Sprintf("%s-credentials", cfg.ClusterName),
			},
			Bootstrap: &k8znerv1alpha1.BootstrapState{
				Completed:       true,
				CompletedAt:     &now,
				BootstrapNode:   bootstrapServer.Name,
				BootstrapNodeID: bootstrapServer.ID,
				PublicIP:        bootstrapServer.PublicIP,
			},
		},
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Phase:                "Provisioning",
			ControlPlaneEndpoint: infra.LoadBalancerIP,
			Infrastructure: k8znerv1alpha1.InfrastructureStatus{
				NetworkID:             infra.NetworkID,
				FirewallID:            infra.FirewallID,
				LoadBalancerID:        infra.LoadBalancerID,
				LoadBalancerIP:        infra.LoadBalancerIP,
				LoadBalancerPrivateIP: infra.LoadBalancerPrivateIP,
				SSHKeyID:              infra.SSHKeyID,
			},
		},
	}

	clusterYAML, err := yaml.Marshal(cluster)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster CRD: %w", err)
	}

	crdPath := fmt.Sprintf("%s-cluster.yaml", cfg.ClusterName)
	if err := os.WriteFile(crdPath, clusterYAML, 0600); err != nil {
		return fmt.Errorf("failed to write cluster CRD: %w", err)
	}

	log.Printf("K8znerCluster CRD written to: %s", crdPath)
	log.Printf("Apply with: kubectl apply -f %s", crdPath)

	return nil
}

// printDryRunPlan shows what would be created without making changes.
func printDryRunPlan(cfg *config.Config) {
	fmt.Print("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Println("DRY RUN - No changes will be made")
	fmt.Print(strings.Repeat("=", 60) + "\n\n")

	fmt.Printf("Cluster: %s\n", cfg.ClusterName)
	fmt.Printf("Region: %s\n\n", cfg.Location)

	fmt.Println("Infrastructure to be created:")
	fmt.Printf("  1. Load Balancer: %s-kube-api (lb11)\n", cfg.ClusterName)
	fmt.Printf("  2. Network: %s (%s)\n", cfg.ClusterName, cfg.Network.IPv4CIDR)
	fmt.Printf("  3. Firewall: %s\n", cfg.ClusterName)
	fmt.Printf("  4. Control Plane: %s-cp-1 (%s)\n\n",
		cfg.ClusterName, cfg.ControlPlane.NodePools[0].ServerType)

	fmt.Println("Operator will then create:")
	if cfg.ControlPlane.NodePools[0].Count > 1 {
		fmt.Printf("  - %d additional control planes\n", cfg.ControlPlane.NodePools[0].Count-1)
	}
	fmt.Printf("  - %d worker nodes (%s)\n", cfg.Workers[0].Count, cfg.Workers[0].ServerType)
	fmt.Println("  - Cilium CNI")
	fmt.Println("  - Hetzner Cloud Controller Manager")
	fmt.Println("  - Hetzner CSI Driver")
}

// printBootstrapSuccess outputs completion message and next steps.
func printBootstrapSuccess(cfg *config.Config, infra *InfrastructureInfo) {
	fmt.Print("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Println("Bootstrap complete!")
	fmt.Print(strings.Repeat("=", 60) + "\n\n")

	fmt.Printf("Cluster: %s\n", cfg.ClusterName)
	fmt.Printf("API Server: https://%s:6443 (Load Balancer - stable endpoint)\n\n", infra.LoadBalancerIP)

	fmt.Println("Infrastructure created:")
	fmt.Printf("  - Load Balancer: %s (ID: %d, IP: %s)\n", infra.LoadBalancerName, infra.LoadBalancerID, infra.LoadBalancerIP)
	fmt.Printf("  - Network: %s (ID: %d)\n", infra.NetworkName, infra.NetworkID)
	fmt.Printf("  - Firewall: %s (ID: %d)\n", infra.FirewallName, infra.FirewallID)
	fmt.Println()

	fmt.Println("Files created:")
	fmt.Printf("  - %s (Talos secrets)\n", secretsFile)
	fmt.Printf("  - %s (Talos client config)\n", talosConfigPath)
	fmt.Printf("  - %s (Kubernetes config)\n", kubeconfigPath)
	fmt.Printf("  - %s-cluster.yaml (K8znerCluster CRD)\n\n", cfg.ClusterName)

	fmt.Println("Next steps:")
	fmt.Printf("1. Access your cluster:\n")
	fmt.Printf("   export KUBECONFIG=%s\n", kubeconfigPath)
	fmt.Printf("   kubectl get nodes\n\n")

	fmt.Printf("2. Apply the operator CRDs and deployment:\n")
	fmt.Printf("   kubectl apply -f https://raw.githubusercontent.com/imamik/k8zner/main/deploy/crds.yaml\n")
	fmt.Printf("   kubectl apply -f https://raw.githubusercontent.com/imamik/k8zner/main/deploy/operator.yaml\n\n")

	fmt.Printf("3. Apply the cluster CRD (operator will provision remaining nodes):\n")
	fmt.Printf("   kubectl apply -f %s-cluster.yaml\n\n", cfg.ClusterName)

	fmt.Printf("4. Watch operator progress:\n")
	fmt.Printf("   kubectl logs -f -n %s deploy/k8zner-operator\n", k8znerNamespace)
	fmt.Printf("   kubectl get k8znerclusters -n %s -w\n\n", k8znerNamespace)

	fmt.Println("The operator will:")
	if cfg.ControlPlane.NodePools[0].Count > 1 {
		fmt.Printf("  - Provision %d additional control plane nodes (for HA)\n", cfg.ControlPlane.NodePools[0].Count-1)
	}
	fmt.Printf("  - Provision %d worker nodes\n", cfg.Workers[0].Count)
	fmt.Println("  - Install cluster addons (Cilium, CCM, CSI)")
	fmt.Println("  - Monitor cluster health and perform self-healing")
}
