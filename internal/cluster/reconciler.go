// Package cluster provides the reconciliation logic for provisioning and managing Hetzner Cloud resources.
package cluster

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
)

// TalosConfigProducer defines the interface for generating Talos configurations.
type TalosConfigProducer interface {
	GenerateControlPlaneConfig(san []string) ([]byte, error)
	GenerateWorkerConfig() ([]byte, error)
	GetClientConfig() ([]byte, error)
	SetEndpoint(endpoint string)
}

// Reconciler is responsible for reconciling the state of the cluster.
type Reconciler struct {
	serverProvisioner hcloud_internal.ServerProvisioner
	networkManager    hcloud_internal.NetworkManager
	firewallManager   hcloud_internal.FirewallManager
	lbManager         hcloud_internal.LoadBalancerManager
	pgManager         hcloud_internal.PlacementGroupManager
	fipManager        hcloud_internal.FloatingIPManager
	certManager       hcloud_internal.CertificateManager
	snapshotManager   hcloud_internal.SnapshotManager
	infra             hcloud_internal.InfrastructureManager // Combined interface for Bootstrapper
	talosGenerator    TalosConfigProducer
	config            *config.Config
	bootstrapper      *Bootstrapper
	timeouts          *config.Timeouts

	// State
	network  *hcloud.Network
	firewall *hcloud.Firewall
}

// NewReconciler creates a new Reconciler.
func NewReconciler(
	infra hcloud_internal.InfrastructureManager,
	talosGenerator TalosConfigProducer,
	cfg *config.Config,
) *Reconciler {
	return &Reconciler{
		serverProvisioner: infra,
		networkManager:    infra,
		firewallManager:   infra,
		lbManager:         infra,
		pgManager:         infra,
		fipManager:        infra,
		certManager:       infra,
		snapshotManager:   infra,
		infra:             infra,
		talosGenerator:    talosGenerator,
		config:            cfg,
		bootstrapper:      NewBootstrapper(infra),
		timeouts:          config.LoadTimeouts(),
	}
}

// Reconcile ensures that the desired state matches the actual state.
// Returns the kubeconfig bytes if bootstrap was performed, or nil if cluster already existed.
func (r *Reconciler) Reconcile(ctx context.Context) ([]byte, error) {
	log.Println("Starting reconciliation...")

	// 0. Calculate Subnets.
	if err := r.config.CalculateSubnets(); err != nil {
		return nil, fmt.Errorf("failed to calculate subnets: %w", err)
	}

	// 1. Network.
	if err := r.reconcileNetwork(ctx); err != nil {
		return nil, fmt.Errorf("failed to reconcile network: %w", err)
	}

	// 1.5. Pre-build all required Talos images in parallel with public IP fetch
	var publicIP string
	imageTasks := []Task{
		{
			Name: "images",
			Func: r.ensureAllImages,
		},
		{
			Name: "publicIP",
			Func: func(ctx context.Context) error {
				ip, err := r.infra.GetPublicIP(ctx)
				if err == nil {
					publicIP = ip
					return nil
				}
				log.Printf("Warning: Failed to detect public IP: %v", err)
				// Don't fail on public IP detection error
				return nil
			},
		},
	}

	if err := RunParallel(ctx, imageTasks, false); err != nil {
		return nil, err
	}

	// 2-5. Parallelize infrastructure setup after network
	log.Printf("=== PARALLELIZING INFRASTRUCTURE SETUP at %s ===", time.Now().Format("15:04:05"))

	infraTasks := []Task{
		{
			Name: "firewall",
			Func: func(ctx context.Context) error {
				return r.reconcileFirewall(ctx, publicIP)
			},
		},
		{Name: "loadBalancers", Func: r.reconcileLoadBalancers},
		{Name: "floatingIPs", Func: r.reconcileFloatingIPs},
	}

	if err := RunParallel(ctx, infraTasks, true); err != nil {
		return nil, err
	}

	log.Printf("=== INFRASTRUCTURE SETUP COMPLETE at %s ===", time.Now().Format("15:04:05"))

	// 6. Control Plane Servers
	cpIPs, err := r.reconcileControlPlane(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile control plane: %w", err)
	}

	// 7. Bootstrap and retrieve kubeconfig
	var kubeconfig []byte
	var clientCfg []byte
	if len(cpIPs) > 0 {
		var err error
		clientCfg, err = r.talosGenerator.GetClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get client config: %w", err)
		}

		// Generate machine configs for each control plane node
		// For now, all control plane nodes get the same config
		// In the future, we could customize per-node if needed
		cpConfig, err := r.talosGenerator.GenerateControlPlaneConfig(nil) // SANs already set during reconcileControlPlane
		if err != nil {
			return nil, fmt.Errorf("failed to generate control plane config: %w", err)
		}

		machineConfigs := make(map[string][]byte)
		for name := range cpIPs {
			machineConfigs[name] = cpConfig
		}

		kubeconfig, err = r.bootstrapper.Bootstrap(ctx, r.config.ClusterName, cpIPs, machineConfigs, clientCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to bootstrap cluster: %w", err)
		}
	}

	// 8. Worker Servers
	workerIPs, workerConfig, err := r.reconcileWorkers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile workers: %w", err)
	}

	// 8a. Apply worker configurations (if workers exist and cluster is bootstrapped)
	if len(workerIPs) > 0 && len(kubeconfig) > 0 {
		log.Printf("Applying Talos configurations to %d worker nodes...", len(workerIPs))
		if err := r.bootstrapper.ApplyWorkerConfigs(ctx, workerIPs, workerConfig, clientCfg); err != nil {
			return nil, fmt.Errorf("failed to apply worker configs: %w", err)
		}
	}

	// 9. Install Addons (if cluster was bootstrapped with kubeconfig)
	if len(kubeconfig) > 0 {
		// Write kubeconfig to temp file for kubectl
		tmpKubeconfig, err := os.CreateTemp("", "kubeconfig-*.yaml")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp kubeconfig: %w", err)
		}
		defer os.Remove(tmpKubeconfig.Name())

		if _, err := tmpKubeconfig.Write(kubeconfig); err != nil {
			return nil, fmt.Errorf("failed to write temp kubeconfig: %w", err)
		}
		tmpKubeconfig.Close()

		// Install CCM if enabled
		if r.config.Addons.CCM.Enabled {
			log.Println("Installing Hetzner Cloud Controller Manager (CCM)...")
			templateData := map[string]string{
				"Token":     r.config.HCloudToken,
				"NetworkID": fmt.Sprintf("%d", r.network.ID),
			}
			if err := r.applyManifests(ctx, "hcloud-ccm", tmpKubeconfig.Name(), templateData); err != nil {
				return nil, fmt.Errorf("failed to install CCM addon: %w", err)
			}
			log.Println("CCM installation completed")
		}
	}

	return kubeconfig, nil
}
