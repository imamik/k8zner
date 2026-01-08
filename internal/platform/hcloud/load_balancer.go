package hcloud

import (
	"context"
	"fmt"
	"net"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"hcloud-k8s/internal/util/retry"
)

// EnsureLoadBalancer ensures that a load balancer exists with the given specifications.
// Note: Load balancer creation can take 1-6 minutes depending on Hetzner Cloud backend load.
// This is normal Hetzner Cloud API behavior, not a bug in this code.
func (c *RealClient) EnsureLoadBalancer(ctx context.Context, name, location, lbType string, algorithm hcloud.LoadBalancerAlgorithmType, labels map[string]string) (*hcloud.LoadBalancer, error) {
	lb, _, err := c.client.LoadBalancer.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get lb: %w", err)
	}

	if lb != nil {
		// Check if updates needed (omitted for brevity, can implement update logic)
		return lb, nil
	}

	// Create
	lbTypeObj, _, err := c.client.LoadBalancerType.Get(ctx, lbType)
	if err != nil {
		return nil, fmt.Errorf("failed to get lb type: %w", err)
	}
	locObj, _, err := c.client.Location.Get(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("failed to get location: %w", err)
	}

	opts := hcloud.LoadBalancerCreateOpts{
		Name:             name,
		LoadBalancerType: lbTypeObj,
		Location:         locObj,
		Algorithm:        &hcloud.LoadBalancerAlgorithm{Type: algorithm},
		Labels:           labels,
	}

	res, _, err := c.client.LoadBalancer.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create lb: %w", err)
	}
	if err := c.client.Action.WaitFor(ctx, res.Action); err != nil {
		return nil, fmt.Errorf("failed to wait for lb creation: %w", err)
	}

	return res.LoadBalancer, nil
}

// ConfigureService configures a service on the load balancer.
func (c *RealClient) ConfigureService(ctx context.Context, lb *hcloud.LoadBalancer, service hcloud.LoadBalancerAddServiceOpts) error {
	// Check if service exists
	if service.ListenPort == nil {
		return fmt.Errorf("listen port is nil")
	}

	for _, s := range lb.Services {
		if s.ListenPort == *service.ListenPort {
			// Update? For now we assume idempotency means "ensure it matches".
			return nil
		}
	}

	action, _, err := c.client.LoadBalancer.AddService(ctx, lb, service)
	if err != nil {
		return fmt.Errorf("failed to add service: %w", err)
	}
	return c.client.Action.WaitFor(ctx, action)
}

// AddTarget adds a target to the load balancer.
func (c *RealClient) AddTarget(ctx context.Context, lb *hcloud.LoadBalancer, targetType hcloud.LoadBalancerTargetType, labelSelector string) error {
	// Check if target exists
	for _, target := range lb.Targets {
		if target.Type == targetType && target.LabelSelector != nil && target.LabelSelector.Selector == labelSelector {
			return nil // Already exists
		}
	}

	if targetType == hcloud.LoadBalancerTargetTypeLabelSelector {
		opts := hcloud.LoadBalancerAddLabelSelectorTargetOpts{
			Selector:     labelSelector,
			UsePrivateIP: hcloud.Ptr(true),
		}
		action, _, err := c.client.LoadBalancer.AddLabelSelectorTarget(ctx, lb, opts)
		if err != nil {
			return fmt.Errorf("failed to add target: %w", err)
		}
		return c.client.Action.WaitFor(ctx, action)
	}

	return fmt.Errorf("unsupported target type: %s", targetType)
}

// AttachToNetwork attaches the load balancer to a network.
func (c *RealClient) AttachToNetwork(ctx context.Context, lb *hcloud.LoadBalancer, network *hcloud.Network, ip net.IP) error {
	// Check if already attached
	for _, privateNet := range lb.PrivateNet {
		if privateNet.Network.ID == network.ID {
			return nil
		}
	}

	opts := hcloud.LoadBalancerAttachToNetworkOpts{
		Network: network,
		IP:      ip,
	}
	action, _, err := c.client.LoadBalancer.AttachToNetwork(ctx, lb, opts)
	if err != nil {
		return fmt.Errorf("failed to attach lb to network: %w", err)
	}
	return c.client.Action.WaitFor(ctx, action)
}

// DeleteLoadBalancer deletes the load balancer with the given name.
func (c *RealClient) DeleteLoadBalancer(ctx context.Context, name string) error {
	// Add timeout context for delete operation
	ctx, cancel := context.WithTimeout(ctx, c.timeouts.Delete)
	defer cancel()

	// Delete with retry logic (resource might be locked)
	return retry.WithExponentialBackoff(ctx, func() error {
		lb, _, err := c.client.LoadBalancer.Get(ctx, name)
		if err != nil {
			return retry.Fatal(fmt.Errorf("failed to get load balancer: %w", err))
		}
		if lb == nil {
			return nil // Load balancer already deleted
		}

		_, err = c.client.LoadBalancer.Delete(ctx, lb)
		if err != nil {
			// Check if resource is locked (retryable)
			if isResourceLocked(err) {
				return err
			}
			// Other errors are fatal
			return retry.Fatal(err)
		}
		return nil
	}, retry.WithMaxRetries(c.timeouts.RetryMaxAttempts), retry.WithInitialDelay(c.timeouts.RetryInitialDelay))
}

// GetLoadBalancer returns the load balancer with the given name.
func (c *RealClient) GetLoadBalancer(ctx context.Context, name string) (*hcloud.LoadBalancer, error) {
	lb, _, err := c.client.LoadBalancer.Get(ctx, name)
	return lb, err
}
