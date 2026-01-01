package infra

import (
	"context"
	"testing"

	"github.com/hcloud-k8s/internal/config"
	"github.com/hcloud-k8s/internal/hcloud"
	hcloudlib "github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func TestEnsureNetwork_CreatesNew(t *testing.T) {
	mockClient := &hcloud.MockClient{}
	mockNetwork := &hcloud.MockNetworkClient{}
	mockClient.NetworkClient = mockNetwork

	// Mock Get to return nil (network doesn't exist)
	mockNetwork.GetFunc = func(ctx context.Context, nameOrID string) (*hcloudlib.Network, *hcloudlib.Response, error) {
		return nil, nil, nil
	}

	// Mock Create
	createCalled := false
	mockNetwork.CreateFunc = func(ctx context.Context, opts hcloudlib.NetworkCreateOpts) (*hcloudlib.Network, *hcloudlib.Response, error) {
		createCalled = true
		if opts.Name != "test-cluster" {
			t.Errorf("expected name 'test-cluster', got %s", opts.Name)
		}
		return &hcloudlib.Network{Name: opts.Name}, nil, nil
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Hetzner: config.Hetzner{
			NetworkZone: "eu-central",
		},
	}

	mgr := NewManager(mockClient, cfg)
	if err := mgr.EnsureNetwork(context.Background()); err != nil {
		t.Fatalf("EnsureNetwork() error = %v", err)
	}

	if !createCalled {
		t.Error("expected Create to be called")
	}
}

func TestEnsureFirewall_UpdatesExisting(t *testing.T) {
	mockClient := &hcloud.MockClient{}
	mockFirewall := &hcloud.MockFirewallClient{}
	mockClient.FirewallClient = mockFirewall

	// Mock Get to return existing firewall
	mockFirewall.GetFunc = func(ctx context.Context, nameOrID string) (*hcloudlib.Firewall, *hcloudlib.Response, error) {
		return &hcloudlib.Firewall{ID: 123, Name: "test-cluster-firewall"}, nil, nil
	}

	// Mock SetRules
	setRulesCalled := false
	mockFirewall.SetRulesFunc = func(ctx context.Context, firewall *hcloudlib.Firewall, opts hcloudlib.FirewallSetRulesOpts) ([]*hcloudlib.Action, *hcloudlib.Response, error) {
		setRulesCalled = true
		if firewall.ID != 123 {
			t.Errorf("expected firewall ID 123, got %d", firewall.ID)
		}
		if len(opts.Rules) != 2 {
			t.Errorf("expected 2 rules, got %d", len(opts.Rules))
		}
		return nil, nil, nil
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Hetzner: config.Hetzner{
			Firewall: config.Firewall{
				APISource: []string{"1.2.3.4/32"},
			},
		},
	}

	mgr := NewManager(mockClient, cfg)
	if err := mgr.EnsureFirewall(context.Background()); err != nil {
		t.Fatalf("EnsureFirewall() error = %v", err)
	}

	if !setRulesCalled {
		t.Error("expected SetRules to be called")
	}
}
