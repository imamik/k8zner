// Package cluster provides the reconciliation logic for provisioning and managing Hetzner Cloud resources.
package cluster

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/addons"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/k8s"
)

// TalosConfigProducer defines the interface for generating Talos configurations.
type TalosConfigProducer interface {
	GenerateControlPlaneConfig(san []string) ([]byte, error)
	GenerateWorkerConfig() ([]byte, error)
	GetClientConfig() ([]byte, error)
	GetKubeconfig(ctx context.Context, nodeIP string) ([]byte, error)
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
	addonManager      addons.AddonManager // Optional, set during reconcileAddons
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

// SetAddonManager injects a custom addon manager (mostly for testing).
func (r *Reconciler) SetAddonManager(mgr addons.AddonManager) {
	r.addonManager = mgr
}

// Reconcile ensures that the desired state matches the actual state.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	log.Println("Starting reconciliation...")

	// 0. Calculate Subnets.
	if err := r.config.CalculateSubnets(); err != nil {
		return fmt.Errorf("failed to calculate subnets: %w", err)
	}

	// 1. Network.
	if err := r.reconcileNetwork(ctx); err != nil {
		return fmt.Errorf("failed to reconcile network: %w", err)
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
		return err
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
		return err
	}

	log.Printf("=== INFRASTRUCTURE SETUP COMPLETE at %s ===", time.Now().Format("15:04:05"))

	// 6. Control Plane Servers
	cpIPs, cpSANs, err := r.reconcileControlPlane(ctx)
	if err != nil {
		return fmt.Errorf("failed to reconcile control plane: %w", err)
	}

	// 7. Bootstrap
	if len(cpIPs) > 0 {
		clientCfg, err := r.talosGenerator.GetClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get client config: %w", err)
		}

		// Generate machine configs for each control plane node
		// Use the SANs gathered during control plane reconciliation
		cpConfig, err := r.talosGenerator.GenerateControlPlaneConfig(cpSANs)
		if err != nil {
			return fmt.Errorf("failed to generate control plane config: %w", err)
		}

		machineConfigs := make(map[string][]byte)
		for name := range cpIPs {
			machineConfigs[name] = cpConfig
		}

		if err := r.bootstrapper.Bootstrap(ctx, r.config.ClusterName, cpIPs, machineConfigs, clientCfg); err != nil {
			return fmt.Errorf("failed to bootstrap cluster: %w", err)
		}
	}

	// 8. Worker Servers
	if err := r.reconcileWorkers(ctx); err != nil {
		return fmt.Errorf("failed to reconcile workers: %w", err)
	}

	// 9. Addons (CCM, CSI, CNI)
	if err := r.reconcileAddons(ctx, cpIPs); err != nil {
		return fmt.Errorf("failed to reconcile addons: %w", err)
	}

	return nil
}
func (r *Reconciler) reconcileAddons(ctx context.Context, cpIPs map[string]string) error {
	if len(cpIPs) == 0 {
		return nil
	}

	log.Println("Reconciling Kubernetes addons...")

	// 1. Get first CP IP
	var nodeIP string
	for _, ip := range cpIPs {
		nodeIP = ip
		break
	}

	// 2. Retrieve Kubeconfig
	kubeconfig, err := r.talosGenerator.GetKubeconfig(ctx, nodeIP)
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	// 3. Save Kubeconfig locally
	kubeconfigPath := "kubeconfig"
	if err := os.WriteFile(kubeconfigPath, kubeconfig, 0600); err != nil {
		log.Printf("Warning: Failed to write kubeconfig to %s: %v", kubeconfigPath, err)
	} else {
		log.Printf("Kubeconfig saved to %s", kubeconfigPath)
	}

	// 3.5 Save talosconfig for debug
	if talosconfig, err := r.talosGenerator.GetClientConfig(); err == nil {
		if err := os.WriteFile("talosconfig", talosconfig, 0600); err == nil {
			log.Printf("Talosconfig saved to talosconfig for debug")
		}
	}

	// 4. Wait for Kubernetes API to be ready
	log.Println("Waiting for Kubernetes API to be ready...")
	if err := r.waitForKubeAPI(ctx, kubeconfig); err != nil {
		return fmt.Errorf("kubernetes API failed to become ready: %w", err)
	}

	// 5. Initialize Addon Manager if not already set
	if r.addonManager == nil {
		addonMgr, err := addons.NewManager(kubeconfig, r.config, r.network.ID)
		if err != nil {
			return fmt.Errorf("failed to initialize addon manager: %w", err)
		}
		r.addonManager = addonMgr
	}

	// 6. Ensure Addons
	return r.addonManager.EnsureAddons(ctx)
}

func (r *Reconciler) waitForKubeAPI(ctx context.Context, kubeconfig []byte) error {
	kClient, err := k8s.NewClient(kubeconfig)
	if err != nil {
		return err
	}

	timeout := 10 * time.Minute
	deadline := time.Now().Add(timeout)
	backoff := 5 * time.Second

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for Kubernetes API")
		}

		_, err := kClient.Clientset.Discovery().ServerVersion()
		if err == nil {
			log.Println("Kubernetes API is ready!")
			return nil
		}

		log.Printf("Waiting for Kubernetes API... (%v)", err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
}
