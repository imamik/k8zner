// Package infrastructure provides infrastructure provisioning functionality including
// network, firewall, load balancers, and floating IP management.
package infrastructure

import (
	"log"
	"net"

	"hcloud-k8s/internal/provisioning"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// ProvisionFirewall provisions the cluster firewall with rules.
func (p *Provisioner) ProvisionFirewall(ctx *provisioning.Context, publicIP string) error {
	log.Printf("Reconciling Firewall %s...", ctx.Config.ClusterName)

	// Collect Allow Sources
	// 1. Kube API
	kubeAPISources := []string{}
	// Add config sources
	if len(ctx.Config.Firewall.KubeAPISource) > 0 {
		kubeAPISources = append(kubeAPISources, ctx.Config.Firewall.KubeAPISource...)
	} else if len(ctx.Config.Firewall.APISource) > 0 {
		kubeAPISources = append(kubeAPISources, ctx.Config.Firewall.APISource...)
	}
	// Add current IP if detected and allowed
	if publicIP != "" && ctx.Config.Firewall.UseCurrentIPv4 {
		kubeAPISources = append(kubeAPISources, publicIP+"/32")
	}

	// 2. Talos API
	talosAPISources := []string{}
	if len(ctx.Config.Firewall.TalosAPISource) > 0 {
		talosAPISources = append(talosAPISources, ctx.Config.Firewall.TalosAPISource...)
	} else if len(ctx.Config.Firewall.APISource) > 0 {
		talosAPISources = append(talosAPISources, ctx.Config.Firewall.APISource...)
	}
	if publicIP != "" && ctx.Config.Firewall.UseCurrentIPv4 {
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

	// Extra Rules
	for _, rule := range ctx.Config.Firewall.ExtraRules {
		// Helper to parse IPs
		parseIPs := func(ips []string) []net.IPNet {
			var nets []net.IPNet
			for _, ip := range ips {
				_, n, err := net.ParseCIDR(ip)
				if err == nil {
					nets = append(nets, *n)
				}
			}
			return nets
		}

		direction := hcloud.FirewallRuleDirectionIn
		if rule.Direction == "out" {
			direction = hcloud.FirewallRuleDirectionOut
		}

		protocol := hcloud.FirewallRuleProtocolTCP
		switch rule.Protocol {
		case "udp":
			protocol = hcloud.FirewallRuleProtocolUDP
		case "icmp":
			protocol = hcloud.FirewallRuleProtocolICMP
		case "gre":
			protocol = hcloud.FirewallRuleProtocolGRE
		case "esp":
			protocol = hcloud.FirewallRuleProtocolESP
		}

		r := hcloud.FirewallRule{
			Description:    hcloud.Ptr(rule.Description),
			Direction:      direction,
			Protocol:       protocol,
			SourceIPs:      parseIPs(rule.SourceIPs),
			DestinationIPs: parseIPs(rule.DestinationIPs),
		}
		if rule.Port != "" {
			r.Port = hcloud.Ptr(rule.Port)
		}
		rules = append(rules, r)
	}

	labels := map[string]string{
		"cluster": ctx.Config.ClusterName,
	}

	fw, err := ctx.Infra.EnsureFirewall(ctx, ctx.Config.ClusterName, rules, labels)
	if err != nil {
		return err
	}
	ctx.State.Firewall = fw
	return nil
}
