package compute

import (
	"fmt"

	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/rdns"
)

// applyServerRDNSSimple configures reverse DNS for a server using pre-resolved templates.
func (p *Provisioner) applyServerRDNSSimple(ctx *provisioning.Context, serverID int64, serverName, ipv4, rdnsIPv4, rdnsIPv6, role, pool string) error {
	// Apply IPv4 RDNS if configured
	if rdnsIPv4 != "" && ipv4 != "" {
		dnsPtr, err := rdns.RenderTemplate(rdnsIPv4, rdns.TemplateVars{
			ClusterName: ctx.Config.ClusterName,
			Hostname:    serverName,
			ID:          serverID,
			IPAddress:   ipv4,
			IPType:      "ipv4",
			Pool:        pool,
			Role:        role,
		})
		if err != nil {
			return fmt.Errorf("failed to render IPv4 RDNS template: %w", err)
		}

		if err := ctx.Infra.SetServerRDNS(ctx, serverID, ipv4, dnsPtr); err != nil {
			return fmt.Errorf("failed to set IPv4 RDNS: %w", err)
		}

		ctx.Logger.Printf("[%s] Set IPv4 RDNS: %s → %s", phase, ipv4, dnsPtr)
	}

	// Apply IPv6 RDNS if configured (IPv6 support can be added later)
	if rdnsIPv6 != "" {
		// IPv6 address retrieval not yet implemented
		ctx.Logger.Printf("[%s] IPv6 RDNS configured but IPv6 address retrieval not yet implemented", phase)
	}

	return nil
}

// applyLoadBalancerRDNS configures reverse DNS for a load balancer's IP addresses.
func (p *Provisioner) applyLoadBalancerRDNS(ctx *provisioning.Context, lbID int64, lbName, ipv4, ipv6, role string) error {
	// Determine RDNS templates
	var rdnsIPv4, rdnsIPv6 string

	if role == "kube-api" {
		// Kube API load balancer uses cluster-wide defaults
		rdnsIPv4 = resolveRDNSTemplate(ctx.Config.RDNS.ClusterRDNSIPv4, ctx.Config.RDNS.ClusterRDNS, "")
		rdnsIPv6 = resolveRDNSTemplate(ctx.Config.RDNS.ClusterRDNSIPv6, ctx.Config.RDNS.ClusterRDNS, "")
	} else if role == "ingress" {
		// Ingress load balancer uses ingress-specific config
		rdnsIPv4 = resolveRDNSTemplate(ctx.Config.Ingress.RDNSIPv4, ctx.Config.RDNS.IngressRDNSIPv4, ctx.Config.RDNS.ClusterRDNSIPv4, ctx.Config.RDNS.ClusterRDNS)
		rdnsIPv6 = resolveRDNSTemplate(ctx.Config.Ingress.RDNSIPv6, ctx.Config.RDNS.IngressRDNSIPv6, ctx.Config.RDNS.ClusterRDNSIPv6, ctx.Config.RDNS.ClusterRDNS)
	}

	// Apply IPv4 RDNS if configured
	if rdnsIPv4 != "" && ipv4 != "" {
		dnsPtr, err := rdns.RenderTemplate(rdnsIPv4, rdns.TemplateVars{
			ClusterName: ctx.Config.ClusterName,
			Hostname:    lbName,
			ID:          lbID,
			IPAddress:   ipv4,
			IPType:      "ipv4",
			Pool:        role,
			Role:        role,
		})
		if err != nil {
			return fmt.Errorf("failed to render IPv4 RDNS template: %w", err)
		}

		if err := ctx.Infra.SetLoadBalancerRDNS(ctx, lbID, ipv4, dnsPtr); err != nil {
			return fmt.Errorf("failed to set IPv4 RDNS: %w", err)
		}

		ctx.Logger.Printf("[%s] Set IPv4 RDNS: %s → %s", phase, ipv4, dnsPtr)
	}

	// Apply IPv6 RDNS if configured
	if rdnsIPv6 != "" && ipv6 != "" {
		dnsPtr, err := rdns.RenderTemplate(rdnsIPv6, rdns.TemplateVars{
			ClusterName: ctx.Config.ClusterName,
			Hostname:    lbName,
			ID:          lbID,
			IPAddress:   ipv6,
			IPType:      "ipv6",
			Pool:        role,
			Role:        role,
		})
		if err != nil {
			return fmt.Errorf("failed to render IPv6 RDNS template: %w", err)
		}

		if err := ctx.Infra.SetLoadBalancerRDNS(ctx, lbID, ipv6, dnsPtr); err != nil {
			return fmt.Errorf("failed to set IPv6 RDNS: %w", err)
		}

		ctx.Logger.Printf("[%s] Set IPv6 RDNS: %s → %s", phase, ipv6, dnsPtr)
	}

	return nil
}

// resolveRDNSTemplate returns the first non-empty template from the provided fallbacks.
func resolveRDNSTemplate(templates ...string) string {
	for _, t := range templates {
		if t != "" {
			return t
		}
	}
	return ""
}
