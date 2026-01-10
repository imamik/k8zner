// Package orchestration provides high-level workflow coordination for cluster provisioning.
//
// This package orchestrates the provisioning workflow by delegating to specialized
// provisioners. It defines the order and coordination but delegates the actual work.
package orchestration

import (
	"context"
	"fmt"
	"log"

	"hcloud-k8s/internal/addons"
	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/provisioning/cluster"
	"hcloud-k8s/internal/provisioning/compute"
	"hcloud-k8s/internal/provisioning/image"
	"hcloud-k8s/internal/provisioning/infrastructure"
)

// Reconciler orchestrates the cluster provisioning workflow.
type Reconciler struct {
	infra          hcloud_internal.InfrastructureManager
	talosGenerator provisioning.TalosConfigProducer
	config         *config.Config
	state          *provisioning.State

	// Phases
	infraProvisioner   *infrastructure.Provisioner
	imageProvisioner   *image.Provisioner
	computeProvisioner *compute.Provisioner
	clusterProvisioner *cluster.Provisioner
}

// NewReconciler creates a new orchestration reconciler.
func NewReconciler(
	infra hcloud_internal.InfrastructureManager,
	talosGenerator provisioning.TalosConfigProducer,
	cfg *config.Config,
) *Reconciler {
	return &Reconciler{
		infra:              infra,
		talosGenerator:     talosGenerator,
		config:             cfg,
		state:              provisioning.NewState(),
		infraProvisioner:   infrastructure.NewProvisioner(),
		imageProvisioner:   image.NewProvisioner(),
		computeProvisioner: compute.NewProvisioner(),
		clusterProvisioner: cluster.NewProvisioner(),
	}
}

// Reconcile ensures that the desired state matches the actual state.
// Returns the kubeconfig bytes if bootstrap was performed, or nil if cluster already existed.
func (r *Reconciler) Reconcile(ctx context.Context) ([]byte, error) {
	// 1. Setup Provisioning Context
	pCtx := &provisioning.Context{
		Context: ctx,
		Config:  r.config,
		State:   r.state,
		Infra:   r.infra,
		Talos:   r.talosGenerator,
	}

	// 2. Sequential Execution of Provisioning Phases
	phases := []struct {
		name  string
		phase provisioning.Phase
	}{
		{"infrastructure", r.infraProvisioner},
		{"image", r.imageProvisioner},
		{"compute", r.computeProvisioner},
		{"cluster", r.clusterProvisioner},
	}

	for _, p := range phases {
		log.Printf("Starting phase: %s", p.name)
		if err := p.phase.Provision(pCtx); err != nil {
			return nil, fmt.Errorf("phase %s failed: %w", p.name, err)
		}
	}

	// 3. Install addons (if cluster was bootstrapped)
	if len(r.state.Kubeconfig) > 0 {
		log.Println("Installing cluster addons...")
		networkID := r.state.Network.ID
		if err := addons.Apply(ctx, r.config, r.state.Kubeconfig, networkID); err != nil {
			return nil, fmt.Errorf("failed to install addons: %w", err)
		}
	}

	return r.state.Kubeconfig, nil
}
