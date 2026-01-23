package compute

import (
	"context"
	"fmt"
	"sync"

	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/async"
	"hcloud-k8s/internal/util/rdns"
)

// ProvisionWorkers provisions worker node pools.
func (p *Provisioner) ProvisionWorkers(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Reconciling worker pools...", phase)

	// Parallelize worker pool provisioning
	if len(ctx.Config.Workers) == 0 {
		return nil
	}

	ctx.Logger.Printf("[%s] Creating %d worker pools...", phase, len(ctx.Config.Workers))

	// Collect IPs from all worker pools using mutex for thread-safe access
	var mu sync.Mutex

	// Build tasks for parallel execution
	tasks := make([]async.Task, len(ctx.Config.Workers))
	for i, pool := range ctx.Config.Workers {
		pool := pool // capture loop variable
		poolIndex := i
		tasks[i] = async.Task{
			Name: fmt.Sprintf("worker-pool-%s", pool.Name),
			Func: func(_ context.Context) error {
				// Resolve RDNS templates with fallback to cluster defaults
				rdnsIPv4 := rdns.ResolveTemplate(pool.RDNSIPv4, ctx.Config.RDNS.ClusterRDNSIPv4, ctx.Config.RDNS.ClusterRDNS)
				rdnsIPv6 := rdns.ResolveTemplate(pool.RDNSIPv6, ctx.Config.RDNS.ClusterRDNSIPv6, ctx.Config.RDNS.ClusterRDNS)

				// We use the outer ctx (*provisioning.Context)
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
				// Thread-safe merge of IPs
				mu.Lock()
				for name, ip := range ips {
					ctx.State.WorkerIPs[name] = ip
				}
				mu.Unlock()
				return nil
			},
		}
	}

	// Execute all worker pool tasks in parallel
	if err := async.RunParallel(ctx, tasks, true); err != nil {
		return fmt.Errorf("failed to provision worker pools: %w", err)
	}

	ctx.Logger.Printf("[%s] Successfully created all worker pools", phase)
	return nil
}
