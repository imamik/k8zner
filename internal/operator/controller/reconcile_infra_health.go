package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/log"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/util/naming"
)

// reconcileInfraHealth checks hcloud infrastructure health via API.
// Updates InfrastructureStatus.*Ready booleans.
// This is non-fatal — errors are logged but never returned.
func (r *ClusterReconciler) reconcileInfraHealth(ctx context.Context, cluster *k8znerv1alpha1.K8znerCluster) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("checking infrastructure health")

	infra := &cluster.Status.Infrastructure

	// Network
	networkName := naming.Network(cluster.Name)
	network, err := r.hcloudClient.GetNetwork(ctx, networkName)
	if err != nil {
		logger.V(1).Info("failed to check network", "error", err)
		infra.NetworkReady = false
	} else {
		infra.NetworkReady = network != nil
	}

	// Firewall
	firewallName := naming.Firewall(cluster.Name)
	firewall, err := r.hcloudClient.GetFirewall(ctx, firewallName)
	if err != nil {
		logger.V(1).Info("failed to check firewall", "error", err)
		infra.FirewallReady = false
	} else {
		infra.FirewallReady = firewall != nil
	}

	// Load Balancer
	lbName := naming.KubeAPILoadBalancer(cluster.Name)
	lb, err := r.hcloudClient.GetLoadBalancer(ctx, lbName)
	if err != nil {
		logger.V(1).Info("failed to check load balancer", "error", err)
		infra.LoadBalancerReady = false
	} else if lb != nil {
		infra.LoadBalancerReady = true
		if lb.PublicNet.Enabled && lb.PublicNet.IPv4.IP != nil {
			infra.LoadBalancerIP = lb.PublicNet.IPv4.IP.String()
		}
	} else {
		infra.LoadBalancerReady = false
	}

	logger.V(1).Info("infrastructure health check complete",
		"networkReady", infra.NetworkReady,
		"firewallReady", infra.FirewallReady,
		"loadBalancerReady", infra.LoadBalancerReady,
	)
}
