package hcloud

import (
	"context"
	"fmt"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/retry"
)

// Timeouts mirror of config.Timeouts to avoid circular dependency if needed.
// However, since we import internal/config in real_client.go, we might be able to use it if we are careful.
// But generic.go is part of hcloud package, so it cannot import config if config imports hcloud.
// Let's assume Timeouts struct here is local for generic helper.
type Timeouts struct {
	Delete            time.Duration
	RetryMaxAttempts  int
	RetryInitialDelay time.Duration
}

// ReconcileFuncs defines the functions required for generic reconciliation.
type ReconcileFuncs[T any] struct {
	// Get retrieves the resource by name.
	Get func(ctx context.Context, name string) (*T, error)
	// Create creates the resource.
	Create func(ctx context.Context) (*T, error)
	// Update updates the resource if needed.
	// If nil, no update is performed.
	Update func(ctx context.Context, resource *T) (*T, error)
	// NeedsUpdate checks if the resource needs to be updated.
	// If nil, and Update is not nil, Update is always called (idempotency assumed or simple existence check).
	NeedsUpdate func(resource *T) bool
}

// reconcileResource ensures that a resource exists with the desired state.
func reconcileResource[T any](ctx context.Context, name string, funcs ReconcileFuncs[T]) (*T, error) {
	resource, err := funcs.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource %s: %w", name, err)
	}

	if resource != nil {
		if funcs.Update != nil {
			if funcs.NeedsUpdate == nil || funcs.NeedsUpdate(resource) {
				updatedResource, err := funcs.Update(ctx, resource)
				if err != nil {
					return nil, fmt.Errorf("failed to update resource %s: %w", name, err)
				}
				if updatedResource != nil {
					return updatedResource, nil
				}
			}
		}
		return resource, nil
	}

	resource, err = funcs.Create(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource %s: %w", name, err)
	}

	return resource, nil
}

// DeleteFuncs defines the functions required for generic deletion.
type DeleteFuncs[T any] struct {
	// Get retrieves the resource by name.
	Get func(ctx context.Context, name string) (*T, error)
	// Delete deletes the resource.
	Delete func(ctx context.Context, resource *T) error
}

// deleteResource deletes a resource with retry logic for locked resources.
func deleteResource[T any](ctx context.Context, name string, funcs DeleteFuncs[T], timeouts *Timeouts) error {
	if timeouts == nil {
		timeouts = &Timeouts{
			Delete:            5 * time.Minute,
			RetryMaxAttempts:  5,
			RetryInitialDelay: 1 * time.Second,
		}
	}

	// Add timeout context for delete operation
	ctx, cancel := context.WithTimeout(ctx, timeouts.Delete)
	defer cancel()

	return retry.WithExponentialBackoff(ctx, func() error {
		resource, err := funcs.Get(ctx, name)
		if err != nil {
			return retry.Fatal(fmt.Errorf("failed to get resource %s: %w", name, err))
		}
		// In Go, interface/pointer nil check. *T is compatible with nil.
		// We need to check if the pointer is nil.
		// Comparing generic *T against nil directly works.
		if resource == nil {
			return nil // Already deleted
		}

		err = funcs.Delete(ctx, resource)
		if err != nil {
			if isResourceLocked(err) {
				return err
			}
			return retry.Fatal(err)
		}
		return nil
	}, retry.WithMaxRetries(timeouts.RetryMaxAttempts), retry.WithInitialDelay(timeouts.RetryInitialDelay))
}
