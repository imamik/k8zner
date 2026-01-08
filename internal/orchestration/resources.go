package orchestration

import (
	"context"

	"hcloud-k8s/internal/util/async"
)

// provisionResources provisions infrastructure resources in parallel.
func (r *Reconciler) provisionResources(ctx context.Context) error {
	// Pre-build images and fetch public IP in parallel
	publicIP, err := r.provisionImagesAndIP(ctx)
	if err != nil {
		return err
	}

	// Provision infrastructure resources in parallel
	return r.provisionInfrastructure(ctx, publicIP)
}

// provisionImagesAndIP pre-builds images and fetches public IP in parallel.
func (r *Reconciler) provisionImagesAndIP(ctx context.Context) (string, error) {
	var publicIP string
	imageTasks := []async.Task{
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

	if err := async.RunParallel(ctx, imageTasks, false); err != nil {
		return "", err
	}

	return publicIP, nil
}

// provisionInfrastructure provisions firewall, load balancers, and floating IPs in parallel.
func (r *Reconciler) provisionInfrastructure(ctx context.Context, publicIP string) error {
	infraTasks := []async.Task{
		{
			Name: "firewall",
			Func: func(ctx context.Context) error {
				return r.infraProvisioner.ProvisionFirewall(ctx, publicIP)
			},
		},
		{Name: "loadBalancers", Func: r.infraProvisioner.ProvisionLoadBalancers},
		{Name: "floatingIPs", Func: r.infraProvisioner.ProvisionFloatingIPs},
	}

	return async.RunParallel(ctx, infraTasks, false)
}
