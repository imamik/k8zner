package cluster

import (
	"context"
	"log"
	"time"
)

// reconcileWorkers provisions worker node pools in parallel.
// Returns a map of worker node names to their public IPs.
func (r *Reconciler) reconcileWorkers(ctx context.Context) (map[string]string, error) {
	log.Printf("Reconciling Workers...")

	// Parallelize worker pool provisioning
	if len(r.config.Workers) == 0 {
		log.Println("No worker pools configured")
		return nil, nil
	}

	log.Printf("=== CREATING %d WORKER POOLS IN PARALLEL at %s ===", len(r.config.Workers), time.Now().Format("15:04:05"))

	// Collect IPs from all worker pools
	type poolResult struct {
		ips map[string]string
		err error
	}
	resultChan := make(chan poolResult, len(r.config.Workers))

	// Build tasks for parallel execution
	for i, pool := range r.config.Workers {
		pool := pool // capture loop variable
		poolIndex := i
		go func() {
			// Placement Group (Managed inside reconcileNodePool for Workers due to sharding)
			// We pass nil here, and handle it inside reconcileNodePool based on pool config and index.
			// userData is empty as configs will be generated and applied per-node in the reconciler
			ips, err := r.reconcileNodePool(ctx, pool.Name, pool.Count, pool.ServerType, pool.Location, pool.Image, "worker", pool.Labels, "", nil, poolIndex)
			resultChan <- poolResult{ips: ips, err: err}
		}()
	}

	// Collect results from all worker pools
	allWorkerIPs := make(map[string]string)
	for i := 0; i < len(r.config.Workers); i++ {
		result := <-resultChan
		if result.err != nil {
			return nil, result.err
		}
		// Merge IPs from this pool
		for name, ip := range result.ips {
			allWorkerIPs[name] = ip
		}
	}

	log.Printf("=== SUCCESSFULLY CREATED ALL WORKER POOLS at %s ===", time.Now().Format("15:04:05"))
	return allWorkerIPs, nil
}
