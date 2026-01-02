package cluster

import (
	"context"
	"log"
	"net"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func (r *Reconciler) reconcileFirewall(ctx context.Context) error {
	log.Printf("Reconciling Firewall %s...", r.config.ClusterName)

	// Base Rules
	rules := []hcloud.FirewallRule{
		{
			Direction: hcloud.FirewallRuleDirectionIn,
			Protocol:  hcloud.FirewallRuleProtocolICMP,
			SourceIPs: []net.IPNet{{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, {IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}},
		},
		{
			Direction: hcloud.FirewallRuleDirectionIn,
			Protocol:  hcloud.FirewallRuleProtocolTCP,
			Port:      ptr("22"),
			SourceIPs: []net.IPNet{{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, {IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}},
		},
		// API
		{
			Direction: hcloud.FirewallRuleDirectionIn,
			Protocol:  hcloud.FirewallRuleProtocolTCP,
			Port:      ptr("6443"),
			SourceIPs: []net.IPNet{{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, {IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}},
		},
		{
			Direction: hcloud.FirewallRuleDirectionIn,
			Protocol:  hcloud.FirewallRuleProtocolTCP,
			Port:      ptr("50000"), // Talos API
			SourceIPs: []net.IPNet{{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, {IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}},
		},
	}

	labels := map[string]string{
		"cluster": r.config.ClusterName,
	}

	fw, err := r.firewallManager.EnsureFirewall(ctx, r.config.ClusterName, rules, labels)
	if err != nil {
		return err
	}
	r.firewall = fw
	return nil
}
