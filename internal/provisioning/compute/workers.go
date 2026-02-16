package compute

import (
	"context"
	"fmt"
	"sync"

	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/async"
)

// ProvisionWorkers provisions worker node pools.
func ProvisionWorkers(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[%s] Reconciling worker pools...", phase)

	// Parallelize worker pool provisioning
	if len(ctx.Config.Workers) == 0 {
		return nil
	}

	ctx.Observer.Printf("[%s] Creating %d worker pools...", phase, len(ctx.Config.Workers))

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
				poolResult, err := reconcileNodePool(ctx, NodePoolSpec{
					Name:             pool.Name,
					Count:            pool.Count,
					ServerType:       pool.ServerType,
					Location:         pool.Location,
					Image:            pool.Image,
					Role:             "worker",
					ExtraLabels:      pool.Labels,
					PoolIndex:        poolIndex,
					EnablePublicIPv4: ctx.Config.ShouldEnablePublicIPv4(),
					EnablePublicIPv6: ctx.Config.ShouldEnablePublicIPv6(),
				})
				if err != nil {
					return err
				}
				// Thread-safe merge of IPs and server IDs
				mu.Lock()
				for name, ip := range poolResult.IPs {
					ctx.State.WorkerIPs[name] = ip
				}
				for name, id := range poolResult.ServerIDs {
					ctx.State.WorkerServerIDs[name] = id
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

	ctx.Observer.Printf("[%s] Successfully created all worker pools", phase)
	return nil
}
