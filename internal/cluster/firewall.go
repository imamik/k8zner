package cluster

import (
	"context"
	"log"
	"net"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func (r *Reconciler) reconcileFirewall(ctx context.Context, publicIP string) error {
	log.Printf("Reconciling Firewall %s...", r.config.ClusterName)

	// Collect Allow Sources
	// 1. Kube API
	kubeAPISources := []string{}
	// Add config sources
	if len(r.config.Firewall.KubeAPISource) > 0 {
		kubeAPISources = append(kubeAPISources, r.config.Firewall.KubeAPISource...)
	} else if len(r.config.Firewall.APISource) > 0 {
		kubeAPISources = append(kubeAPISources, r.config.Firewall.APISource...)
	}
	// Add current IP if detected and allowed
	// Logic matches terraform: !firewall_external && network_public_ipv4_enabled && coalesce(var.firewall_use_current_ipv4, cluster_access == "public" && source == null)
	// Simplified: if we have public IP and we assume public access/defaults, add it.
	if publicIP != "" && r.config.Firewall.UseCurrentIPv4 {
		kubeAPISources = append(kubeAPISources, publicIP+"/32")
	}

	// 2. Talos API
	talosAPISources := []string{}
	if len(r.config.Firewall.TalosAPISource) > 0 {
		talosAPISources = append(talosAPISources, r.config.Firewall.TalosAPISource...)
	} else if len(r.config.Firewall.APISource) > 0 {
		talosAPISources = append(talosAPISources, r.config.Firewall.APISource...)
	}
	if publicIP != "" && r.config.Firewall.UseCurrentIPv4 {
		talosAPISources = append(talosAPISources, publicIP+"/32")
	}

	// Build Rules
	rules := []hcloud.FirewallRule{}

	// Kube API Rule (TCP 6443)
	if len(kubeAPISources) > 0 {
		sourceNets := make([]net.IPNet, 0)
		for _, s := range kubeAPISources {
			_, n, err := net.ParseCIDR(s)
			if err == nil {
				sourceNets = append(sourceNets, *n)
			}
		}
		if len(sourceNets) > 0 {
			rules = append(rules, hcloud.FirewallRule{
				Description: hcloud.Ptr("Allow Incoming Requests to Kube API"),
				Direction:   hcloud.FirewallRuleDirectionIn,
				Protocol:    hcloud.FirewallRuleProtocolTCP,
				Port:        hcloud.Ptr("6443"),
				SourceIPs:   sourceNets,
			})
		}
	}

	// Talos API Rule (TCP 50000)
	if len(talosAPISources) > 0 {
		sourceNets := make([]net.IPNet, 0)
		for _, s := range talosAPISources {
			_, n, err := net.ParseCIDR(s)
			if err == nil {
				sourceNets = append(sourceNets, *n)
			}
		}
		if len(sourceNets) > 0 {
			rules = append(rules, hcloud.FirewallRule{
				Description: hcloud.Ptr("Allow Incoming Requests to Talos API"),
				Direction:   hcloud.FirewallRuleDirectionIn,
				Protocol:    hcloud.FirewallRuleProtocolTCP,
				Port:        hcloud.Ptr("50000"),
				SourceIPs:   sourceNets,
			})
		}
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
