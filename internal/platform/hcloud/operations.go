package hcloud

import (
	"context"
	"fmt"
	"reflect"

	"github.com/imamik/k8zner/internal/util/retry"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// CreateResult wraps the result of a resource creation operation.
// It handles both single and multiple actions that may need to be awaited.
type CreateResult[T any] struct {
	Resource T
	Action   *hcloud.Action
	Actions  []*hcloud.Action
}

// DeleteOperation encapsulates deletion logic for any hcloud resource.
// It provides consistent retry, timeout, and error handling across all resource types.
//
// Usage example:
//
//	func (c *RealClient) DeleteFirewall(ctx context.Context, name string) error {
//	    return (&DeleteOperation[*hcloud.Firewall]{
//	        Name:         name,
//	        ResourceType: "firewall",
//	        Get:          c.client.Firewall.Get,
//	        Delete:       c.client.Firewall.Delete,
//	    }).Execute(ctx, c)
//	}
type DeleteOperation[T any] struct {
	Name         string
	ResourceType string

	// Get retrieves the resource by name
	Get func(ctx context.Context, name string) (T, *hcloud.Response, error)

	// Delete removes the resource
	Delete func(ctx context.Context, resource T) (*hcloud.Response, error)
}

// Execute performs the delete operation with retry logic and timeout handling.
// The operation is idempotent - it succeeds if the resource doesn't exist.
// Locked resources are retried with exponential backoff.
func (op *DeleteOperation[T]) Execute(ctx context.Context, client *RealClient) error {
	ctx, cancel := context.WithTimeout(ctx, client.timeouts.Delete)
	defer cancel()

	return retry.WithExponentialBackoff(ctx, func() error {
		resource, _, err := op.Get(ctx, op.Name)
		if err != nil {
			return retry.Fatal(fmt.Errorf("failed to get %s: %w", op.ResourceType, err))
		}

		// Check if resource is nil (already deleted)
		if reflect.ValueOf(resource).IsNil() {
			return nil
		}

		_, err = op.Delete(ctx, resource)
		if err != nil {
			if isResourceLocked(err) {
				return err // Retryable
			}
			return retry.Fatal(err)
		}
		return nil
	},
		retry.WithMaxRetries(client.timeouts.RetryMaxAttempts),
		retry.WithInitialDelay(client.timeouts.RetryInitialDelay))
}

// EnsureOperation encapsulates get-or-create logic for any hcloud resource.
// It supports optional update and validation logic for existing resources.
//
// Usage examples:
//
// Simple ensure (no update):
//
//	func (c *RealClient) EnsurePlacementGroup(ctx context.Context, name, pgType string, labels map[string]string) (*hcloud.PlacementGroup, error) {
//	    return (&EnsureOperation[*hcloud.PlacementGroup, hcloud.PlacementGroupCreateOpts, any]{
//	        Name:         name,
//	        ResourceType: "placement group",
//	        Get:          c.client.PlacementGroup.Get,
//	        Create: func(ctx context.Context, opts hcloud.PlacementGroupCreateOpts) (*CreateResult[*hcloud.PlacementGroup], *hcloud.Response, error) {
//	            res, resp, err := c.client.PlacementGroup.Create(ctx, opts)
//	            if err != nil {
//	                return nil, resp, err
//	            }
//	            return &CreateResult[*hcloud.PlacementGroup]{Resource: res.PlacementGroup}, resp, nil
//	        },
//	        CreateOptsMapper: func() hcloud.PlacementGroupCreateOpts {
//	            return hcloud.PlacementGroupCreateOpts{Name: name, Type: hcloud.PlacementGroupType(pgType), Labels: labels}
//	        },
//	    }).Execute(ctx, c)
//	}
//
// Ensure with validation:
//
//	EnsureOperation{
//	    // ... other fields
//	    Validate: func(network *hcloud.Network) error {
//	        if network.IPRange.String() != ipRange {
//	            return fmt.Errorf("network exists with different IP range")
//	        }
//	        return nil
//	    },
//	}
//
// Ensure with update:
//
//	EnsureOperation{
//	    // ... other fields
//	    Update: func(ctx context.Context, fw *hcloud.Firewall, opts hcloud.FirewallSetRulesOpts) ([]*hcloud.Action, *hcloud.Response, error) {
//	        return c.client.Firewall.SetRules(ctx, fw, opts)
//	    },
//	    UpdateOptsMapper: func(fw *hcloud.Firewall) hcloud.FirewallSetRulesOpts {
//	        return hcloud.FirewallSetRulesOpts{Rules: rules}
//	    },
//	}
type EnsureOperation[T any, CreateOpts any, UpdateOpts any] struct {
	Name         string
	ResourceType string

	// Get retrieves the resource by name
	Get func(ctx context.Context, name string) (T, *hcloud.Response, error)

	// Create creates the resource with the given options
	Create func(ctx context.Context, opts CreateOpts) (*CreateResult[T], *hcloud.Response, error)

	// Update updates the resource if it exists (optional)
	Update func(ctx context.Context, resource T, opts UpdateOpts) ([]*hcloud.Action, *hcloud.Response, error)

	// Validate checks if existing resource matches desired state (optional)
	Validate func(resource T) error

	// CreateOptsMapper maps input parameters to create options
	CreateOptsMapper func() CreateOpts

	// UpdateOptsMapper maps input parameters to update options (required if Update is provided)
	UpdateOptsMapper func(resource T) UpdateOpts
}

// Execute performs the ensure operation: get existing resource, update/validate if needed, or create new.
func (op *EnsureOperation[T, CreateOpts, UpdateOpts]) Execute(
	ctx context.Context,
	client *RealClient,
) (T, error) {
	var zero T

	// Try to get existing resource
	resource, _, err := op.Get(ctx, op.Name)
	if err != nil {
		return zero, fmt.Errorf("failed to get %s: %w", op.ResourceType, err)
	}

	// Resource exists
	if !reflect.ValueOf(resource).IsNil() {
		// Validate if validator provided
		if op.Validate != nil {
			if err := op.Validate(resource); err != nil {
				return zero, err
			}
		}

		// Update if updater provided
		if op.Update != nil && op.UpdateOptsMapper != nil {
			updateOpts := op.UpdateOptsMapper(resource)
			actions, _, err := op.Update(ctx, resource, updateOpts)
			if err != nil {
				return zero, fmt.Errorf("failed to update %s: %w", op.ResourceType, err)
			}
			if err := waitForActions(ctx, client.client, actions...); err != nil {
				return zero, fmt.Errorf("failed to wait for %s update: %w", op.ResourceType, err)
			}
		}

		return resource, nil
	}

	// Create new resource
	createOpts := op.CreateOptsMapper()
	result, _, err := op.Create(ctx, createOpts)
	if err != nil {
		return zero, fmt.Errorf("failed to create %s: %w", op.ResourceType, err)
	}

	// Wait for creation actions
	if err := waitForActionResult(ctx, client.client, result); err != nil {
		return zero, fmt.Errorf("failed to wait for %s creation: %w", op.ResourceType, err)
	}

	return result.Resource, nil
}

// waitForActions waits for one or more actions to complete.
// Handles both single actions and multiple actions uniformly.
func waitForActions(ctx context.Context, client *hcloud.Client, actions ...*hcloud.Action) error {
	if len(actions) == 0 {
		return nil
	}
	return client.Action.WaitFor(ctx, actions...)
}

// waitForActionResult waits for actions from a CreateResult.
// Handles both singular Action and plural Actions fields.
func waitForActionResult[T any](ctx context.Context, client *hcloud.Client, result *CreateResult[T]) error {
	if result.Action != nil {
		return client.Action.WaitFor(ctx, result.Action)
	}
	if len(result.Actions) > 0 {
		return client.Action.WaitFor(ctx, result.Actions...)
	}
	return nil
}

// Helper functions to reduce boilerplate when wrapping hcloud Create calls

// simpleCreate wraps create functions returning the resource directly.
// Use for: Certificate, Network (return Resource, Response, error)
func simpleCreate[T any, Opts any](
	createFn func(context.Context, Opts) (T, *hcloud.Response, error),
) func(context.Context, Opts) (*CreateResult[T], *hcloud.Response, error) {
	return func(ctx context.Context, opts Opts) (*CreateResult[T], *hcloud.Response, error) {
		resource, resp, err := createFn(ctx, opts)
		if err != nil {
			return nil, resp, err
		}
		return &CreateResult[T]{Resource: resource}, resp, nil
	}
}
