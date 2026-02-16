package infrastructure

import (
	"github.com/imamik/k8zner/internal/provisioning"
)

// Provision creates network, firewall, and load balancer resources.
func Provision(ctx *provisioning.Context) error {
	// 1. Network
	if err := ProvisionNetwork(ctx); err != nil {
		return err
	}

	// 2. Firewall
	if err := ProvisionFirewall(ctx); err != nil {
		return err
	}

	// 3. Load Balancers
	if err := ProvisionLoadBalancers(ctx); err != nil {
		return err
	}

	return nil
}
