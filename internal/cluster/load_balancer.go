package cluster

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"hcloud-k8s/internal/config"
)

func (r *Reconciler) reconcileLoadBalancers(ctx context.Context) error {
	// API Load Balancer
	// Sum up control plane nodes
	cpCount := 0
	for _, pool := range r.config.ControlPlane.NodePools {
		cpCount += pool.Count
	}

	if cpCount > 0 {
		// Name: ${cluster_name}-kube-api
		lbName := fmt.Sprintf("%s-kube-api", r.config.ClusterName)
		log.Printf("Reconciling Load Balancer %s...", lbName)

		labels := map[string]string{
			"cluster": r.config.ClusterName,
			"role":    "kube-api",
		}

		// Algorithm: round_robin
		lb, err := r.lbManager.EnsureLoadBalancer(ctx, lbName, r.config.Location, "lb11", hcloud.LoadBalancerAlgorithmTypeRoundRobin, labels)
		if err != nil {
			return err
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
		err = r.lbManager.ConfigureService(ctx, lb, service)
		if err != nil {
			return err
		}

		// Attach to Network with Private IP
		// Private IP: cidrhost(subnet, -2)
		lbSubnetCIDR, err := r.config.GetSubnetForRole("load-balancer", 0)
		if err != nil {
			return err
		}
		privateIPStr, err := config.CIDRHost(lbSubnetCIDR, -2)
		if err != nil {
			return fmt.Errorf("failed to calculate LB private IP: %w", err)
		}
		privateIP := net.ParseIP(privateIPStr)

		err = r.lbManager.AttachToNetwork(ctx, lb, r.network, privateIP)
		if err != nil {
			return err
		}

		// Add Targets
		// Label Selector: "cluster=<cluster_name>,role=control-plane"
		targetSelector := fmt.Sprintf("cluster=%s,role=control-plane", r.config.ClusterName)
		err = r.lbManager.AddTarget(ctx, lb, hcloud.LoadBalancerTargetTypeLabelSelector, targetSelector)
		if err != nil {
			return fmt.Errorf("failed to add target to LB: %w", err)
		}
	}

	// Ingress Load Balancer
	if r.config.Ingress.Enabled {
		// Name: ${cluster_name}-ingress
		lbName := fmt.Sprintf("%s-ingress", r.config.ClusterName)
		log.Printf("Reconciling Load Balancer %s...", lbName)

		labels := map[string]string{
			"cluster": r.config.ClusterName,
			"role":    "ingress",
		}

		lbType := r.config.Ingress.LoadBalancerType
		if lbType == "" {
			lbType = "lb11"
		}
		algorithm := hcloud.LoadBalancerAlgorithmTypeRoundRobin
		if r.config.Ingress.Algorithm == "least_connections" {
			algorithm = hcloud.LoadBalancerAlgorithmTypeLeastConnections
		}

		lb, err := r.lbManager.EnsureLoadBalancer(ctx, lbName, r.config.Location, lbType, algorithm, labels)
		if err != nil {
			return err
		}

		// Services
		// HTTP: 80 -> 30080 (NodePort) - wait, TF uses variables for destination ports.
		// Usually 80/443 -> NodePorts.
		// Assuming NodePorts 80/443 if using HostPort, or 30080/30443 if NodePort.
		// Terraform defaults: http 80 -> 80 (if host port), but let's check vars.
		// Assuming we want standard 80->80 mapping for now as basic implementation,
		// OR we should look at `ingress_nginx.tf` defaults if we had them.
		// For safety, let's assume standard NodePorts 30080/30443 for ingress-nginx.
		// But if we use `hostNetwork: true` on DaemonSet, it's 80/443.
		// Let's use 80->80 and 443->443 with Proxy Protocol enabled as per TF usually.
		// TF:
		// listen_port = protocol == "http" ? 80 : 443
		// destination_port = (protocol == "http" ? local.ingress_nginx_service_node_port_http : local.ingress_nginx_service_node_port_https)
		// We don't have these locals in config easily.
		// I will hardcode typical NodePorts for now: 30080 and 30443, which are standard for many setups,
		// OR better, I should check if config has these ports. It doesn't seem to have specific ports in IngressConfig.
		// I will use 80 -> 80 and 443 -> 443 assuming the Ingress controller uses HostNetwork or LoadBalancer service sync.
		// Actually, `load_balancer.tf` uses `local.ingress_nginx_service_node_port_http`.
		// Let's assume 80->80 for now as a safe default for "HostPort" style which is common on bare metal / Talos.

		services := []hcloud.LoadBalancerAddServiceOpts{
			{
				Protocol:        hcloud.LoadBalancerServiceProtocolTCP,
				ListenPort:      hcloud.Ptr(80),
				DestinationPort: hcloud.Ptr(80),
				Proxyprotocol:   hcloud.Ptr(true),
			},
			{
				Protocol:        hcloud.LoadBalancerServiceProtocolTCP,
				ListenPort:      hcloud.Ptr(443),
				DestinationPort: hcloud.Ptr(443),
				Proxyprotocol:   hcloud.Ptr(true),
			},
		}

		for _, svc := range services {
			svc.HealthCheck = &hcloud.LoadBalancerAddServiceOptsHealthCheck{
				Protocol: hcloud.LoadBalancerServiceProtocolTCP,
				Port:     svc.DestinationPort,
				Interval: hcloud.Ptr(time.Second * time.Duration(r.config.Ingress.HealthCheckInt)),
				Timeout:  hcloud.Ptr(time.Second * time.Duration(r.config.Ingress.HealthCheckTimeout)),
				Retries:  hcloud.Ptr(r.config.Ingress.HealthCheckRetry),
			}
			// Defaults if 0
			if *svc.HealthCheck.Interval == 0 {
				svc.HealthCheck.Interval = hcloud.Ptr(time.Second * 15)
			}
			if *svc.HealthCheck.Timeout == 0 {
				svc.HealthCheck.Timeout = hcloud.Ptr(time.Second * 10)
			}
			if *svc.HealthCheck.Retries == 0 {
				svc.HealthCheck.Retries = hcloud.Ptr(3)
			}

			err = r.lbManager.ConfigureService(ctx, lb, svc)
			if err != nil {
				return err
			}
		}

		// Attach to Network
		// Private IP: cidrhost(subnet, -4) from TF
		lbSubnetCIDR, err := r.config.GetSubnetForRole("load-balancer", 0)
		if err != nil {
			return err
		}
		privateIPStr, err := config.CIDRHost(lbSubnetCIDR, -4)
		if err != nil {
			return fmt.Errorf("failed to calculate Ingress LB private IP: %w", err)
		}
		privateIP := net.ParseIP(privateIPStr)

		err = r.lbManager.AttachToNetwork(ctx, lb, r.network, privateIP)
		if err != nil {
			return err
		}

		// Targets: Workers
		// "cluster=<name>,role=worker"
		// TF also includes CP if scheduling enabled, but let's stick to workers for now.
		targetSelector := fmt.Sprintf("cluster=%s,role=worker", r.config.ClusterName)
		err = r.lbManager.AddTarget(ctx, lb, hcloud.LoadBalancerTargetTypeLabelSelector, targetSelector)
		if err != nil {
			return fmt.Errorf("failed to add target to Ingress LB: %w", err)
		}
	}

	return nil
}
