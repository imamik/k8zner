// Package destroy handles cluster teardown and resource cleanup.
package destroy

import (
	"fmt"

	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/labels"
)

// Provisioner handles cluster destruction.
type Provisioner struct{}

// NewProvisioner creates a new destroy provisioner.
func NewProvisioner() *Provisioner {
	return &Provisioner{}
}

// Name returns the phase name.
func (p *Provisioner) Name() string {
	return "Destroy"
}

// Provision destroys the cluster and all associated resources.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[Destroy] Starting cluster destruction for: %s", ctx.Config.ClusterName)

	// Build label selector for cluster resources
	clusterLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
		WithTestIDIfSet(ctx.Config.TestID).
		Build()

	provisioning.LogResourceDeleting(ctx.Observer, "destroy", "cluster", ctx.Config.ClusterName)

	// Delete all cluster resources by label
	// This includes: servers, load balancers, floating IPs, firewalls, networks,
	// placement groups, SSH keys, and certificates
	if err := ctx.Infra.CleanupByLabel(ctx, clusterLabels); err != nil {
		return fmt.Errorf("failed to cleanup cluster resources: %w", err)
	}

	provisioning.LogResourceDeleted(ctx.Observer, "destroy", "cluster", ctx.Config.ClusterName)
	ctx.Observer.Printf("[Destroy] Cluster %s destroyed successfully", ctx.Config.ClusterName)

	return nil
}
