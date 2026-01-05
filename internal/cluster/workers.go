package cluster

import (
	"context"
	"fmt"
	"log"
	"time"
)

// reconcileWorkers provisions worker node pools in parallel.
func (r *Reconciler) reconcileWorkers(ctx context.Context) error {
	log.Printf("Reconciling Workers...")

	workerConfig, err := r.talosGenerator.GenerateWorkerConfig()
	if err != nil {
		return err
	}

	// Parallelize worker pool provisioning
	if len(r.config.Workers) == 0 {
		log.Println("No worker pools configured")
		return nil
	}

	log.Printf("=== CREATING %d WORKER POOLS IN PARALLEL at %s ===", len(r.config.Workers), time.Now().Format("15:04:05"))

	// Build tasks for parallel execution
	tasks := make([]Task, len(r.config.Workers))
	for i, pool := range r.config.Workers {
		tasks[i] = Task{
			Name: fmt.Sprintf("workerPool:%s", pool.Name),
			Func: func(ctx context.Context) error {
				// Placement Group (Managed inside reconcileNodePool for Workers due to sharding)
				// We pass nil here, and handle it inside reconcileNodePool based on pool config and index.
				_, err := r.reconcileNodePool(ctx, pool.Name, pool.Count, pool.ServerType, pool.Location, pool.Image, "worker", pool.Labels, string(workerConfig), nil, i)
				return err
			},
		}
	}

	// Execute all worker pools in parallel with logging
	if err := RunParallel(ctx, tasks, true); err != nil {
		return err
	}

	log.Printf("=== SUCCESSFULLY CREATED ALL WORKER POOLS at %s ===", time.Now().Format("15:04:05"))
	return nil
}
