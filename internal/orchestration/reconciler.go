// Package orchestration provides high-level workflow coordination for cluster provisioning.
//
// This package orchestrates the provisioning workflow by delegating to specialized
// provisioners. It defines the order and coordination but delegates the actual work.
package orchestration

import (
	"context"

	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning/cluster"
	"hcloud-k8s/internal/provisioning/compute"
	"hcloud-k8s/internal/provisioning/image"
	"hcloud-k8s/internal/provisioning/infrastructure"
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
	infraProvisioner *infrastructure.Provisioner
	imageProvisioner *image.Provisioner
	// computeProvisioner is created after network provisioning in provisionPrerequisites()
	// since it requires the provisioned network reference.
	computeProvisioner *compute.Provisioner
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
	// 1. Prerequisites (network, compute provisioner setup)
	if err := r.provisionPrerequisites(ctx); err != nil {
		return nil, err
	}

	// 2. Resources (images, firewall, load balancers, floating IPs)
	if err := r.provisionResources(ctx); err != nil {
		return nil, err
	}

	// 3. Control plane nodes
	cpIPs, sans, err := r.provisionControlPlane(ctx)
	if err != nil {
		return nil, err
	}

	// 4. Bootstrap cluster (if needed)
	kubeconfig, clientCfg, err := r.bootstrapCluster(ctx, cpIPs, sans)
	if err != nil {
		return nil, err
	}

	// 5. Worker nodes
	workerIPs, err := r.provisionWorkers(ctx)
	if err != nil {
		return nil, err
	}

	// 6. Apply worker configs (if needed)
	if err := r.applyWorkerConfigs(ctx, workerIPs, kubeconfig, clientCfg); err != nil {
		return nil, err
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
