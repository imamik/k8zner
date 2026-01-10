package infrastructure

import (
	"fmt"
	"net"
	"time"

	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/naming"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
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

		labels := map[string]string{
			"cluster": ctx.Config.ClusterName,
			"role":    "kube-api",
		}

		// Algorithm: round_robin
		lb, err := ctx.Infra.EnsureLoadBalancer(ctx, lbName, ctx.Config.Network.Zone, "lb11", hcloud.LoadBalancerAlgorithmTypeRoundRobin, labels)
		if err != nil {
			return fmt.Errorf("failed to ensure API load balancer: %w", err)
		}

		// Service: 6443
		service := hcloud.LoadBalancerAddServiceOpts{
			Protocol:        hcloud.LoadBalancerServiceProtocolTCP,
			ListenPort:      hcloud.Ptr(6443),
			DestinationPort: hcloud.Ptr(6443),
			HealthCheck: &hcloud.LoadBalancerAddServiceOptsHealthCheck{
				Protocol: hcloud.LoadBalancerServiceProtocolHTTP,
				Port:     hcloud.Ptr(6443),
				Interval: hcloud.Ptr(time.Second * 3), // Terraform default: 3
				Timeout:  hcloud.Ptr(time.Second * 2), // Terraform default: 2
				Retries:  hcloud.Ptr(2),               // Terraform default: 2
				HTTP: &hcloud.LoadBalancerAddServiceOptsHealthCheckHTTP{
					Path:        hcloud.Ptr("/version"),
					StatusCodes: []string{"401"},
					TLS:         hcloud.Ptr(true),
				},
			},
		}
		err = ctx.Infra.ConfigureService(ctx, lb, service)
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

		err = ctx.Infra.AttachToNetwork(ctx, lb, ctx.State.Network, privateIP)
		if err != nil {
			return err
		}

		// Add Targets
		// Label Selector: "cluster=<cluster_name>,role=control-plane"
		targetSelector := fmt.Sprintf("cluster=%s,role=control-plane", ctx.Config.ClusterName)
		err = ctx.Infra.AddTarget(ctx, lb, hcloud.LoadBalancerTargetTypeLabelSelector, targetSelector)
		if err != nil {
			return fmt.Errorf("failed to add target to LB: %w", err)
		}
	}

	// Ingress Load Balancer
	if ctx.Config.Ingress.Enabled {
		// Name: ${cluster_name}-ingress
		lbName := naming.IngressLoadBalancer(ctx.Config.ClusterName)
		ctx.Logger.Printf("[%s] Reconciling ingress load balancer %s...", phase, lbName)

		labels := map[string]string{
			"cluster": ctx.Config.ClusterName,
			"role":    "ingress",
		}

		lbType := ctx.Config.Ingress.LoadBalancerType
		if lbType == "" {
			lbType = "lb11"
		}
		algorithm := hcloud.LoadBalancerAlgorithmTypeRoundRobin
		if ctx.Config.Ingress.Algorithm == "least_connections" {
			algorithm = hcloud.LoadBalancerAlgorithmTypeLeastConnections
		}

		lb, err := ctx.Infra.EnsureLoadBalancer(ctx, lbName, ctx.Config.Location, lbType, algorithm, labels)
		if err != nil {
			return err
		}

		// HTTP (80) and HTTPS (443) services with proxy protocol
		// Uses HostNetwork port mapping (80->80, 443->443) which is common for Talos/bare-metal
		for _, port := range []int{80, 443} {
			svc := newIngressService(port, ctx.Config.Ingress)
			if err := ctx.Infra.ConfigureService(ctx, lb, svc); err != nil {
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

		err = ctx.Infra.AttachToNetwork(ctx, lb, ctx.State.Network, privateIP)
		if err != nil {
			return err
		}

		// Targets: Workers
		// "cluster=<name>,role=worker"
		// TF also includes CP if scheduling enabled, but let's stick to workers for now.
		targetSelector := fmt.Sprintf("cluster=%s,role=worker", ctx.Config.ClusterName)
		err = ctx.Infra.AddTarget(ctx, lb, hcloud.LoadBalancerTargetTypeLabelSelector, targetSelector)
		if err != nil {
			return fmt.Errorf("failed to add target to Ingress LB: %w", err)
		}
	}

	return nil
}

// newIngressService creates a load balancer service for the given port with health check defaults.
func newIngressService(port int, cfg config.IngressConfig) hcloud.LoadBalancerAddServiceOpts {
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

	return hcloud.LoadBalancerAddServiceOpts{
		Protocol:        hcloud.LoadBalancerServiceProtocolTCP,
		ListenPort:      hcloud.Ptr(port),
		DestinationPort: hcloud.Ptr(port),
		Proxyprotocol:   hcloud.Ptr(true),
		HealthCheck: &hcloud.LoadBalancerAddServiceOptsHealthCheck{
			Protocol: hcloud.LoadBalancerServiceProtocolTCP,
			Port:     hcloud.Ptr(port),
			Interval: hcloud.Ptr(interval),
			Timeout:  hcloud.Ptr(timeout),
			Retries:  hcloud.Ptr(retries),
		},
	}
}
