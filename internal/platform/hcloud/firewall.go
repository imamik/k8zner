package hcloud

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// EnsureFirewall ensures that a firewall exists with the given specifications.
func (c *RealClient) EnsureFirewall(ctx context.Context, name string, rules []hcloud.FirewallRule, labels map[string]string) (*hcloud.Firewall, error) {
	return (&EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, hcloud.FirewallSetRulesOpts]{
		Name:         name,
		ResourceType: "firewall",
		Get:          c.client.Firewall.Get,
		Create:       c.createFirewall,
		Update:       c.client.Firewall.SetRules,
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{
				Name:   name,
				Rules:  rules,
				Labels: labels,
			}
		},
		UpdateOptsMapper: func(_ *hcloud.Firewall) hcloud.FirewallSetRulesOpts {
			return hcloud.FirewallSetRulesOpts{
				Rules: rules,
			}
		},
	}).Execute(ctx, c)
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
