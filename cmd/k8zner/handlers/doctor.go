package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/config"
	hcloudInternal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/ui/tui"
	"github.com/imamik/k8zner/internal/util/naming"
)

// DoctorStatus represents the cluster diagnostic status.
type DoctorStatus struct {
	ClusterName    string                 `json:"clusterName"`
	Region         string                 `json:"region"`
	Phase          string                 `json:"phase"`
	Provisioning   string                 `json:"provisioning,omitempty"`
	Infrastructure InfrastructureHealth   `json:"infrastructure"`
	ControlPlanes  NodeGroupHealth        `json:"controlPlanes"`
	Workers        NodeGroupHealth        `json:"workers"`
	Addons         map[string]AddonHealth `json:"addons"`
	Connectivity   ConnectivityHealth     `json:"connectivity,omitempty"`
}

// InfrastructureHealth represents infrastructure component status.
type InfrastructureHealth struct {
	Network        bool   `json:"network"`
	Firewall       bool   `json:"firewall"`
	LoadBalancer   bool   `json:"loadBalancer"`
	LoadBalancerIP string `json:"loadBalancerIP,omitempty"`
}

// ConnectivityHealth represents connectivity probe results.
type ConnectivityHealth struct {
	KubeAPI    bool            `json:"kubeAPI"`
	MetricsAPI bool            `json:"metricsAPI"`
	Endpoints  []EndpointCheck `json:"endpoints,omitempty"`
}

// EndpointCheck represents external endpoint health.
type EndpointCheck struct {
	Host    string `json:"host"`
	DNS     bool   `json:"dns"`
	TLS     bool   `json:"tls"`
	HTTP    bool   `json:"http"`
	Message string `json:"message,omitempty"`
}

// NodeGroupHealth represents control plane or worker status.
type NodeGroupHealth struct {
	Desired   int `json:"desired"`
	Ready     int `json:"ready"`
	Unhealthy int `json:"unhealthy"`
}

// AddonHealth represents addon status.
type AddonHealth struct {
	Installed bool   `json:"installed"`
	Healthy   bool   `json:"healthy"`
	Phase     string `json:"phase,omitempty"`
	Message   string `json:"message,omitempty"`
}

// Doctor handles the doctor command.
//
// Pre-cluster mode: validates config and checks Hetzner API connectivity.
// Cluster mode: shows live status from K8znerCluster CRD.
func Doctor(ctx context.Context, configPath string, watch, jsonOutput bool) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Pre-cluster mode: no kubeconfig
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return doctorPreCluster(cfg, jsonOutput)
	}

	// Cluster mode: kubeconfig exists
	kubecfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(kubecfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	if watch {
		// Use TUI for interactive terminals (unless JSON output requested)
		if !jsonOutput && isInteractiveTTY() {
			return tui.RunDoctorTUI(ctx, k8sClient, cfg.ClusterName)
		}
		return doctorWatch(ctx, k8sClient, cfg.ClusterName, jsonOutput)
	}

	// Single render: use styled output for TTY, plain for non-TTY
	if !jsonOutput && isInteractiveTTY() {
		return doctorShowStyled(ctx, k8sClient, cfg)
	}

	return doctorShow(ctx, k8sClient, cfg.ClusterName, jsonOutput)
}

// doctorPreCluster shows diagnostic info when no cluster exists.
// It probes the Hetzner Cloud API to check if infrastructure resources exist.
func doctorPreCluster(cfg *config.Config, jsonOutput bool) error {
	status := &DoctorStatus{
		ClusterName: cfg.ClusterName,
		Region:      cfg.Location,
		Phase:       "Not Created",
	}

	// Probe hcloud API for existing infrastructure
	token := os.Getenv("HCLOUD_TOKEN")
	if token != "" {
		infraClient := newInfraClient(token)
		status.Infrastructure = probeInfraHealth(context.Background(), infraClient, cfg.ClusterName)
		// If any infra exists, it's a partial provisioning
		if status.Infrastructure.Network || status.Infrastructure.Firewall || status.Infrastructure.LoadBalancer {
			status.Phase = "Provisioning"
		}
	}

	if jsonOutput {
		return printDoctorJSON(status)
	}

	fmt.Println()
	printHeader(cfg.ClusterName, cfg.Location)

	if status.Phase == "Provisioning" {
		fmt.Println("  Status: Provisioning (pre-kubeconfig)")
		fmt.Println()
		fmt.Println("  Infrastructure")
		fmt.Println("  " + strings.Repeat("─", 35))
		printRow("Network", status.Infrastructure.Network, "")
		printRow("Firewall", status.Infrastructure.Firewall, "")
		lbExtra := ""
		if status.Infrastructure.LoadBalancerIP != "" {
			lbExtra = status.Infrastructure.LoadBalancerIP
		}
		printRow("Load Balancer", status.Infrastructure.LoadBalancer, lbExtra)
	} else {
		fmt.Println("  Status: Not created")
	}
	fmt.Println()

	fmt.Println("  Configuration:")
	if len(cfg.ControlPlane.NodePools) > 0 {
		pool := cfg.ControlPlane.NodePools[0]
		fmt.Printf("    Control Planes: %d x %s\n", pool.Count, pool.ServerType)
	}
	if len(cfg.Workers) > 0 {
		pool := cfg.Workers[0]
		fmt.Printf("    Workers:        %d x %s\n", pool.Count, pool.ServerType)
	}
	fmt.Printf("    Kubernetes:     %s\n", cfg.Kubernetes.Version)
	fmt.Printf("    Talos:          %s\n", cfg.Talos.Version)

	fmt.Println()
	if status.Phase == "Not Created" {
		fmt.Println("  Run 'k8zner apply' to create the cluster.")
	} else {
		fmt.Println("  Infrastructure partially created. Run 'k8zner apply' to continue or 'k8zner destroy' to clean up.")
	}
	fmt.Println()

	return nil
}

// probeInfraHealth checks hcloud API for existing infrastructure resources.
func probeInfraHealth(ctx context.Context, infraClient hcloudInternal.InfrastructureManager, clusterName string) InfrastructureHealth {
	health := InfrastructureHealth{}

	networkName := naming.Network(clusterName)
	if nw, err := infraClient.GetNetwork(ctx, networkName); err == nil && nw != nil {
		health.Network = true
	}

	fwName := naming.Firewall(clusterName)
	if fw, err := infraClient.GetFirewall(ctx, fwName); err == nil && fw != nil {
		health.Firewall = true
	}

	lbName := naming.KubeAPILoadBalancer(clusterName)
	if lb, err := infraClient.GetLoadBalancer(ctx, lbName); err == nil && lb != nil {
		health.LoadBalancer = true
		if lb.PublicNet.Enabled && lb.PublicNet.IPv4.IP != nil {
			health.LoadBalancerIP = lb.PublicNet.IPv4.IP.String()
		}
	}

	return health
}

// doctorShow displays the current cluster status once.
func doctorShow(ctx context.Context, k8sClient client.Client, clusterName string, jsonOutput bool) error {
	status, err := getClusterStatus(ctx, k8sClient, clusterName)
	if err != nil {
		return err
	}

	if jsonOutput {
		return printDoctorJSON(status)
	}

	return printDoctorFormatted(status)
}

// doctorWatch continuously displays cluster status.
func doctorWatch(ctx context.Context, k8sClient client.Client, clusterName string, jsonOutput bool) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	if err := doctorShow(ctx, k8sClient, clusterName, jsonOutput); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if !jsonOutput {
				fmt.Print("\033[H\033[2J")
			}
			if err := doctorShow(ctx, k8sClient, clusterName, jsonOutput); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		}
	}
}

// getClusterStatus retrieves the cluster status from the K8znerCluster CRD.
func getClusterStatus(ctx context.Context, k8sClient client.Client, clusterName string) (*DoctorStatus, error) {
	k8zCluster := &k8znerv1alpha1.K8znerCluster{}
	key := client.ObjectKey{
		Namespace: k8znerNamespace,
		Name:      clusterName,
	}

	if err := k8sClient.Get(ctx, key, k8zCluster); err != nil {
		return &DoctorStatus{
			ClusterName: clusterName,
			Phase:       "Unknown",
		}, nil
	}

	infra := k8zCluster.Status.Infrastructure

	// Use *Ready booleans if available, fall back to ID != 0
	networkReady := infra.NetworkReady || infra.NetworkID != 0
	firewallReady := infra.FirewallReady || infra.FirewallID != 0
	lbReady := infra.LoadBalancerReady || infra.LoadBalancerID != 0

	// Map connectivity status
	var connectivity ConnectivityHealth
	conn := k8zCluster.Status.Connectivity
	connectivity.KubeAPI = conn.KubeAPIReady
	connectivity.MetricsAPI = conn.MetricsAPIReady
	for _, ep := range conn.Endpoints {
		connectivity.Endpoints = append(connectivity.Endpoints, EndpointCheck{
			Host:    ep.Host,
			DNS:     ep.DNSReady,
			TLS:     ep.TLSReady,
			HTTP:    ep.HTTPReady,
			Message: ep.Message,
		})
	}

	return &DoctorStatus{
		ClusterName:  clusterName,
		Region:       k8zCluster.Spec.Region,
		Phase:        string(k8zCluster.Status.Phase),
		Provisioning: string(k8zCluster.Status.ProvisioningPhase),
		Infrastructure: InfrastructureHealth{
			Network:        networkReady,
			Firewall:       firewallReady,
			LoadBalancer:   lbReady,
			LoadBalancerIP: infra.LoadBalancerIP,
		},
		ControlPlanes: NodeGroupHealth{
			Desired:   k8zCluster.Status.ControlPlanes.Desired,
			Ready:     k8zCluster.Status.ControlPlanes.Ready,
			Unhealthy: k8zCluster.Status.ControlPlanes.Unhealthy,
		},
		Workers: NodeGroupHealth{
			Desired:   k8zCluster.Status.Workers.Desired,
			Ready:     k8zCluster.Status.Workers.Ready,
			Unhealthy: k8zCluster.Status.Workers.Unhealthy,
		},
		Addons:       buildAddonHealth(k8zCluster.Status.Addons),
		Connectivity: connectivity,
	}, nil
}

// buildAddonHealth converts CRD addon status to health format.
func buildAddonHealth(addons map[string]k8znerv1alpha1.AddonStatus) map[string]AddonHealth {
	result := make(map[string]AddonHealth)
	for name, status := range addons {
		result[name] = AddonHealth{
			Installed: status.Installed,
			Healthy:   status.Healthy,
			Phase:     string(status.Phase),
			Message:   status.Message,
		}
	}
	return result
}

// printDoctorJSON outputs status as JSON.
func printDoctorJSON(status *DoctorStatus) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// printDoctorFormatted outputs status as a formatted ASCII table with emoji.
func printDoctorFormatted(status *DoctorStatus) error {
	fmt.Println()
	printHeader(status.ClusterName, status.Region)

	// Phase
	phaseEmoji := phaseIndicator(status.Phase)
	fmt.Printf("  %s Status: %s", phaseEmoji, status.Phase)
	if status.Provisioning != "" && status.Phase != "Running" {
		fmt.Printf(" (%s)", status.Provisioning)
	}
	fmt.Println()
	fmt.Println()

	// Infrastructure
	fmt.Println("  Infrastructure")
	fmt.Println("  " + strings.Repeat("─", 35))
	printRow("Network", status.Infrastructure.Network, "")
	printRow("Firewall", status.Infrastructure.Firewall, "")
	printRow("Load Balancer", status.Infrastructure.LoadBalancer, "")
	fmt.Println()

	// Control Planes
	cpReady := status.ControlPlanes.Ready == status.ControlPlanes.Desired && status.ControlPlanes.Desired > 0
	cpExtra := fmt.Sprintf("%d/%d", status.ControlPlanes.Ready, status.ControlPlanes.Desired)
	fmt.Println("  Nodes")
	fmt.Println("  " + strings.Repeat("─", 35))
	printRow("Control Planes", cpReady, cpExtra)

	wReady := status.Workers.Ready == status.Workers.Desired && status.Workers.Desired > 0
	wExtra := fmt.Sprintf("%d/%d", status.Workers.Ready, status.Workers.Desired)
	printRow("Workers", wReady, wExtra)
	fmt.Println()

	// Addons
	if len(status.Addons) > 0 {
		fmt.Println("  Addons")
		fmt.Println("  " + strings.Repeat("─", 35))

		addonOrder := []string{
			k8znerv1alpha1.AddonNameCilium,
			k8znerv1alpha1.AddonNameCCM,
			k8znerv1alpha1.AddonNameCSI,
			k8znerv1alpha1.AddonNameMetricsServer,
			k8znerv1alpha1.AddonNameCertManager,
			k8znerv1alpha1.AddonNameTraefik,
			k8znerv1alpha1.AddonNameExternalDNS,
			k8znerv1alpha1.AddonNameArgoCD,
			k8znerv1alpha1.AddonNameMonitoring,
			k8znerv1alpha1.AddonNameTalosBackup,
		}

		printed := make(map[string]bool)
		for _, name := range addonOrder {
			if addon, ok := status.Addons[name]; ok {
				extra := addonExtra(addon)
				printRow(name, addon.Healthy, extra)
				printed[name] = true
			}
		}

		for name, addon := range status.Addons {
			if printed[name] {
				continue
			}
			extra := addonExtra(addon)
			printRow(name, addon.Healthy, extra)
		}
	}

	// Connectivity
	if status.Connectivity.KubeAPI || len(status.Connectivity.Endpoints) > 0 {
		fmt.Println()
		fmt.Println("  Connectivity")
		fmt.Println("  " + strings.Repeat("─", 35))
		printRow("Kube API", status.Connectivity.KubeAPI, "")
		printRow("Metrics API", status.Connectivity.MetricsAPI, "")
		for _, ep := range status.Connectivity.Endpoints {
			allOK := ep.DNS && ep.TLS && ep.HTTP
			extra := ""
			if !allOK {
				parts := []string{}
				if !ep.DNS {
					parts = append(parts, "DNS")
				}
				if !ep.TLS {
					parts = append(parts, "TLS")
				}
				if !ep.HTTP {
					parts = append(parts, "HTTP")
				}
				extra = "missing: " + strings.Join(parts, ",")
			}
			printRow(ep.Host, allOK, extra)
		}
	}

	fmt.Println()
	return nil
}

func printHeader(clusterName, region string) {
	title := fmt.Sprintf("k8zner cluster: %s", clusterName)
	if region != "" {
		title += fmt.Sprintf(" (%s)", region)
	}
	fmt.Printf("  %s\n", title)
	fmt.Println("  " + strings.Repeat("═", len(title)))
	fmt.Println()
}

func phaseIndicator(phase string) string {
	switch phase {
	case "Running":
		return "\u2705" // green check
	case "Provisioning":
		return "\u23f3" // hourglass
	case "Failed", "Error":
		return "\u274c" // red X
	default:
		return "\u2753" // question mark
	}
}

func printRow(name string, ready bool, extra string) {
	indicator := "\u2705" // green check
	if !ready {
		indicator = "\u274c" // red X
	}

	if extra != "" {
		fmt.Printf("  %s  %-20s %s\n", indicator, name, extra)
	} else {
		fmt.Printf("  %s  %s\n", indicator, name)
	}
}

func isInteractiveTTY() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// doctorShowStyled renders doctor output once using TUI styles.
func doctorShowStyled(ctx context.Context, k8sClient client.Client, cfg *config.Config) error {
	cluster := &k8znerv1alpha1.K8znerCluster{}
	key := client.ObjectKey{
		Namespace: k8znerNamespace,
		Name:      cfg.ClusterName,
	}

	if err := k8sClient.Get(ctx, key, cluster); err != nil {
		return fmt.Errorf("failed to get cluster: %w", err)
	}

	lastReconcile := ""
	if cluster.Status.LastReconcileTime != nil {
		lastReconcile = time.Since(cluster.Status.LastReconcileTime.Time).Round(time.Second).String() + " ago"
	}

	msg := tui.CRDStatusMsg{
		ClusterPhase:   cluster.Status.Phase,
		ProvPhase:      cluster.Status.ProvisioningPhase,
		Infrastructure: cluster.Status.Infrastructure,
		ControlPlanes:  cluster.Status.ControlPlanes,
		Workers:        cluster.Status.Workers,
		Addons:         cluster.Status.Addons,
		PhaseHistory:   cluster.Status.PhaseHistory,
		LastErrors:     cluster.Status.LastErrors,
		LastReconcile:  lastReconcile,
	}

	fmt.Println(tui.RenderDoctorOnce(msg, cfg.ClusterName, cfg.Location))
	return nil
}

// doctorSummaryLine returns a compact one-line summary of doctor status for log output.
func doctorSummaryLine(status *DoctorStatus) string {
	infraOK := 0
	if status.Infrastructure.Network {
		infraOK++
	}
	if status.Infrastructure.Firewall {
		infraOK++
	}
	if status.Infrastructure.LoadBalancer {
		infraOK++
	}

	addonsTotal := 0
	addonsHealthy := 0
	for _, a := range status.Addons {
		if a.Installed {
			addonsTotal++
			if a.Healthy {
				addonsHealthy++
			}
		}
	}

	epTotal := len(status.Connectivity.Endpoints)
	epOK := 0
	for _, ep := range status.Connectivity.Endpoints {
		if ep.DNS && ep.TLS && ep.HTTP {
			epOK++
		}
	}

	parts := []string{
		fmt.Sprintf("infra=%d/3", infraOK),
		fmt.Sprintf("cp=%d/%d", status.ControlPlanes.Ready, status.ControlPlanes.Desired),
		fmt.Sprintf("workers=%d/%d", status.Workers.Ready, status.Workers.Desired),
	}
	if addonsTotal > 0 {
		parts = append(parts, fmt.Sprintf("addons=%d/%d", addonsHealthy, addonsTotal))
	}
	if epTotal > 0 {
		parts = append(parts, fmt.Sprintf("endpoints=%d/%d", epOK, epTotal))
	}

	return "[" + strings.Join(parts, " ") + "]"
}

func addonExtra(addon AddonHealth) string {
	if !addon.Installed && addon.Phase == "" {
		return ""
	}
	if addon.Phase != "" && addon.Phase != string(k8znerv1alpha1.AddonPhaseInstalled) {
		return addon.Phase
	}
	return ""
}
