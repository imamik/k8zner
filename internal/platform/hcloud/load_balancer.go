package hcloud

import (
	"context"
	"fmt"
	"net"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// loadBalancerCreateParams holds parameters for creating a load balancer.
type loadBalancerCreateParams struct {
	name      string
	location  string
	lbType    string
	algorithm hcloud.LoadBalancerAlgorithmType
	labels    map[string]string
}

// EnsureLoadBalancer ensures that a load balancer exists with the given specifications.
// Note: Load balancer creation can take 1-6 minutes depending on Hetzner Cloud backend load.
// This is normal Hetzner Cloud API behavior, not a bug in this code.
func (c *RealClient) EnsureLoadBalancer(ctx context.Context, name, location, lbType string, algorithm hcloud.LoadBalancerAlgorithmType, labels map[string]string) (*hcloud.LoadBalancer, error) {
	params := loadBalancerCreateParams{
		name:      name,
		location:  location,
		lbType:    lbType,
		algorithm: algorithm,
		labels:    labels,
	}

	return (&EnsureOperation[*hcloud.LoadBalancer, loadBalancerCreateParams, any]{
		Name:         name,
		ResourceType: "load balancer",
		Get:          c.client.LoadBalancer.Get,
		Create:       c.createLoadBalancerWithDeps,
		CreateOptsMapper: func() loadBalancerCreateParams {
			return params
		},
	}).Execute(ctx, c)
}

// createLoadBalancerWithDeps resolves dependencies and creates a load balancer.
func (c *RealClient) createLoadBalancerWithDeps(ctx context.Context, params loadBalancerCreateParams) (*CreateResult[*hcloud.LoadBalancer], *hcloud.Response, error) {
	// Resolve load balancer type dependency (only when creating)
	lbTypeObj, _, err := c.client.LoadBalancerType.Get(ctx, params.lbType)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get lb type: %w", err)
	}

	// Resolve location dependency (only when creating)
	locObj, _, err := c.client.Location.Get(ctx, params.location)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get location: %w", err)
	}

	// Build final opts with resolved dependencies
	opts := hcloud.LoadBalancerCreateOpts{
		Name:             params.name,
		LoadBalancerType: lbTypeObj,
		Location:         locObj,
		Algorithm:        &hcloud.LoadBalancerAlgorithm{Type: params.algorithm},
		Labels:           params.labels,
	}

	// Create the load balancer
	res, resp, err := c.client.LoadBalancer.Create(ctx, opts)
	if err != nil {
		return nil, resp, err
	}
	return &CreateResult[*hcloud.LoadBalancer]{
		Resource: res.LoadBalancer,
		Action:   res.Action,
	}, resp, nil
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
	// Validate required parameters
	if ip == nil {
		return fmt.Errorf("ip parameter is required for network attachment")
	}

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
	return (&DeleteOperation[*hcloud.LoadBalancer]{
		Name:         name,
		ResourceType: "load balancer",
		Get:          c.client.LoadBalancer.Get,
		Delete:       c.client.LoadBalancer.Delete,
	}).Execute(ctx, c)
}

// GetLoadBalancer returns the load balancer with the given name.
func (c *RealClient) GetLoadBalancer(ctx context.Context, name string) (*hcloud.LoadBalancer, error) {
	lb, _, err := c.client.LoadBalancer.Get(ctx, name)
	return lb, err
}
