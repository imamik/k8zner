// Package orchestration provides high-level reconciliation orchestration for cluster provisioning.
//
// This package coordinates the workflow of provisioning infrastructure and managing
// cluster state by delegating to lifecycle and platform packages.
package orchestration

import (
	"context"
	"fmt"
	"log"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/hcloud"
	"hcloud-k8s/internal/lifecycle"
)

// TalosConfigProducer defines the interface for generating Talos configurations.
type TalosConfigProducer interface {
	GenerateControlPlaneConfig(san []string, hostname string) ([]byte, error)
	GenerateWorkerConfig(hostname string) ([]byte, error)
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
	bootstrapper      *lifecycle.Bootstrapper
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
		bootstrapper:      lifecycle.NewBootstrapper(infra),
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

	if err := RunParallel(ctx, infraTasks, false); err != nil {
		return nil, err
	}

	// 6. Control Plane Servers
	cpIPs, sans, err := r.reconcileControlPlane(ctx)
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

		// Generate per-node machine configs with hostnames
		machineConfigs := make(map[string][]byte)
		for nodeName := range cpIPs {
			// Generate config with this node's hostname
			nodeConfig, err := r.talosGenerator.GenerateControlPlaneConfig(sans, nodeName)
			if err != nil {
				return nil, fmt.Errorf("failed to generate control plane config for %s: %w", nodeName, err)
			}
			machineConfigs[nodeName] = nodeConfig
		}

		kubeconfig, err = r.bootstrapper.Bootstrap(ctx, r.config.ClusterName, cpIPs, machineConfigs, clientCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to bootstrap cluster: %w", err)
		}
	}

	// 8. Worker Servers
	workerIPs, err := r.reconcileWorkers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile workers: %w", err)
	}

	// 8a. Apply worker configurations (if workers exist and cluster is bootstrapped)
	if len(workerIPs) > 0 && len(kubeconfig) > 0 {
		log.Printf("Applying Talos configurations to %d worker nodes...", len(workerIPs))

		// Generate per-node worker configs with hostnames
		workerConfigs := make(map[string][]byte)
		for nodeName := range workerIPs {
			nodeConfig, err := r.talosGenerator.GenerateWorkerConfig(nodeName)
			if err != nil {
				return nil, fmt.Errorf("failed to generate worker config for %s: %w", nodeName, err)
			}
			workerConfigs[nodeName] = nodeConfig
		}

		if err := r.bootstrapper.ApplyWorkerConfigs(ctx, workerIPs, workerConfigs, clientCfg); err != nil {
			return nil, fmt.Errorf("failed to apply worker configs: %w", err)
		}
	}

	return kubeconfig, nil
}

// GetNetworkID returns the ID of the reconciled network.
// Returns 0 if network has not been reconciled yet.
func (r *Reconciler) GetNetworkID() int64 {
	if r.network == nil {
		return 0
	}
	return r.network.ID
}
