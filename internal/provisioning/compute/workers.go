package compute

import (
	"context"
	"fmt"
	"log"
	"sync"

	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/async"
)

// ProvisionWorkers provisions worker node pools in parallel and returns a map of worker node names to their public IPs.
func (p *Provisioner) ProvisionWorkers(ctx *provisioning.Context) (map[string]string, error) {
	log.Printf("Reconciling Workers...")

	// Parallelize worker pool provisioning
	if len(ctx.Config.Workers) == 0 {
		log.Println("No worker pools configured")
		return nil, nil
	}

	log.Printf("Creating %d worker pools...", len(ctx.Config.Workers))

	// Collect IPs from all worker pools using mutex for thread-safe access
	var mu sync.Mutex
	allWorkerIPs := make(map[string]string)

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
					allWorkerIPs[name] = ip
				}
				mu.Unlock()
				return nil
			},
		}
	}

	// Execute all worker pool tasks in parallel
	if err := async.RunParallel(ctx, tasks, true); err != nil {
		return nil, err
	}

	log.Printf("Successfully created all worker pools")
	return allWorkerIPs, nil
}
