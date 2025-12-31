package mocks

import (
	"context"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type MockNetworkClient struct {
	GetByIDFunc   func(ctx context.Context, id int64) (*hcloud.Network, *hcloud.Response, error)
	GetByNameFunc func(ctx context.Context, name string) (*hcloud.Network, *hcloud.Response, error)
	CreateFunc    func(ctx context.Context, opts hcloud.NetworkCreateOpts) (*hcloud.Network, *hcloud.Response, error)
	DeleteFunc    func(ctx context.Context, network *hcloud.Network) (*hcloud.Response, error)
}

func (m *MockNetworkClient) GetByID(ctx context.Context, id int64) (*hcloud.Network, *hcloud.Response, error) {
	return m.GetByIDFunc(ctx, id)
}

func (m *MockNetworkClient) GetByName(ctx context.Context, name string) (*hcloud.Network, *hcloud.Response, error) {
	return m.GetByNameFunc(ctx, name)
}

func (m *MockNetworkClient) Create(ctx context.Context, opts hcloud.NetworkCreateOpts) (*hcloud.Network, *hcloud.Response, error) {
	return m.CreateFunc(ctx, opts)
}

func (m *MockNetworkClient) Delete(ctx context.Context, network *hcloud.Network) (*hcloud.Response, error) {
	return m.DeleteFunc(ctx, network)
}

type MockFirewallClient struct {
    GetByNameFunc func(ctx context.Context, name string) (*hcloud.Firewall, *hcloud.Response, error)
    GetByIDFunc   func(ctx context.Context, id int64) (*hcloud.Firewall, *hcloud.Response, error)
    CreateFunc    func(ctx context.Context, opts hcloud.FirewallCreateOpts) (hcloud.FirewallCreateResult, *hcloud.Response, error)
    DeleteFunc    func(ctx context.Context, firewall *hcloud.Firewall) (*hcloud.Response, error)
}

func (m *MockFirewallClient) GetByName(ctx context.Context, name string) (*hcloud.Firewall, *hcloud.Response, error) {
    return m.GetByNameFunc(ctx, name)
}
func (m *MockFirewallClient) GetByID(ctx context.Context, id int64) (*hcloud.Firewall, *hcloud.Response, error) {
    return m.GetByIDFunc(ctx, id)
}
func (m *MockFirewallClient) Create(ctx context.Context, opts hcloud.FirewallCreateOpts) (hcloud.FirewallCreateResult, *hcloud.Response, error) {
    return m.CreateFunc(ctx, opts)
}
func (m *MockFirewallClient) Delete(ctx context.Context, firewall *hcloud.Firewall) (*hcloud.Response, error) {
    return m.DeleteFunc(ctx, firewall)
}

type MockActionClient struct {
    GetByIDFunc func(ctx context.Context, id int64) (*hcloud.Action, *hcloud.Response, error)
}

func (m *MockActionClient) GetByID(ctx context.Context, id int64) (*hcloud.Action, *hcloud.Response, error) {
    return m.GetByIDFunc(ctx, id)
}

type MockLoadBalancerClient struct {
    GetByNameFunc func(ctx context.Context, name string) (*hcloud.LoadBalancer, *hcloud.Response, error)
    CreateFunc    func(ctx context.Context, opts hcloud.LoadBalancerCreateOpts) (hcloud.LoadBalancerCreateResult, *hcloud.Response, error)
    AddServiceFunc func(ctx context.Context, lb *hcloud.LoadBalancer, opts hcloud.LoadBalancerAddServiceOpts) (*hcloud.Action, *hcloud.Response, error)
    AddServerTargetFunc func(ctx context.Context, lb *hcloud.LoadBalancer, opts hcloud.LoadBalancerAddServerTargetOpts) (*hcloud.Action, *hcloud.Response, error)
}

func (m *MockLoadBalancerClient) GetByName(ctx context.Context, name string) (*hcloud.LoadBalancer, *hcloud.Response, error) {
    return m.GetByNameFunc(ctx, name)
}
func (m *MockLoadBalancerClient) Create(ctx context.Context, opts hcloud.LoadBalancerCreateOpts) (hcloud.LoadBalancerCreateResult, *hcloud.Response, error) {
    return m.CreateFunc(ctx, opts)
}
func (m *MockLoadBalancerClient) AddService(ctx context.Context, lb *hcloud.LoadBalancer, opts hcloud.LoadBalancerAddServiceOpts) (*hcloud.Action, *hcloud.Response, error) {
    return m.AddServiceFunc(ctx, lb, opts)
}
func (m *MockLoadBalancerClient) AddServerTarget(ctx context.Context, lb *hcloud.LoadBalancer, opts hcloud.LoadBalancerAddServerTargetOpts) (*hcloud.Action, *hcloud.Response, error) {
    return m.AddServerTargetFunc(ctx, lb, opts)
}
