package hcloud

import (
	"context"
	"fmt"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// EnsureFirewall ensures that a firewall exists with the given specifications.
// If applyToLabelSelector is non-empty, the firewall will be applied to all servers
// matching the label selector (e.g., "cluster=my-cluster").
func (c *RealClient) EnsureFirewall(ctx context.Context, name string, rules []hcloud.FirewallRule, labels map[string]string, applyToLabelSelector string) (*hcloud.Firewall, error) {
	// Build ApplyTo resources if label selector is provided
	var applyTo []hcloud.FirewallResource
	if applyToLabelSelector != "" {
		applyTo = []hcloud.FirewallResource{
			{
				Type: hcloud.FirewallResourceTypeLabelSelector,
				LabelSelector: &hcloud.FirewallResourceLabelSelector{
					Selector: applyToLabelSelector,
				},
			},
		}
	}

	fw, err := (&EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, hcloud.FirewallSetRulesOpts]{
		Name:         name,
		ResourceType: "firewall",
		Get:          c.client.Firewall.Get,
		Create:       c.createFirewall,
		Update:       c.client.Firewall.SetRules,
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{
				Name:    name,
				Rules:   rules,
				Labels:  labels,
				ApplyTo: applyTo,
			}
		},
		UpdateOptsMapper: func(_ *hcloud.Firewall) hcloud.FirewallSetRulesOpts {
			return hcloud.FirewallSetRulesOpts{
				Rules: rules,
			}
		},
	}).Execute(ctx, c)
	if err != nil {
		return nil, err
	}

	// For existing firewalls, ensure the label selector is applied
	// (the EnsureOperation only updates rules, not ApplyTo)
	if applyToLabelSelector != "" {
		if err := c.ensureFirewallAppliedTo(ctx, fw, applyToLabelSelector); err != nil {
			return nil, fmt.Errorf("failed to apply firewall to label selector: %w", err)
		}
	}

	return fw, nil
}

func (c *RealClient) createFirewall(ctx context.Context, opts hcloud.FirewallCreateOpts) (*CreateResult[*hcloud.Firewall], *hcloud.Response, error) {
	res, resp, err := c.client.Firewall.Create(ctx, opts)
	if err != nil {
		return nil, resp, err
	}
	return &CreateResult[*hcloud.Firewall]{
		Resource: res.Firewall,
		Actions:  res.Actions,
	}, resp, nil
}

// ensureFirewallAppliedTo ensures the firewall is applied to resources matching the label selector.
// This is idempotent - if the label selector is already applied, it does nothing.
func (c *RealClient) ensureFirewallAppliedTo(ctx context.Context, fw *hcloud.Firewall, labelSelector string) error {
	// Check if the label selector is already applied
	for _, applied := range fw.AppliedTo {
		if applied.Type == hcloud.FirewallResourceTypeLabelSelector &&
			applied.LabelSelector != nil &&
			applied.LabelSelector.Selector == labelSelector {
			// Already applied
			return nil
		}
	}

	// Apply the label selector
	resources := []hcloud.FirewallResource{
		{
			Type: hcloud.FirewallResourceTypeLabelSelector,
			LabelSelector: &hcloud.FirewallResourceLabelSelector{
				Selector: labelSelector,
			},
		},
	}

	actions, _, err := c.client.Firewall.ApplyResources(ctx, fw, resources)
	if err != nil {
		return err
	}

	return c.client.Action.WaitFor(ctx, actions...)
}

// DeleteFirewall deletes the firewall with the given name.
func (c *RealClient) DeleteFirewall(ctx context.Context, name string) error {
	return (&DeleteOperation[*hcloud.Firewall]{
		Name:         name,
		ResourceType: "firewall",
		Get:          c.client.Firewall.Get,
		Delete:       c.client.Firewall.Delete,
	}).Execute(ctx, c)
}

// GetFirewall returns the firewall with the given name.
func (c *RealClient) GetFirewall(ctx context.Context, name string) (*hcloud.Firewall, error) {
	fw, _, err := c.client.Firewall.Get(ctx, name)
	return fw, err
}
