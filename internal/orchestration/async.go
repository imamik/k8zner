package orchestration

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Task represents an asynchronous operation with a name and function.
type Task struct {
	Name string
	Func func(context.Context) error
}

// RunParallel executes multiple tasks in parallel and returns the first error encountered.
// All tasks are started concurrently, and the function waits for all to complete.
// If any task returns an error, the first error is returned after all tasks finish.
//
// Set withLogging to true to log task start and completion times, which is useful
// for tracking infrastructure provisioning progress.
//
// Example:
//
//	tasks := []Task{
//	    {Name: "firewall", Func: r.reconcileFirewall},
//	    {Name: "loadBalancers", Func: r.reconcileLoadBalancers},
//	}
//	if err := RunParallel(ctx, tasks, false); err != nil {
//	    return err
//	}
func RunParallel(ctx context.Context, tasks []Task, withLogging bool) error {
	if len(tasks) == 0 {
		return nil
	}

	type result struct {
		name string
		err  error
	}

	resultChan := make(chan result, len(tasks))

	// Start all tasks
	for _, task := range tasks {
		go func() {
			if withLogging {
				log.Printf("[%s] Starting at %s", task.Name, time.Now().Format("15:04:05"))
			}
			err := task.Func(ctx)
			if withLogging {
				log.Printf("[%s] Completed at %s", task.Name, time.Now().Format("15:04:05"))
			}
			resultChan <- result{name: task.Name, err: err}
		}()
	}

	// Wait for all tasks to complete and collect first error
	var firstError error
	for range len(tasks) {
		res := <-resultChan
		if res.err != nil && firstError == nil {
			firstError = fmt.Errorf("failed to reconcile %s: %w", res.name, res.err)
		}
	}

	return firstError
}
