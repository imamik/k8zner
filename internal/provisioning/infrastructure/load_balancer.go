package infrastructure

import (
	"fmt"
	"net"
	"time"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/labels"
	"github.com/imamik/k8zner/internal/util/naming"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"
)

const (
	// Load balancer health check configuration for Kubernetes API
	kubeAPIHealthCheckInterval = 3 * time.Second
	kubeAPIHealthCheckTimeout  = 2 * time.Second
	kubeAPIHealthCheckRetries  = 2

	// Load balancer health check configuration for Talos API
	talosAPIHealthCheckInterval = 5 * time.Second
	talosAPIHealthCheckTimeout  = 3 * time.Second
	talosAPIHealthCheckRetries  = 2
)

// ProvisionLoadBalancers provisions API and Ingress load balancers.
func ProvisionLoadBalancers(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[%s] Reconciling load balancers for %s...", phase, ctx.Config.ClusterName)
	// API Load Balancer
	// Sum up control plane nodes
	cpCount := 0
	for _, pool := range ctx.Config.ControlPlane.NodePools {
		cpCount += pool.Count
	}

	if cpCount > 0 {
		// Name: ${cluster_name}-kube-api
		lbName := naming.KubeAPILoadBalancer(ctx.Config.ClusterName)
		ctx.Observer.Printf("[%s] Reconciling load balancer %s...", phase, lbName)

		apiLBLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
			WithRole("kube-api").
			WithTestIDIfSet(ctx.Config.TestID).
			Build()

		// Algorithm: round_robin
		apiLB, err := ctx.Infra.EnsureLoadBalancer(ctx, lbName, ctx.Config.Location, "lb11", hcloudgo.LoadBalancerAlgorithmTypeRoundRobin, apiLBLabels)
		if err != nil {
			return fmt.Errorf("failed to ensure API load balancer: %w", err)
		}

		// Service: 6443 (Kubernetes API)
		kubeAPIService := hcloudgo.LoadBalancerAddServiceOpts{
			Protocol:        hcloudgo.LoadBalancerServiceProtocolTCP,
			ListenPort:      hcloudgo.Ptr(6443),
			DestinationPort: hcloudgo.Ptr(6443),
			HealthCheck: &hcloudgo.LoadBalancerAddServiceOptsHealthCheck{
				Protocol: hcloudgo.LoadBalancerServiceProtocolHTTP,
				Port:     hcloudgo.Ptr(6443),
				Interval: hcloudgo.Ptr(kubeAPIHealthCheckInterval),
				Timeout:  hcloudgo.Ptr(kubeAPIHealthCheckTimeout),
				Retries:  hcloudgo.Ptr(kubeAPIHealthCheckRetries),
				HTTP: &hcloudgo.LoadBalancerAddServiceOptsHealthCheckHTTP{
					Path:        hcloudgo.Ptr("/version"),
					StatusCodes: []string{"401"},
					TLS:         hcloudgo.Ptr(true),
				},
			},
		}
		err = ctx.Infra.ConfigureService(ctx, apiLB, kubeAPIService)
		if err != nil {
			return err
		}

		// Service: 50000 (Talos API) - Enables CLI communication with control planes
		// via the LB, allowing private-first architecture where servers have no public IPv4.
		// This is used during bootstrap and for ongoing Talos operations (upgrades, etc.)
		talosAPIService := hcloudgo.LoadBalancerAddServiceOpts{
			Protocol:        hcloudgo.LoadBalancerServiceProtocolTCP,
			ListenPort:      hcloudgo.Ptr(50000),
			DestinationPort: hcloudgo.Ptr(50000),
			HealthCheck: &hcloudgo.LoadBalancerAddServiceOptsHealthCheck{
				Protocol: hcloudgo.LoadBalancerServiceProtocolTCP,
				Port:     hcloudgo.Ptr(50000),
				Interval: hcloudgo.Ptr(talosAPIHealthCheckInterval),
				Timeout:  hcloudgo.Ptr(talosAPIHealthCheckTimeout),
				Retries:  hcloudgo.Ptr(talosAPIHealthCheckRetries),
			},
		}
		err = ctx.Infra.ConfigureService(ctx, apiLB, talosAPIService)
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

		// Refresh LB from API to get updated info (private network IPs, etc.)
		// The local apiLB object doesn't have PrivateNet populated after AttachToNetwork
		refreshedLB, err := ctx.Infra.GetLoadBalancer(ctx, lbName)
		if err != nil {
			ctx.Observer.Printf("[%s] Warning: Failed to refresh LB after configuration: %v", phase, err)
			// Fall back to the local object
			ctx.State.LoadBalancer = apiLB
		} else {
			ctx.State.LoadBalancer = refreshedLB
		}
	}

	return nil
}
