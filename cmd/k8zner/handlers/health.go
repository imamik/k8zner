package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

// HealthStatus represents the cluster health for JSON output.
type HealthStatus struct {
	ClusterName    string                 `json:"clusterName"`
	Region         string                 `json:"region"`
	Phase          string                 `json:"phase"`
	Infrastructure InfrastructureHealth   `json:"infrastructure"`
	ControlPlanes  NodeGroupHealth        `json:"controlPlanes"`
	Workers        NodeGroupHealth        `json:"workers"`
	Addons         map[string]AddonHealth `json:"addons"`
}

// InfrastructureHealth represents infrastructure component status.
type InfrastructureHealth struct {
	Network      bool `json:"network"`
	Firewall     bool `json:"firewall"`
	LoadBalancer bool `json:"loadBalancer"`
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

// Health handles the health command.
//
// This function displays the current status of the cluster including
// infrastructure, control planes, workers, and addons.
func Health(ctx context.Context, configPath string, watch, jsonOutput bool) error {
	// Load config to get cluster name
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	// Check if kubeconfig exists
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return fmt.Errorf("kubeconfig not found at %s - is the cluster created?", kubeconfigPath)
	}

	// Load kubeconfig
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

	if watch {
		return watchHealth(ctx, k8sClient, cfg.ClusterName, jsonOutput)
	}

	return showHealth(ctx, k8sClient, cfg.ClusterName, jsonOutput)
}

// showHealth displays the current cluster health once.
func showHealth(ctx context.Context, k8sClient client.Client, clusterName string, jsonOutput bool) error {
	status, err := getClusterHealth(ctx, k8sClient, clusterName)
	if err != nil {
		return err
	}

	if jsonOutput {
		return printHealthJSON(status)
	}

	return printHealthFormatted(status)
}

// watchHealth continuously displays cluster health.
func watchHealth(ctx context.Context, k8sClient client.Client, clusterName string, jsonOutput bool) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Show immediately first
	if err := showHealth(ctx, k8sClient, clusterName, jsonOutput); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Clear screen for non-JSON output
			if !jsonOutput {
				fmt.Print("\033[H\033[2J")
			}
			if err := showHealth(ctx, k8sClient, clusterName, jsonOutput); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		}
	}
}

// getClusterHealth retrieves the cluster health status.
func getClusterHealth(ctx context.Context, k8sClient client.Client, clusterName string) (*HealthStatus, error) {
	cluster := &k8znerv1alpha1.K8znerCluster{}
	key := client.ObjectKey{
		Namespace: k8znerNamespace,
		Name:      clusterName,
	}

	if err := k8sClient.Get(ctx, key, cluster); err != nil {
		// Try to build status from raw infrastructure if CRD doesn't exist
		return buildBasicHealth(clusterName), nil
	}

	return &HealthStatus{
		ClusterName: clusterName,
		Region:      cluster.Spec.Region,
		Phase:       string(cluster.Status.Phase),
		Infrastructure: InfrastructureHealth{
			Network:      cluster.Status.Infrastructure.NetworkID != 0,
			Firewall:     cluster.Status.Infrastructure.FirewallID != 0,
			LoadBalancer: cluster.Status.Infrastructure.LoadBalancerID != 0,
		},
		ControlPlanes: NodeGroupHealth{
			Desired:   cluster.Status.ControlPlanes.Desired,
			Ready:     cluster.Status.ControlPlanes.Ready,
			Unhealthy: cluster.Status.ControlPlanes.Unhealthy,
		},
		Workers: NodeGroupHealth{
			Desired:   cluster.Status.Workers.Desired,
			Ready:     cluster.Status.Workers.Ready,
			Unhealthy: cluster.Status.Workers.Unhealthy,
		},
		Addons: buildAddonHealth(cluster.Status.Addons),
	}, nil
}

// buildBasicHealth creates a basic health status when CRD is not available.
func buildBasicHealth(clusterName string) *HealthStatus {
	return &HealthStatus{
		ClusterName: clusterName,
		Phase:       "Unknown",
		Infrastructure: InfrastructureHealth{
			Network:      false,
			Firewall:     false,
			LoadBalancer: false,
		},
	}
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

// printHealthJSON outputs health status as JSON.
func printHealthJSON(status *HealthStatus) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal health status: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// printHealthFormatted outputs health status in a formatted display.
func printHealthFormatted(status *HealthStatus) error {
	fmt.Printf("k8zner cluster: %s (%s)\n", status.ClusterName, status.Region)
	fmt.Println("─────────────────────────────────────")
	fmt.Println()

	// Infrastructure
	fmt.Println("Infrastructure:")
	printStatusLine("Network", status.Infrastructure.Network, "")
	printStatusLine("Firewall", status.Infrastructure.Firewall, "")
	printStatusLine("Load Balancer", status.Infrastructure.LoadBalancer, "")

	// Control Planes
	cpReady := status.ControlPlanes.Ready == status.ControlPlanes.Desired
	cpStatus := fmt.Sprintf("(%d/%d)", status.ControlPlanes.Ready, status.ControlPlanes.Desired)
	printStatusLine("Control Planes", cpReady, cpStatus)

	// Workers
	wReady := status.Workers.Ready == status.Workers.Desired
	wStatus := fmt.Sprintf("(%d/%d)", status.Workers.Ready, status.Workers.Desired)
	printStatusLine("Workers", wReady, wStatus)

	fmt.Println()

	// Addons
	if len(status.Addons) > 0 {
		fmt.Println("Addons:")
		// Print in order
		addonOrder := []string{
			k8znerv1alpha1.AddonNameCilium,
			k8znerv1alpha1.AddonNameCCM,
			k8znerv1alpha1.AddonNameCSI,
			k8znerv1alpha1.AddonNameMetricsServer,
			k8znerv1alpha1.AddonNameCertManager,
			k8znerv1alpha1.AddonNameTraefik,
			k8znerv1alpha1.AddonNameExternalDNS,
			k8znerv1alpha1.AddonNameArgoCD,
		}

		for _, name := range addonOrder {
			if addon, ok := status.Addons[name]; ok {
				extra := ""
				if addon.Phase != "" && addon.Phase != string(k8znerv1alpha1.AddonPhaseInstalled) {
					extra = fmt.Sprintf("(%s)", addon.Phase)
				}
				printStatusLine(name, addon.Healthy, extra)
			}
		}

		// Print any addons not in the standard order
		for name, addon := range status.Addons {
			found := false
			for _, orderedName := range addonOrder {
				if name == orderedName {
					found = true
					break
				}
			}
			if !found {
				extra := ""
				if addon.Phase != "" && addon.Phase != string(k8znerv1alpha1.AddonPhaseInstalled) {
					extra = fmt.Sprintf("(%s)", addon.Phase)
				}
				printStatusLine(name, addon.Healthy, extra)
			}
		}
	}

	fmt.Println()
	fmt.Printf("Status: %s\n", status.Phase)

	return nil
}

// printStatusLine prints a single status line with indicator.
func printStatusLine(name string, ready bool, extra string) {
	var indicator string
	switch {
	case ready:
		indicator = "✓"
	case extra != "" && (extra == "(installing...)" || extra == "(Installing)"):
		indicator = "◐"
	default:
		indicator = "○"
	}

	if extra != "" {
		fmt.Printf("  %s %s %s\n", indicator, name, extra)
	} else {
		fmt.Printf("  %s %s\n", indicator, name)
	}
}
