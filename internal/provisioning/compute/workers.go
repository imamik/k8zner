package compute

import (
	"context"
	"fmt"
	"sync"

	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/async"
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
				// We use the outer ctx (*provisioning.Context)
				ips, err := p.reconcileNodePool(ctx, pool.Name, pool.Count, pool.ServerType, pool.Location, pool.Image, "worker", pool.Labels, "", nil, poolIndex)
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
