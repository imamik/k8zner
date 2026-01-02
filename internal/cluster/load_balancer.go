package cluster

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func (r *Reconciler) reconcileLoadBalancers(ctx context.Context) error {
	// API Load Balancer
	// Sum up control plane nodes
	cpCount := 0
	for _, pool := range r.config.ControlPlane.NodePools {
		cpCount += pool.Count
	}

	if cpCount > 0 {
		lbName := fmt.Sprintf("%s-control-plane", r.config.ClusterName)
		log.Printf("Reconciling Load Balancer %s...", lbName)

		labels := map[string]string{
			"cluster": r.config.ClusterName,
			"role": "control-plane-lb",
		}

		lb, err := r.lbManager.EnsureLoadBalancer(ctx, lbName, r.config.Location, "lb11", hcloud.LoadBalancerAlgorithmTypeLeastConnections, labels)
		if err != nil {
			return err
		}

		// Service: 6443
		service := hcloud.LoadBalancerAddServiceOpts{
			Protocol:        hcloud.LoadBalancerServiceProtocolTCP,
			ListenPort:      ptr(6443),
			DestinationPort: ptr(6443),
			HealthCheck: &hcloud.LoadBalancerAddServiceOptsHealthCheck{
				Protocol: hcloud.LoadBalancerServiceProtocolHTTP,
				Port:     ptr(6443),
				Interval: ptr(time.Second * 10),
				Timeout:  ptr(time.Second * 5),
				Retries:  ptr(3),
				HTTP: &hcloud.LoadBalancerAddServiceOptsHealthCheckHTTP{
					Path:        ptr("/version"),
					StatusCodes: []string{"401"},
					TLS:         ptr(true),
				},
			},
		}
		err = r.lbManager.ConfigureService(ctx, lb, service)
		if err != nil {
			return err
		}

		err = r.lbManager.AttachToNetwork(ctx, lb, r.network, nil)
		if err != nil {
			return err
		}
	}
	return nil
}
