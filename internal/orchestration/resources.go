package orchestration

import (
	"context"

	"hcloud-k8s/internal/util/async"
)

// provisionImagesAndInfrastructure provisions images, firewall, load balancers, and floating IPs in parallel.
func (r *Reconciler) provisionImagesAndInfrastructure(ctx context.Context) error {
	// Pre-build images and fetch public IP in parallel
	publicIP, err := r.buildImagesAndFetchPublicIP(ctx)
	if err != nil {
		return err
	}

	// Provision firewall, load balancers, and floating IPs in parallel
	return r.provisionFirewallAndLoadBalancers(ctx, publicIP)
}

// buildImagesAndFetchPublicIP pre-builds Talos images and fetches public IP in parallel.
func (r *Reconciler) buildImagesAndFetchPublicIP(ctx context.Context) (string, error) {
	var publicIP string
	tasks := []async.Task{
		{
			Name: "images",
			Func: r.imageProvisioner.EnsureAllImages,
		},
		{
			Name: "publicIP",
			Func: func(ctx context.Context) error {
				ip, err := r.infra.GetPublicIP(ctx)
				if err == nil {
					publicIP = ip
				}
				return nil
			},
		},
	}

	if err := async.RunParallel(ctx, tasks, false); err != nil {
		return "", err
	}

	return publicIP, nil
}

// provisionFirewallAndLoadBalancers provisions firewall, load balancers, and floating IPs in parallel.
func (r *Reconciler) provisionFirewallAndLoadBalancers(ctx context.Context, publicIP string) error {
	tasks := []async.Task{
		{
			Name: "firewall",
			Func: func(ctx context.Context) error {
				return r.infraProvisioner.ProvisionFirewall(ctx, publicIP)
			},
		},
		{Name: "loadBalancers", Func: r.infraProvisioner.ProvisionLoadBalancers},
		{Name: "floatingIPs", Func: r.infraProvisioner.ProvisionFloatingIPs},
	}

	return async.RunParallel(ctx, tasks, false)
}
