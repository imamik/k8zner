// Package orchestration provides high-level workflow coordination for cluster provisioning.
//
// This package orchestrates the provisioning workflow by delegating to specialized
// provisioners. It defines the order and coordination but delegates the actual work.
package orchestration

import (
	"context"
	"fmt"

	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/platform/talos"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/provisioning/cluster"
	"github.com/imamik/k8zner/internal/provisioning/compute"
	"github.com/imamik/k8zner/internal/provisioning/image"
	"github.com/imamik/k8zner/internal/provisioning/infrastructure"
)

const phase = "orchestrator"

// Reconciler orchestrates the cluster provisioning workflow.
type Reconciler struct {
	infra          hcloud_internal.InfrastructureManager
	talosGenerator provisioning.TalosConfigProducer
	config         *config.Config
	state          *provisioning.State
	timeouts       *config.Timeouts // Optional custom timeouts (for testing)

	// Phases
	infraProvisioner   *infrastructure.Provisioner
	imageProvisioner   *image.Provisioner
	computeProvisioner *compute.Provisioner
	clusterProvisioner *cluster.Provisioner
}

// SetTimeouts sets custom timeouts for the reconciler.
// This is primarily used for testing to avoid long waits.
func (r *Reconciler) SetTimeouts(t *config.Timeouts) {
	r.timeouts = t
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
	// 1. Configure Talos generator with machine config options from config
	machineOpts := talos.NewMachineConfigOptions(r.config)
	r.talosGenerator.SetMachineConfigOptions(machineOpts)

	// 2. Setup Provisioning Context
	pCtx := provisioning.NewContext(ctx, r.config, r.infra, r.talosGenerator)

	// Override timeouts if custom ones are set (e.g., for testing)
	if r.timeouts != nil {
		pCtx.Timeouts = r.timeouts
	}

	// 3. Execute Provisioning Pipeline
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

	// 4. Install addons (if cluster was bootstrapped)
	if len(r.state.Kubeconfig) > 0 {
		pCtx.Logger.Printf("[%s] Installing cluster addons...", phase)
		if err := addons.Apply(ctx, r.config, r.state.Kubeconfig, r.state.Network.ID); err != nil {
			return nil, fmt.Errorf("failed to install addons: %w", err)
		}
	}

	return r.state.Kubeconfig, nil
}
