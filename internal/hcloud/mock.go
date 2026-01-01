package hcloud

import (
	"context"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// MockClient is a mock implementation of Client.
type MockClient struct {
	NetworkClient  *MockNetworkClient
	FirewallClient *MockFirewallClient
}

func (m *MockClient) Network() NetworkClient {
	return m.NetworkClient
}

func (m *MockClient) Firewall() FirewallClient {
	return m.FirewallClient
}

type MockNetworkClient struct {
	GetFunc    func(ctx context.Context, nameOrID string) (*hcloud.Network, *hcloud.Response, error)
	CreateFunc func(ctx context.Context, opts hcloud.NetworkCreateOpts) (*hcloud.Network, *hcloud.Response, error)
	DeleteFunc func(ctx context.Context, network *hcloud.Network) (*hcloud.Response, error)
}

func (m *MockNetworkClient) Get(ctx context.Context, nameOrID string) (*hcloud.Network, *hcloud.Response, error) {
	return m.GetFunc(ctx, nameOrID)
}

func (m *MockNetworkClient) Create(ctx context.Context, opts hcloud.NetworkCreateOpts) (*hcloud.Network, *hcloud.Response, error) {
	return m.CreateFunc(ctx, opts)
}

func (m *MockNetworkClient) Delete(ctx context.Context, network *hcloud.Network) (*hcloud.Response, error) {
	return m.DeleteFunc(ctx, network)
}

type MockFirewallClient struct {
	GetFunc      func(ctx context.Context, nameOrID string) (*hcloud.Firewall, *hcloud.Response, error)
	CreateFunc   func(ctx context.Context, opts hcloud.FirewallCreateOpts) (hcloud.FirewallCreateResult, *hcloud.Response, error)
	UpdateFunc   func(ctx context.Context, firewall *hcloud.Firewall, opts hcloud.FirewallUpdateOpts) (*hcloud.Firewall, *hcloud.Response, error)
	SetRulesFunc func(ctx context.Context, firewall *hcloud.Firewall, opts hcloud.FirewallSetRulesOpts) ([]*hcloud.Action, *hcloud.Response, error)
	DeleteFunc   func(ctx context.Context, firewall *hcloud.Firewall) (*hcloud.Response, error)
}

func (m *MockFirewallClient) Get(ctx context.Context, nameOrID string) (*hcloud.Firewall, *hcloud.Response, error) {
	return m.GetFunc(ctx, nameOrID)
}

func (m *MockFirewallClient) Create(ctx context.Context, opts hcloud.FirewallCreateOpts) (hcloud.FirewallCreateResult, *hcloud.Response, error) {
	return m.CreateFunc(ctx, opts)
}

func (m *MockFirewallClient) Update(ctx context.Context, firewall *hcloud.Firewall, opts hcloud.FirewallUpdateOpts) (*hcloud.Firewall, *hcloud.Response, error) {
	return m.UpdateFunc(ctx, firewall, opts)
}

func (m *MockFirewallClient) SetRules(ctx context.Context, firewall *hcloud.Firewall, opts hcloud.FirewallSetRulesOpts) ([]*hcloud.Action, *hcloud.Response, error) {
	return m.SetRulesFunc(ctx, firewall, opts)
}

func (m *MockFirewallClient) Delete(ctx context.Context, firewall *hcloud.Firewall) (*hcloud.Response, error) {
	return m.DeleteFunc(ctx, firewall)
}
