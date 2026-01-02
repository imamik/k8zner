package cluster

import (
	"context"
	"fmt"
)

func (r *Reconciler) reconcileFloatingIPs(ctx context.Context) error {
	if r.config.ControlPlane.PublicVIPIPv4Enabled {
		name := fmt.Sprintf("%s-control-plane-ipv4", r.config.ClusterName)
		labels := map[string]string{"cluster": r.config.ClusterName, "role": "control-plane"}
		_, err := r.fipManager.EnsureFloatingIP(ctx, name, r.config.Location, "ipv4", labels)
		if err != nil {
			return err
		}
	}
	return nil
}
