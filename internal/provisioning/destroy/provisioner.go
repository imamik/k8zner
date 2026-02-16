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

// Provision destroys the cluster and all associated resources.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[Destroy] Starting cluster destruction for: %s", ctx.Config.ClusterName)

	// Build label selector for cluster resources
	// IMPORTANT: Use only cluster labels, NOT managed-by, because:
	// - CLI-created resources have k8zner.io/managed-by: k8zner
	// - Operator-created resources have k8zner.io/managed-by: k8zner-operator
	// Both should be cleaned up when destroying the cluster.
	clusterLabels := map[string]string{
		labels.KeyCluster:       ctx.Config.ClusterName,
		labels.LegacyKeyCluster: ctx.Config.ClusterName,
	}
	if ctx.Config.TestID != "" {
		clusterLabels[labels.LegacyKeyTestID] = ctx.Config.TestID
	}

	ctx.Observer.Printf("[Destroy] Deleting cluster resources for %s...", ctx.Config.ClusterName)

	// Delete all cluster resources by label
	// This includes: servers, load balancers, firewalls, networks,
	// placement groups, SSH keys, and certificates
	if err := ctx.Infra.CleanupByLabel(ctx, clusterLabels); err != nil {
		return fmt.Errorf("failed to cleanup cluster resources: %w", err)
	}

	ctx.Observer.Printf("[Destroy] Cluster %s destroyed successfully", ctx.Config.ClusterName)

	return nil
}
