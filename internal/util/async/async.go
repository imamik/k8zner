// Package async provides utilities for parallel task execution.
//
// This package contains generic helpers for running multiple operations concurrently,
// collecting results, and handling errors. It's used across the codebase for parallel
// infrastructure provisioning and other concurrent workflows.
package async

import (
	"context"
	"fmt"
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
// Example:
//
//	tasks := []Task{
//	    {Name: "firewall", Func: r.reconcileFirewall},
//	    {Name: "loadBalancers", Func: r.reconcileLoadBalancers},
//	}
//	if err := RunParallel(ctx, tasks); err != nil {
//	    return err
//	}
func RunParallel(ctx context.Context, tasks []Task) error {
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
			err := task.Func(ctx)
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
