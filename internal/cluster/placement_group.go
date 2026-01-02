package cluster

import (
	"context"
	"fmt"
)

func (r *Reconciler) reconcilePlacementGroups(ctx context.Context) error {
	// Control Plane Spread
	name := fmt.Sprintf("%s-control-plane", r.config.ClusterName)
	labels := map[string]string{"cluster": r.config.ClusterName, "role": "control-plane"}
	_, err := r.pgManager.EnsurePlacementGroup(ctx, name, "spread", labels)
	return err
}
