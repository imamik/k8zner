package async

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunParallel_Success(t *testing.T) {
	var count atomic.Int32

	tasks := []Task{
		{Name: "task1", Func: func(_ context.Context) error {
			count.Add(1)
			return nil
		}},
		{Name: "task2", Func: func(_ context.Context) error {
			count.Add(1)
			return nil
		}},
		{Name: "task3", Func: func(_ context.Context) error {
			count.Add(1)
			return nil
		}},
	}

	err := RunParallel(context.Background(), tasks, false)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if count.Load() != 3 {
		t.Errorf("expected 3 tasks to run, got %d", count.Load())
	}
}

func TestRunParallel_EmptyTasks(t *testing.T) {
	err := RunParallel(context.Background(), nil, false)
	if err != nil {
		t.Errorf("expected no error for empty tasks, got: %v", err)
	}

	err = RunParallel(context.Background(), []Task{}, false)
	if err != nil {
		t.Errorf("expected no error for empty slice, got: %v", err)
	}
}

func TestRunParallel_SingleError(t *testing.T) {
	expectedErr := errors.New("task failed")

	tasks := []Task{
		{Name: "success", Func: func(_ context.Context) error {
			return nil
		}},
		{Name: "failing", Func: func(_ context.Context) error {
			return expectedErr
		}},
	}

	err := RunParallel(context.Background(), tasks, false)
	if err == nil {
		t.Error("expected error, got nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap %v, got: %v", expectedErr, err)
	}
}

func TestRunParallel_MultipleErrors(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	tasks := []Task{
		{Name: "fail1", Func: func(_ context.Context) error {
			return err1
		}},
		{Name: "fail2", Func: func(_ context.Context) error {
			return err2
		}},
	}

	err := RunParallel(context.Background(), tasks, false)
	if err == nil {
		t.Error("expected error, got nil")
	}

	// With errors.Join, both errors should be accessible
	if !errors.Is(err, err1) {
		t.Errorf("expected error to contain err1, got: %v", err)
	}
	if !errors.Is(err, err2) {
		t.Errorf("expected error to contain err2, got: %v", err)
	}
}

func TestRunParallel_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var executed atomic.Bool

	tasks := []Task{
		{Name: "task", Func: func(_ context.Context) error {
			executed.Store(true)
			// Task respects context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		}},
	}

	err := RunParallel(ctx, tasks, false)
	if err == nil {
		t.Error("expected error due to context cancellation")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

func TestRunParallel_AllTasksComplete(t *testing.T) {
	var completed atomic.Int32
	started := make(chan struct{}, 3)

	tasks := []Task{
		{Name: "fast-fail", Func: func(_ context.Context) error {
			started <- struct{}{}
			return errors.New("fast fail")
		}},
		{Name: "slow-success-1", Func: func(_ context.Context) error {
			started <- struct{}{}
			time.Sleep(50 * time.Millisecond)
			completed.Add(1)
			return nil
		}},
		{Name: "slow-success-2", Func: func(_ context.Context) error {
			started <- struct{}{}
			time.Sleep(50 * time.Millisecond)
			completed.Add(1)
			return nil
		}},
	}

	_ = RunParallel(context.Background(), tasks, false)

	// All tasks should have started
	for range 3 {
		select {
		case <-started:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("not all tasks started")
		}
	}

	// Wait for slow tasks to complete
	time.Sleep(100 * time.Millisecond)

	// All tasks should have completed even though one failed fast
	if completed.Load() != 2 {
		t.Errorf("expected 2 slow tasks to complete, got %d", completed.Load())
	}
}

func TestRunParallel_Concurrent(t *testing.T) {
	var maxConcurrent atomic.Int32
	var current atomic.Int32

	tasks := make([]Task, 5)
	for i := range tasks {
		tasks[i] = Task{
			Name: "task",
			Func: func(_ context.Context) error {
				c := current.Add(1)
				// Track max concurrent
				for {
					old := maxConcurrent.Load()
					if c <= old || maxConcurrent.CompareAndSwap(old, c) {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
				current.Add(-1)
				return nil
			},
		}
	}

	err := RunParallel(context.Background(), tasks, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// All tasks should run concurrently
	if maxConcurrent.Load() != 5 {
		t.Errorf("expected 5 concurrent tasks, got %d", maxConcurrent.Load())
	}
}

func TestRunParallel_TaskNameInError(t *testing.T) {
	tasks := []Task{
		{Name: "specific-task-name", Func: func(_ context.Context) error {
			return errors.New("task error")
		}},
	}

	err := RunParallel(context.Background(), tasks, false)
	if err == nil {
		t.Fatal("expected error")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "specific-task-name") {
		t.Errorf("error message should contain task name, got: %s", errStr)
	}
}
