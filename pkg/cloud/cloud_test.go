package cloud

import (
	"context"
	"testing"

    "github.com/hcloud-k8s/hcloud-k8s/pkg/config"
    "github.com/hcloud-k8s/hcloud-k8s/pkg/cloud/mocks"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func TestEnsureNetwork_CreatesNetwork(t *testing.T) {
    cfg := &config.ClusterConfig{
        ClusterName: "test-cluster",
        NetworkIPv4CIDR: "10.100.0.0/16",
    }

    mockNetwork := &mocks.MockNetworkClient{
        GetByNameFunc: func(ctx context.Context, name string) (*hcloud.Network, *hcloud.Response, error) {
            return nil, nil, nil // Not found
        },
        CreateFunc: func(ctx context.Context, opts hcloud.NetworkCreateOpts) (*hcloud.Network, *hcloud.Response, error) {
            if opts.Name != "test-cluster" {
                t.Errorf("Expected name 'test-cluster', got '%s'", opts.Name)
            }
            if opts.IPRange.String() != "10.100.0.0/16" {
                 t.Errorf("Expected CIDR '10.100.0.0/16', got '%s'", opts.IPRange.String())
            }
            return &hcloud.Network{ID: 1, Name: opts.Name}, nil, nil
        },
    }

    c := &Cloud{
        Config: cfg,
        Network: mockNetwork,
    }

    netw, err := c.EnsureNetwork(context.Background())
    if err != nil {
        t.Fatalf("EnsureNetwork failed: %v", err)
    }

    if netw.ID != 1 {
        t.Errorf("Expected Network ID 1, got %d", netw.ID)
    }
}

func TestEnsureFirewall_CreatesFirewall(t *testing.T) {
    cfg := &config.ClusterConfig{
        ClusterName: "test-cluster",
        Firewall: config.FirewallConfig{
            ApiAllowList: []string{"1.2.3.4/32"},
        },
    }

    mockFirewall := &mocks.MockFirewallClient{
        GetByNameFunc: func(ctx context.Context, name string) (*hcloud.Firewall, *hcloud.Response, error) {
            return nil, nil, nil
        },
        CreateFunc: func(ctx context.Context, opts hcloud.FirewallCreateOpts) (hcloud.FirewallCreateResult, *hcloud.Response, error) {
            if opts.Name != "test-cluster" {
                t.Errorf("Expected name 'test-cluster', got '%s'", opts.Name)
            }
            // Verify allow list rule
            found := false
            for _, rule := range opts.Rules {
                if rule.Port != nil && *rule.Port == "6443" {
                    if len(rule.SourceIPs) > 0 && rule.SourceIPs[0].String() == "1.2.3.4/32" {
                        found = true
                    }
                }
            }
            if !found {
                t.Error("Expected firewall rule allowing 1.2.3.4/32 on port 6443")
            }

            return hcloud.FirewallCreateResult{Firewall: &hcloud.Firewall{ID: 1, Name: opts.Name}}, nil, nil
        },
    }

    c := &Cloud{
        Config: cfg,
        Firewall: mockFirewall,
    }

    fw, err := c.EnsureFirewall(context.Background())
    if err != nil {
        t.Fatalf("EnsureFirewall failed: %v", err)
    }
    if fw.ID != 1 {
        t.Errorf("Expected ID 1, got %d", fw.ID)
    }
}

func TestCreateLoadBalancer(t *testing.T) {
    mockLB := &mocks.MockLoadBalancerClient{
        GetByNameFunc: func(ctx context.Context, name string) (*hcloud.LoadBalancer, *hcloud.Response, error) {
            return nil, nil, nil
        },
        CreateFunc: func(ctx context.Context, opts hcloud.LoadBalancerCreateOpts) (hcloud.LoadBalancerCreateResult, *hcloud.Response, error) {
            return hcloud.LoadBalancerCreateResult{
                LoadBalancer: &hcloud.LoadBalancer{ID: 1, Name: opts.Name},
                Action: &hcloud.Action{ID: 100, Status: hcloud.ActionStatusSuccess},
            }, nil, nil
        },
        AddServiceFunc: func(ctx context.Context, lb *hcloud.LoadBalancer, opts hcloud.LoadBalancerAddServiceOpts) (*hcloud.Action, *hcloud.Response, error) {
             return &hcloud.Action{ID: 101, Status: hcloud.ActionStatusSuccess}, nil, nil
        },
    }

    mockAction := &mocks.MockActionClient{
        GetByIDFunc: func(ctx context.Context, id int64) (*hcloud.Action, *hcloud.Response, error) {
            return &hcloud.Action{ID: id, Status: hcloud.ActionStatusSuccess}, nil, nil
        },
    }

    c := &Cloud{
        LoadBalancer: mockLB,
        Action: mockAction,
    }

    lb, err := c.CreateLoadBalancer(context.Background(), "test-lb", "nbg1", nil)
    if err != nil {
        t.Fatalf("CreateLoadBalancer failed: %v", err)
    }
    if lb.ID != 1 {
        t.Errorf("Expected LB ID 1, got %d", lb.ID)
    }
}
