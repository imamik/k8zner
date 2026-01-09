package orchestration

import (
	"context"
	"fmt"
)

// provisionNetwork calculates subnets and provisions the cluster network.
func (r *Reconciler) provisionNetwork(ctx context.Context) error {
	// Calculate subnets
	if err := r.config.CalculateSubnets(); err != nil {
		return fmt.Errorf("failed to calculate subnets: %w", err)
	}

	// Provision network
	if err := r.infraProvisioner.ProvisionNetwork(ctx); err != nil {
		return fmt.Errorf("failed to provision network: %w", err)
	}

	return nil
}

// GetNetworkID returns the ID of the provisioned network.
func (r *Reconciler) GetNetworkID() int64 {
	network := r.infraProvisioner.GetNetwork()
	if network == nil {
		return 0
	}
	return network.ID
}
