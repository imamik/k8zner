// Package infrastructure provides infrastructure provisioning functionality including
// network, firewall, and load balancer management.
package infrastructure

import (
	"fmt"
	"net"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/labels"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

const phase = "infrastructure"

// ProvisionFirewall provisions the cluster firewall with rules.
func (p *Provisioner) ProvisionFirewall(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[%s] Reconciling firewall %s...", phase, ctx.Config.ClusterName)

	publicIP := ctx.State.PublicIP
	fw := &ctx.Config.Firewall

	// Collect API sources using helpers
	kubeAPISources := collectAPISources(fw.KubeAPISource, fw.APISource, publicIP, fw.UseCurrentIPv4)
	talosAPISources := collectAPISources(fw.TalosAPISource, fw.APISource, publicIP, fw.UseCurrentIPv4)

	// Build firewall rules
	rules := []hcloud.FirewallRule{}

	// Kube API Rule (TCP 6443)
	if sourceNets := parseCIDRs(kubeAPISources); len(sourceNets) > 0 {
		rules = append(rules, hcloud.FirewallRule{
			Description: hcloud.Ptr("Allow Incoming Requests to Kube API"),
			Direction:   hcloud.FirewallRuleDirectionIn,
			Protocol:    hcloud.FirewallRuleProtocolTCP,
			Port:        hcloud.Ptr("6443"),
			SourceIPs:   sourceNets,
		})
	}

	// Talos API Rule (TCP 50000)
	if sourceNets := parseCIDRs(talosAPISources); len(sourceNets) > 0 {
		rules = append(rules, hcloud.FirewallRule{
			Description: hcloud.Ptr("Allow Incoming Requests to Talos API"),
			Direction:   hcloud.FirewallRuleDirectionIn,
			Protocol:    hcloud.FirewallRuleProtocolTCP,
			Port:        hcloud.Ptr("50000"),
			SourceIPs:   sourceNets,
		})
	}

	// Extra Rules from config
	for _, rule := range fw.ExtraRules {
		rules = append(rules, buildFirewallRule(rule))
	}

	firewallLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
		WithTestIDIfSet(ctx.Config.TestID).
		Build()

	// Apply firewall to all servers in this cluster using label selector
	applyToLabelSelector := fmt.Sprintf("cluster=%s", ctx.Config.ClusterName)

	result, err := ctx.Infra.EnsureFirewall(ctx, ctx.Config.ClusterName, rules, firewallLabels, applyToLabelSelector)
	if err != nil {
		return fmt.Errorf("failed to ensure firewall: %w", err)
	}
	ctx.State.Firewall = result
	ctx.Observer.Printf("[%s] Firewall %s applied to servers with label selector: %s", phase, ctx.Config.ClusterName, applyToLabelSelector)
	return nil
}

// collectAPISources collects IP sources with fallback and current IP logic.
func collectAPISources(specific, fallback []string, publicIP string, useCurrentIP *bool) []string {
	sources := []string{}
	if len(specific) > 0 {
		sources = append(sources, specific...)
	} else if len(fallback) > 0 {
		sources = append(sources, fallback...)
	}
	if publicIP != "" && useCurrentIP != nil && *useCurrentIP {
		sources = append(sources, publicIP+"/32")
	}
	return sources
}

// parseCIDRs parses a slice of CIDR strings into net.IPNet, skipping invalid entries.
func parseCIDRs(cidrs []string) []net.IPNet {
	var nets []net.IPNet
	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, *n)
		}
	}
	return nets
}

// buildFirewallRule converts a config FirewallRule to an hcloud FirewallRule.
func buildFirewallRule(rule config.FirewallRule) hcloud.FirewallRule {
	direction := hcloud.FirewallRuleDirectionIn
	if rule.Direction == "out" {
		direction = hcloud.FirewallRuleDirectionOut
	}

	r := hcloud.FirewallRule{
		Description:    hcloud.Ptr(rule.Description),
		Direction:      direction,
		Protocol:       parseProtocol(rule.Protocol),
		SourceIPs:      parseCIDRs(rule.SourceIPs),
		DestinationIPs: parseCIDRs(rule.DestinationIPs),
	}
	if rule.Port != "" {
		r.Port = hcloud.Ptr(rule.Port)
	}
	return r
}

// parseProtocol converts a protocol string to hcloud FirewallRuleProtocol.
func parseProtocol(proto string) hcloud.FirewallRuleProtocol {
	switch proto {
	case "udp":
		return hcloud.FirewallRuleProtocolUDP
	case "icmp":
		return hcloud.FirewallRuleProtocolICMP
	case "gre":
		return hcloud.FirewallRuleProtocolGRE
	case "esp":
		return hcloud.FirewallRuleProtocolESP
	default:
		return hcloud.FirewallRuleProtocolTCP
	}
}
