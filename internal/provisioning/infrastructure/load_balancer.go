package infrastructure

import (
	"fmt"
	"net"
	"time"

	"github.com/imamik/k8zner/internal/config"
	hcloud "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/labels"
	"github.com/imamik/k8zner/internal/util/naming"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// ProvisionLoadBalancers provisions API and Ingress load balancers.
func (p *Provisioner) ProvisionLoadBalancers(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Reconciling load balancers for %s...", phase, ctx.Config.ClusterName)
	// API Load Balancer
	// Sum up control plane nodes
	cpCount := 0
	for _, pool := range ctx.Config.ControlPlane.NodePools {
		cpCount += pool.Count
	}

	if cpCount > 0 {
		// Name: ${cluster_name}-kube-api
		lbName := naming.KubeAPILoadBalancer(ctx.Config.ClusterName)
		ctx.Logger.Printf("[%s] Reconciling load balancer %s...", phase, lbName)

		apiLBLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
			WithRole("kube-api").
			WithTestIDIfSet(ctx.Config.TestID).
			Build()

		// Algorithm: round_robin
		apiLB, err := ctx.Infra.EnsureLoadBalancer(ctx, lbName, ctx.Config.Location, "lb11", hcloudgo.LoadBalancerAlgorithmTypeRoundRobin, apiLBLabels)
		if err != nil {
			return fmt.Errorf("failed to ensure API load balancer: %w", err)
		}

		// Service: 6443
		service := hcloudgo.LoadBalancerAddServiceOpts{
			Protocol:        hcloudgo.LoadBalancerServiceProtocolTCP,
			ListenPort:      hcloudgo.Ptr(6443),
			DestinationPort: hcloudgo.Ptr(6443),
			HealthCheck: &hcloudgo.LoadBalancerAddServiceOptsHealthCheck{
				Protocol: hcloudgo.LoadBalancerServiceProtocolHTTP,
				Port:     hcloudgo.Ptr(6443),
				Interval: hcloudgo.Ptr(time.Second * 3), // Terraform default: 3
				Timeout:  hcloudgo.Ptr(time.Second * 2), // Terraform default: 2
				Retries:  hcloudgo.Ptr(2),               // Terraform default: 2
				HTTP: &hcloudgo.LoadBalancerAddServiceOptsHealthCheckHTTP{
					Path:        hcloudgo.Ptr("/version"),
					StatusCodes: []string{"401"},
					TLS:         hcloudgo.Ptr(true),
				},
			},
		}
		err = ctx.Infra.ConfigureService(ctx, apiLB, service)
		if err != nil {
			return err
		}

		// Attach to Network with Private IP
		// Private IP: cidrhost(subnet, -2)
		lbSubnetCIDR, err := ctx.Config.GetSubnetForRole("load-balancer", 0)
		if err != nil {
			return err
		}
		privateIPStr, err := config.CIDRHost(lbSubnetCIDR, -2)
		if err != nil {
			return fmt.Errorf("failed to calculate LB private IP: %w", err)
		}
		privateIP := net.ParseIP(privateIPStr)

		err = ctx.Infra.AttachToNetwork(ctx, apiLB, ctx.State.Network, privateIP)
		if err != nil {
			return err
		}

		// Add Targets
		// Label Selector: "cluster=<cluster_name>,role=control-plane"
		targetSelector := fmt.Sprintf("cluster=%s,role=control-plane", ctx.Config.ClusterName)
		err = ctx.Infra.AddTarget(ctx, apiLB, hcloudgo.LoadBalancerTargetTypeLabelSelector, targetSelector)
		if err != nil {
			return fmt.Errorf("failed to add target to LB: %w", err)
		}

		// Apply RDNS if configured
		ipv4 := hcloud.LoadBalancerIPv4(apiLB)
		ipv6 := hcloud.LoadBalancerIPv6(apiLB)

		if err := p.applyLoadBalancerRDNS(ctx, apiLB.ID, lbName, ipv4, ipv6, "kube-api"); err != nil {
			ctx.Logger.Printf("[%s] Warning: Failed to set RDNS for %s: %v", phase, lbName, err)
		}

		// Refresh LB from API to get updated info (private network IPs, etc.)
		// The local apiLB object doesn't have PrivateNet populated after AttachToNetwork
		refreshedLB, err := ctx.Infra.GetLoadBalancer(ctx, lbName)
		if err != nil {
			ctx.Logger.Printf("[%s] Warning: Failed to refresh LB after configuration: %v", phase, err)
			// Fall back to the local object
			ctx.State.LoadBalancer = apiLB
		} else {
			ctx.State.LoadBalancer = refreshedLB
		}
	}

	// Ingress Load Balancer
	if ctx.Config.Ingress.Enabled {
		// Name: ${cluster_name}-ingress
		lbName := naming.IngressLoadBalancer(ctx.Config.ClusterName)
		ctx.Logger.Printf("[%s] Reconciling ingress load balancer %s...", phase, lbName)

		ingressLBLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
			WithRole("ingress").
			WithTestIDIfSet(ctx.Config.TestID).
			Build()

		lbType := ctx.Config.Ingress.LoadBalancerType
		if lbType == "" {
			lbType = "lb11"
		}
		algorithm := hcloudgo.LoadBalancerAlgorithmTypeRoundRobin
		if ctx.Config.Ingress.Algorithm == "least_connections" {
			algorithm = hcloudgo.LoadBalancerAlgorithmTypeLeastConnections
		}

		ingressLB, err := ctx.Infra.EnsureLoadBalancer(ctx, lbName, ctx.Config.Location, lbType, algorithm, ingressLBLabels)
		if err != nil {
			return err
		}

		// HTTP (80) and HTTPS (443) services with proxy protocol
		// Uses HostNetwork port mapping (80->80, 443->443) which is common for Talos/bare-metal
		for _, port := range []int{80, 443} {
			svc := newIngressService(port, ctx.Config.Ingress)
			if err := ctx.Infra.ConfigureService(ctx, ingressLB, svc); err != nil {
				return err
			}
		}

		// Attach to Network
		// Private IP: cidrhost(subnet, -4) from TF
		lbSubnetCIDR, err := ctx.Config.GetSubnetForRole("load-balancer", 0)
		if err != nil {
			return err
		}
		privateIPStr, err := config.CIDRHost(lbSubnetCIDR, -4)
		if err != nil {
			return fmt.Errorf("failed to calculate Ingress LB private IP: %w", err)
		}
		privateIP := net.ParseIP(privateIPStr)

		err = ctx.Infra.AttachToNetwork(ctx, ingressLB, ctx.State.Network, privateIP)
		if err != nil {
			return err
		}

		// Targets: Workers
		// "cluster=<name>,role=worker"
		// TF also includes CP if scheduling enabled, but let's stick to workers for now.
		targetSelector := fmt.Sprintf("cluster=%s,role=worker", ctx.Config.ClusterName)
		err = ctx.Infra.AddTarget(ctx, ingressLB, hcloudgo.LoadBalancerTargetTypeLabelSelector, targetSelector)
		if err != nil {
			return fmt.Errorf("failed to add target to Ingress LB: %w", err)
		}

		// Apply RDNS if configured
		ipv4 := hcloud.LoadBalancerIPv4(ingressLB)
		ipv6 := hcloud.LoadBalancerIPv6(ingressLB)

		if err := p.applyLoadBalancerRDNS(ctx, ingressLB.ID, lbName, ipv4, ipv6, "ingress"); err != nil {
			ctx.Logger.Printf("[%s] Warning: Failed to set RDNS for %s: %v", phase, lbName, err)
		}
	}

	return nil
}

// newIngressService creates a load balancer service for the given port with health check defaults.
func newIngressService(port int, cfg config.IngressConfig) hcloudgo.LoadBalancerAddServiceOpts {
	// Apply defaults for health check values
	interval := time.Second * time.Duration(cfg.HealthCheckInt)
	if interval == 0 {
		interval = time.Second * 15
	}
	timeout := time.Second * time.Duration(cfg.HealthCheckTimeout)
	if timeout == 0 {
		timeout = time.Second * 10
	}
	retries := cfg.HealthCheckRetry
	if retries == 0 {
		retries = 3
	}

	return hcloudgo.LoadBalancerAddServiceOpts{
		Protocol:        hcloudgo.LoadBalancerServiceProtocolTCP,
		ListenPort:      hcloudgo.Ptr(port),
		DestinationPort: hcloudgo.Ptr(port),
		Proxyprotocol:   hcloudgo.Ptr(true),
		HealthCheck: &hcloudgo.LoadBalancerAddServiceOptsHealthCheck{
			Protocol: hcloudgo.LoadBalancerServiceProtocolTCP,
			Port:     hcloudgo.Ptr(port),
			Interval: hcloudgo.Ptr(interval),
			Timeout:  hcloudgo.Ptr(timeout),
			Retries:  hcloudgo.Ptr(retries),
		},
	}
}
