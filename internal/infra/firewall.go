package infra

import (
	"context"
	"fmt"
	"net"

	hcloudlib "github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// EnsureFirewall ensures the firewall exists and has the correct rules.
func (m *Manager) EnsureFirewall(ctx context.Context) error {
	firewallClient := m.client.Firewall()
	fwName := m.cfg.ClusterName + "-firewall"

	rules := []hcloudlib.FirewallRule{
		{
			Direction: hcloudlib.FirewallRuleDirectionIn,
			Protocol:  hcloudlib.FirewallRuleProtocolTCP,
			Port:      hcloudlib.Ptr("6443"),
			SourceIPs: []net.IPNet{}, // Filled from config
		},
		{
			Direction: hcloudlib.FirewallRuleDirectionIn,
			Protocol:  hcloudlib.FirewallRuleProtocolTCP,
			Port:      hcloudlib.Ptr("50000"),
			SourceIPs: []net.IPNet{}, // Filled from config
		},
	}

	for _, cidr := range m.cfg.Hetzner.Firewall.APISource {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("invalid API source CIDR %s: %w", cidr, err)
		}
		rules[0].SourceIPs = append(rules[0].SourceIPs, *ipNet)
		rules[1].SourceIPs = append(rules[1].SourceIPs, *ipNet)
	}

	// 1. Check if firewall exists
	firewall, _, err := firewallClient.Get(ctx, fwName)
	if err != nil {
		return fmt.Errorf("failed to get firewall: %w", err)
	}

	if firewall != nil {
		// Firewall exists, update rules using SetRules
		opts := hcloudlib.FirewallSetRulesOpts{
			Rules: rules,
		}
		_, _, err := firewallClient.SetRules(ctx, firewall, opts)
		if err != nil {
			return fmt.Errorf("failed to update firewall rules: %w", err)
		}
		return nil
	}

	// 2. Create firewall
	opts := hcloudlib.FirewallCreateOpts{
		Name:  fwName,
		Rules: rules,
		Labels: map[string]string{
			"cluster": m.cfg.ClusterName,
		},
	}

	_, _, err = firewallClient.Create(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to create firewall: %w", err)
	}

	return nil
}
