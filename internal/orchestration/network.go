package orchestration

import (
	"context"
	"fmt"

	"hcloud-k8s/internal/provisioning/compute"
)

// provisionPrerequisites handles initial setup and network provisioning.
func (r *Reconciler) provisionPrerequisites(ctx context.Context) error {
	// Calculate subnets
	if err := r.config.CalculateSubnets(); err != nil {
		return fmt.Errorf("failed to calculate subnets: %w", err)
	}

	// Provision network (must be first)
	if err := r.infraProvisioner.ProvisionNetwork(ctx); err != nil {
		return fmt.Errorf("failed to provision network: %w", err)
	}

	// Create compute provisioner with the provisioned network
	r.computeProvisioner = compute.NewProvisioner(
		r.infra,
		r.talosGenerator,
		r.config,
		r.infraProvisioner.GetNetwork(),
	)

	return nil
}
