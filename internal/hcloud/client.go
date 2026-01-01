package hcloud

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// Client defines the interface for Hetzner Cloud interactions.
// This interface allows for mocking in tests.
type Client interface {
	Network() NetworkClient
	Firewall() FirewallClient
}

type NetworkClient interface {
	Get(ctx context.Context, nameOrID string) (*hcloud.Network, *hcloud.Response, error)
	Create(ctx context.Context, opts hcloud.NetworkCreateOpts) (*hcloud.Network, *hcloud.Response, error)
	Delete(ctx context.Context, network *hcloud.Network) (*hcloud.Response, error)
}

type FirewallClient interface {
	Get(ctx context.Context, nameOrID string) (*hcloud.Firewall, *hcloud.Response, error)
	Create(ctx context.Context, opts hcloud.FirewallCreateOpts) (hcloud.FirewallCreateResult, *hcloud.Response, error)
	Update(ctx context.Context, firewall *hcloud.Firewall, opts hcloud.FirewallUpdateOpts) (*hcloud.Firewall, *hcloud.Response, error)
	SetRules(ctx context.Context, firewall *hcloud.Firewall, opts hcloud.FirewallSetRulesOpts) ([]*hcloud.Action, *hcloud.Response, error)
	Delete(ctx context.Context, firewall *hcloud.Firewall) (*hcloud.Response, error)
}

// RealClient implements Client using the official hcloud-go library.
type RealClient struct {
	client *hcloud.Client
}

func NewClient(token string) *RealClient {
	return &RealClient{
		client: hcloud.NewClient(hcloud.WithToken(token)),
	}
}

func (c *RealClient) Network() NetworkClient {
	return &c.client.Network
}

func (c *RealClient) Firewall() FirewallClient {
	return &c.client.Firewall
}
