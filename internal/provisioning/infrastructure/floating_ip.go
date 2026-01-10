package infrastructure

import (
	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/naming"
)

// ProvisionFloatingIPs creates floating IPs for the control plane if enabled in the configuration.
func (p *Provisioner) ProvisionFloatingIPs(ctx *provisioning.Context) error {
	if ctx.Config.ControlPlane.PublicVIPIPv4Enabled {
		name := naming.ControlPlaneFloatingIP(ctx.Config.ClusterName)
		labels := map[string]string{"cluster": ctx.Config.ClusterName, "role": "control-plane"}
		_, err := ctx.Infra.EnsureFloatingIP(ctx, name, ctx.Config.Location, "ipv4", labels)
		if err != nil {
			return err
		}
	}
	return nil
}
