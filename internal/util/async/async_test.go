package async

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRunParallel_Success(t *testing.T) {
	ctx := context.Background()
	taskCount := 5
	var mu sync.Mutex
	completed := 0

	tasks := make([]Task, taskCount)
	for i := 0; i < taskCount; i++ {
		tasks[i] = Task{
			Name: "test-task",
			Func: func(ctx context.Context) error {
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				completed++
				mu.Unlock()
				return nil
			},
		}
	}

	err := RunParallel(ctx, tasks)
	if err != nil {
		t.Errorf("RunParallel failed: %v", err)
	}

	if completed != taskCount {
		t.Errorf("Expected %d completed tasks, got %d", taskCount, completed)
	}
}

func TestRunParallel_Error(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("task failed")

	tasks := []Task{
		{
			Name: "success-task",
			Func: func(ctx context.Context) error {
				return nil
			},
		},
		{
			Name: "fail-task",
			Func: func(ctx context.Context) error {
				return expectedErr
			},
		},
	}

	err := RunParallel(ctx, tasks)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// The error message should wrap the original error and include the task name
	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected error wrapping '%v', got '%v'", expectedErr, err)
	}
}

func TestRunParallel_Empty(t *testing.T) {
	ctx := context.Background()
	err := RunParallel(ctx, []Task{})
	if err != nil {
		t.Errorf("Expected nil error for empty tasks, got: %v", err)
	}
}

func TestRunParallel_Concurrency(t *testing.T) {
	ctx := context.Background()
	start := time.Now()
	sleepTime := 100 * time.Millisecond

	// Run 3 tasks that each sleep for 100ms
	// If sequential, would take 300ms
	// If parallel, should take ~100ms
	tasks := []Task{
		{Name: "t1", Func: func(ctx context.Context) error { time.Sleep(sleepTime); return nil }},
		{Name: "t2", Func: func(ctx context.Context) error { time.Sleep(sleepTime); return nil }},
		{Name: "t3", Func: func(ctx context.Context) error { time.Sleep(sleepTime); return nil }},
	}

	err := RunParallel(ctx, tasks)
	if err != nil {
		t.Errorf("RunParallel failed: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > 2*sleepTime {
		t.Errorf("Tasks took %v, expected parallel execution (~%v)", elapsed, sleepTime)
	}
}
