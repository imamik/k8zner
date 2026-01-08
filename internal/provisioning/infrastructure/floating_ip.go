package infrastructure

import (
	"context"

	"hcloud-k8s/internal/util/naming"
)

func (p *Provisioner) ProvisionFloatingIPs(ctx context.Context) error {
	if p.config.ControlPlane.PublicVIPIPv4Enabled {
		name := naming.ControlPlaneFloatingIP(p.config.ClusterName)
		labels := map[string]string{"cluster": p.config.ClusterName, "role": "control-plane"}
		_, err := p.fipManager.EnsureFloatingIP(ctx, name, p.config.Location, "ipv4", labels)
		if err != nil {
			return err
		}
	}
	return nil
}
