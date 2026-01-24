package infrastructure

import (
	"fmt"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"

	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/naming"
)

// ProvisionFloatingIPs provisions floating IPs for the control plane.
func (p *Provisioner) ProvisionFloatingIPs(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Reconciling floating IPs for %s...", phase, ctx.Config.ClusterName)
	if ctx.Config.ControlPlane.PublicVIPIPv4Enabled {
		name := naming.ControlPlaneFloatingIP(ctx.Config.ClusterName)
		labels := map[string]string{"cluster": ctx.Config.ClusterName, "role": "control-plane"}
		_, err := ctx.Infra.EnsureFloatingIP(ctx, name, ctx.Config.Location, string(hcloud.FloatingIPTypeIPv4), labels)
		if err != nil {
			return fmt.Errorf("failed to ensure floating IP %s: %w", name, err)
		}
	}
	return nil
}
