package cluster

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/config"
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
	}
	return nil
}
