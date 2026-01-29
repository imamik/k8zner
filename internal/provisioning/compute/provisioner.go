package compute

import (
	"context"
	"fmt"
	"sync"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/async"
	"github.com/imamik/k8zner/internal/util/labels"
	"github.com/imamik/k8zner/internal/util/naming"
	"github.com/imamik/k8zner/internal/util/rdns"
)

// Provisioner handles compute resource provisioning (servers, node pools).
type Provisioner struct{}

// NewProvisioner creates a new compute provisioner.
func NewProvisioner() *Provisioner {
	return &Provisioner{}
}

// Name implements the provisioning.Phase interface.
func (p *Provisioner) Name() string {
	return "compute"
}

// Provision implements the provisioning.Phase interface.
// This method creates ALL servers (control plane + workers) in parallel
// for maximum provisioning speed.
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	// 1. Pre-compute: Setup LB endpoint and collect SANs
	if err := p.prepareControlPlaneEndpoint(ctx); err != nil {
		return err
	}

	// 2. Create ALL servers in parallel (CP + Workers together)
	if err := p.provisionAllServers(ctx); err != nil {
		return err
	}

	return nil
}

// prepareControlPlaneEndpoint sets up the Talos endpoint from the Load Balancer.
// This must run before server creation so Talos configs have the right endpoint.
func (p *Provisioner) prepareControlPlaneEndpoint(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Preparing control plane endpoint...", phase)

	var sans []string

	// Get LB IP for endpoint
	lb, err := ctx.Infra.GetLoadBalancer(ctx, naming.KubeAPILoadBalancer(ctx.Config.ClusterName))
	if err != nil {
		return fmt.Errorf("failed to get load balancer: %w", err)
	}
	if lb != nil {
		if lbIP := hcloud.LoadBalancerIPv4(lb); lbIP != "" {
			sans = append(sans, lbIP)
			endpoint := fmt.Sprintf("https://%s:%d", lbIP, config.KubeAPIPort)
			ctx.Logger.Printf("[%s] Setting Talos endpoint to: %s", phase, endpoint)
			ctx.Talos.SetEndpoint(endpoint)
		}

		for _, net := range lb.PrivateNet {
			sans = append(sans, net.IP.String())
		}
	}

	ctx.State.SANs = sans
	return nil
}

// provisionAllServers creates all control plane and worker servers in parallel.
// This significantly reduces total provisioning time by overlapping server creation.
func (p *Provisioner) provisionAllServers(ctx *provisioning.Context) error {
	var tasks []async.Task
	var mu sync.Mutex

	// Build tasks for control plane pools
	for i, pool := range ctx.Config.ControlPlane.NodePools {
		pool := pool
		poolIndex := i
		tasks = append(tasks, async.Task{
			Name: fmt.Sprintf("cp-pool-%s", pool.Name),
			Func: func(_ context.Context) error {
				// Ensure placement group
				pgLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
					WithPool(pool.Name).
					WithTestIDIfSet(ctx.Config.TestID).
					Build()

				pg, err := ctx.Infra.EnsurePlacementGroup(ctx, naming.PlacementGroup(ctx.Config.ClusterName, pool.Name), "spread", pgLabels)
				if err != nil {
					return fmt.Errorf("failed to ensure placement group for pool %s: %w", pool.Name, err)
				}

				// Resolve RDNS templates
				rdnsIPv4 := rdns.ResolveTemplate(pool.RDNSIPv4, ctx.Config.RDNS.ClusterRDNSIPv4, ctx.Config.RDNS.ClusterRDNS)
				rdnsIPv6 := rdns.ResolveTemplate(pool.RDNSIPv6, ctx.Config.RDNS.ClusterRDNSIPv6, ctx.Config.RDNS.ClusterRDNS)

				poolIPs, err := p.reconcileNodePool(ctx, NodePoolSpec{
					Name:             pool.Name,
					Count:            pool.Count,
					ServerType:       pool.ServerType,
					Location:         pool.Location,
					Image:            pool.Image,
					Role:             "control-plane",
					ExtraLabels:      pool.Labels,
					UserData:         "",
					PlacementGroupID: &pg.ID,
					PoolIndex:        poolIndex,
					RDNSIPv4:         rdnsIPv4,
					RDNSIPv6:         rdnsIPv6,
				})
				if err != nil {
					return err
				}

				mu.Lock()
				for k, v := range poolIPs {
					ctx.State.ControlPlaneIPs[k] = v
				}
				mu.Unlock()
				return nil
			},
		})
	}

	// Build tasks for worker pools
	for i, pool := range ctx.Config.Workers {
		pool := pool
		poolIndex := i
		tasks = append(tasks, async.Task{
			Name: fmt.Sprintf("worker-pool-%s", pool.Name),
			Func: func(_ context.Context) error {
				rdnsIPv4 := rdns.ResolveTemplate(pool.RDNSIPv4, ctx.Config.RDNS.ClusterRDNSIPv4, ctx.Config.RDNS.ClusterRDNS)
				rdnsIPv6 := rdns.ResolveTemplate(pool.RDNSIPv6, ctx.Config.RDNS.ClusterRDNSIPv6, ctx.Config.RDNS.ClusterRDNS)

				ips, err := p.reconcileNodePool(ctx, NodePoolSpec{
					Name:             pool.Name,
					Count:            pool.Count,
					ServerType:       pool.ServerType,
					Location:         pool.Location,
					Image:            pool.Image,
					Role:             "worker",
					ExtraLabels:      pool.Labels,
					UserData:         "",
					PlacementGroupID: nil,
					PoolIndex:        poolIndex,
					RDNSIPv4:         rdnsIPv4,
					RDNSIPv6:         rdnsIPv6,
				})
				if err != nil {
					return err
				}

				mu.Lock()
				for name, ip := range ips {
					ctx.State.WorkerIPs[name] = ip
				}
				mu.Unlock()
				return nil
			},
		})
	}

	if len(tasks) == 0 {
		return nil
	}

	totalCPs := 0
	for _, pool := range ctx.Config.ControlPlane.NodePools {
		totalCPs += pool.Count
	}
	totalWorkers := 0
	for _, pool := range ctx.Config.Workers {
		totalWorkers += pool.Count
	}

	ctx.Logger.Printf("[%s] Creating %d control plane + %d worker servers in parallel...", phase, totalCPs, totalWorkers)

	if err := async.RunParallel(ctx, tasks, true); err != nil {
		return fmt.Errorf("failed to provision servers: %w", err)
	}

	ctx.Logger.Printf("[%s] Successfully created all %d servers", phase, totalCPs+totalWorkers)
	return nil
}
