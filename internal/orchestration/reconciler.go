// Package orchestration provides high-level workflow coordination for cluster provisioning.
//
// This package orchestrates the provisioning workflow by delegating to specialized
// provisioners. It defines the order and coordination but delegates the actual work.
package orchestration

import (
	"context"
	"fmt"

	"hcloud-k8s/internal/addons"
	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/provisioning/cluster"
	"hcloud-k8s/internal/provisioning/compute"
	"hcloud-k8s/internal/provisioning/image"
	"hcloud-k8s/internal/provisioning/infrastructure"
)

const phase = "orchestrator"

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
	pCtx := provisioning.NewContext(ctx, r.config, r.infra, r.talosGenerator)

	// 2. Execute Provisioning Pipeline
	pipeline := provisioning.NewPipeline(
		provisioning.NewValidationPhase(), // Validation first
		r.infraProvisioner,
		r.imageProvisioner,
		r.computeProvisioner,
		r.clusterProvisioner,
	)

	if err := pipeline.Run(pCtx); err != nil {
		return nil, err
	}

	// Persist state back to reconciler (if needed for legacy reasons, though pCtx.State is what matters)
	r.state = pCtx.State

	// 3. Install addons (if cluster was bootstrapped)
	if len(r.state.Kubeconfig) > 0 {
		pCtx.Logger.Printf("[%s] Installing cluster addons...", phase)
		networkID := r.state.Network.ID
		if err := addons.Apply(ctx, r.config, r.state.Kubeconfig, networkID); err != nil {
			return nil, fmt.Errorf("failed to install addons: %w", err)
		}
	}

	return r.state.Kubeconfig, nil
}
