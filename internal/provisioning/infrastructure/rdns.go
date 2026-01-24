package infrastructure

import (
	"fmt"

	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/rdns"
)

// applyLoadBalancerRDNS configures reverse DNS for a load balancer's IP addresses.
func (p *Provisioner) applyLoadBalancerRDNS(ctx *provisioning.Context, lbID int64, lbName, ipv4, ipv6, role string) error {
	// Determine RDNS templates based on role
	var rdnsIPv4, rdnsIPv6 string

	switch role {
	case "kube-api":
		// Kube API load balancer uses cluster-wide defaults
		rdnsIPv4 = rdns.ResolveTemplate(ctx.Config.RDNS.ClusterRDNSIPv4, ctx.Config.RDNS.ClusterRDNS, "")
		rdnsIPv6 = rdns.ResolveTemplate(ctx.Config.RDNS.ClusterRDNSIPv6, ctx.Config.RDNS.ClusterRDNS, "")
	case "ingress":
		// Ingress load balancer uses ingress-specific config with fallback
		rdnsIPv4 = rdns.ResolveTemplate(ctx.Config.Ingress.RDNSIPv4, ctx.Config.RDNS.IngressRDNSIPv4, ctx.Config.RDNS.ClusterRDNSIPv4, ctx.Config.RDNS.ClusterRDNS)
		rdnsIPv6 = rdns.ResolveTemplate(ctx.Config.Ingress.RDNSIPv6, ctx.Config.RDNS.IngressRDNSIPv6, ctx.Config.RDNS.ClusterRDNSIPv6, ctx.Config.RDNS.ClusterRDNS)
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
			return fmt.Errorf("failed to render IPv4 RDNS template for %s (template: %s): %w", lbName, rdnsIPv4, err)
		}

		if err := ctx.Infra.SetLoadBalancerRDNS(ctx, lbID, ipv4, dnsPtr); err != nil {
			return fmt.Errorf("failed to set IPv4 RDNS for load balancer %s (ID: %d, IP: %s → %s): %w", lbName, lbID, ipv4, dnsPtr, err)
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
			return fmt.Errorf("failed to render IPv6 RDNS template for %s (template: %s): %w", lbName, rdnsIPv6, err)
		}

		if err := ctx.Infra.SetLoadBalancerRDNS(ctx, lbID, ipv6, dnsPtr); err != nil {
			return fmt.Errorf("failed to set IPv6 RDNS for load balancer %s (ID: %d, IP: %s → %s): %w", lbName, lbID, ipv6, dnsPtr, err)
		}

		ctx.Logger.Printf("[%s] Set IPv6 RDNS: %s → %s", phase, ipv6, dnsPtr)
	}

	return nil
}
