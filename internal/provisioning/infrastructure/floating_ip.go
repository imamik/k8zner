package infrastructure

import (
	"fmt"
	"log"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"

	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/naming"
)

// ProvisionFloatingIPs provisions floating IPs for the control plane.
func (p *Provisioner) ProvisionFloatingIPs(ctx *provisioning.Context) error {
	log.Printf("[Infra:FIP] Reconciling Floating IPs for %s...", ctx.Config.ClusterName)
	if ctx.Config.ControlPlane.PublicVIPIPv4Enabled {
		name := naming.ControlPlaneFloatingIP(ctx.Config.ClusterName)
		labels := map[string]string{"cluster": ctx.Config.ClusterName, "role": "control-plane"}
		_, err := ctx.Infra.EnsureFloatingIP(ctx, name, ctx.Config.Network.Zone, string(hcloud.FloatingIPTypeIPv4), labels)
		if err != nil {
			return fmt.Errorf("failed to ensure floating IP %s: %w", name, err)
		}
	}
	return nil
}
