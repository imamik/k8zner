// Package orchestration provides high-level workflow coordination for cluster provisioning.
//
// This package orchestrates the provisioning workflow by delegating to specialized
// provisioners. It defines the order and coordination but delegates the actual work.
package orchestration

import (
	"context"
	"fmt"
	"log"

	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning/cluster"
	"hcloud-k8s/internal/provisioning/compute"
	"hcloud-k8s/internal/provisioning/image"
	"hcloud-k8s/internal/provisioning/infrastructure"
	"hcloud-k8s/internal/util/async"
)

// TalosConfigProducer defines the interface for generating Talos configurations.
type TalosConfigProducer interface {
	GenerateControlPlaneConfig(san []string, hostname string) ([]byte, error)
	GenerateWorkerConfig(hostname string) ([]byte, error)
	GetClientConfig() ([]byte, error)
	SetEndpoint(endpoint string)
}

// Reconciler orchestrates the cluster provisioning workflow.
// It delegates actual provisioning to specialized provisioners.
type Reconciler struct {
	infra          hcloud_internal.InfrastructureManager
	talosGenerator TalosConfigProducer
	config         *config.Config

	// Provisioners
	infraProvisioner   *infrastructure.Provisioner
	computeProvisioner *compute.Provisioner
	imageProvisioner   *image.Provisioner
	clusterProvisioner *cluster.Bootstrapper
}

// NewReconciler creates a new orchestration reconciler.
func NewReconciler(
	infra hcloud_internal.InfrastructureManager,
	talosGenerator TalosConfigProducer,
	cfg *config.Config,
) *Reconciler {
	return &Reconciler{
		infra:              infra,
		talosGenerator:     talosGenerator,
		config:             cfg,
		infraProvisioner:   infrastructure.NewProvisioner(infra, cfg),
		imageProvisioner:   image.NewProvisioner(infra, cfg),
		clusterProvisioner: cluster.NewBootstrapper(infra),
		// Note: computeProvisioner will be created after network provisioning
	}
}

// Reconcile ensures that the desired state matches the actual state.
// Returns the kubeconfig bytes if bootstrap was performed, or nil if cluster already existed.
func (r *Reconciler) Reconcile(ctx context.Context) ([]byte, error) {
	log.Println("Starting reconciliation...")

	// 0. Calculate Subnets
	if err := r.config.CalculateSubnets(); err != nil {
		return nil, fmt.Errorf("failed to calculate subnets: %w", err)
	}

	// 1. Provision Network (must be first)
	if err := r.infraProvisioner.ProvisionNetwork(ctx); err != nil {
		return nil, fmt.Errorf("failed to provision network: %w", err)
	}

	// Now create compute provisioner with the provisioned network
	r.computeProvisioner = compute.NewProvisioner(
		r.infra,
		r.talosGenerator,
		r.config,
		r.infraProvisioner.GetNetwork(),
	)

	// 2. Pre-build images and fetch public IP in parallel
	var publicIP string
	imageTasks := []async.Task{
		{
			Name: "images",
			Func: r.imageProvisioner.EnsureAllImages,
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
				return nil
			},
		},
	}

	if err := async.RunParallel(ctx, imageTasks, false); err != nil {
		return nil, err
	}

	// 3. Provision infrastructure in parallel
	infraTasks := []async.Task{
		{
			Name: "firewall",
			Func: func(ctx context.Context) error {
				return r.infraProvisioner.ProvisionFirewall(ctx, publicIP)
			},
		},
		{Name: "loadBalancers", Func: r.infraProvisioner.ProvisionLoadBalancers},
		{Name: "floatingIPs", Func: r.infraProvisioner.ProvisionFloatingIPs},
	}

	if err := async.RunParallel(ctx, infraTasks, false); err != nil {
		return nil, err
	}

	// 4. Provision Control Plane
	cpIPs, sans, err := r.computeProvisioner.ProvisionControlPlane(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to provision control plane: %w", err)
	}

	// 5. Bootstrap cluster
	var kubeconfig []byte
	var clientCfg []byte
	if len(cpIPs) > 0 {
		clientCfg, err = r.talosGenerator.GetClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get client config: %w", err)
		}

		// Generate per-node machine configs
		machineConfigs := make(map[string][]byte)
		for nodeName := range cpIPs {
			nodeConfig, err := r.talosGenerator.GenerateControlPlaneConfig(sans, nodeName)
			if err != nil {
				return nil, fmt.Errorf("failed to generate control plane config for %s: %w", nodeName, err)
			}
			machineConfigs[nodeName] = nodeConfig
		}

		kubeconfig, err = r.clusterProvisioner.Bootstrap(ctx, r.config.ClusterName, cpIPs, machineConfigs, clientCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to bootstrap cluster: %w", err)
		}
	}

	// 6. Provision Workers
	workerIPs, err := r.computeProvisioner.ProvisionWorkers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to provision workers: %w", err)
	}

	// 7. Apply worker configurations
	if len(workerIPs) > 0 && len(kubeconfig) > 0 {
		log.Printf("Applying Talos configurations to %d worker nodes...", len(workerIPs))

		workerConfigs := make(map[string][]byte)
		for nodeName := range workerIPs {
			nodeConfig, err := r.talosGenerator.GenerateWorkerConfig(nodeName)
			if err != nil {
				return nil, fmt.Errorf("failed to generate worker config for %s: %w", nodeName, err)
			}
			workerConfigs[nodeName] = nodeConfig
		}

		if err := r.clusterProvisioner.ApplyWorkerConfigs(ctx, workerIPs, workerConfigs, clientCfg); err != nil {
			return nil, fmt.Errorf("failed to apply worker configs: %w", err)
		}
	}

	return kubeconfig, nil
}

// GetNetworkID returns the ID of the provisioned network.
func (r *Reconciler) GetNetworkID() int64 {
	network := r.infraProvisioner.GetNetwork()
	if network == nil {
		return 0
	}
	return network.ID
}
