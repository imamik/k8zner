package hcloud

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// TestMockClient_InterfaceCompliance verifies MockClient implements InfrastructureManager.
func TestMockClient_InterfaceCompliance(_ *testing.T) {
	var _ InfrastructureManager = (*MockClient)(nil)
}

func TestMockClient_CreateServer_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	id, err := m.CreateServer(ctx, "test", "image", "type", "loc", nil, nil, "", nil, 1, "", true, true)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if id != "mock-id" {
		t.Errorf("expected 'mock-id', got %q", id)
	}
}

func TestMockClient_CreateServer_CustomFunc(t *testing.T) {
	expectedErr := errors.New("custom error")
	m := &MockClient{
		CreateServerFunc: func(_ context.Context, name, _, _, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string, _, _ bool) (string, error) {
			if name != "test-server" {
				t.Errorf("expected name 'test-server', got %q", name)
			}
			return "", expectedErr
		},
	}
	ctx := context.Background()

	_, err := m.CreateServer(ctx, "test-server", "image", "type", "loc", nil, nil, "", nil, 1, "", true, true)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestMockClient_DeleteServer_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.DeleteServer(ctx, "test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_GetServerIP_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	ip, err := m.GetServerIP(ctx, "test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ip != "127.0.0.1" {
		t.Errorf("expected '127.0.0.1', got %q", ip)
	}
}

func TestMockClient_GetServerID_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	id, err := m.GetServerID(ctx, "test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if id != "123" {
		t.Errorf("expected '123', got %q", id)
	}
}

func TestMockClient_GetServersByLabel_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	servers, err := m.GetServersByLabel(ctx, map[string]string{"key": "value"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected empty slice, got %d servers", len(servers))
	}
}

func TestMockClient_EnableRescue_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	password, err := m.EnableRescue(ctx, "123", []string{"key1"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if password != "mock-password" {
		t.Errorf("expected 'mock-password', got %q", password)
	}
}

func TestMockClient_ResetServer_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.ResetServer(ctx, "123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_PoweroffServer_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.PoweroffServer(ctx, "123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_CreateSnapshot_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	id, err := m.CreateSnapshot(ctx, "123", "test snapshot", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if id != "mock-snapshot-id" {
		t.Errorf("expected 'mock-snapshot-id', got %q", id)
	}
}

func TestMockClient_DeleteImage_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.DeleteImage(ctx, "123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_GetSnapshotByLabels_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	image, err := m.GetSnapshotByLabels(ctx, map[string]string{"key": "value"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if image != nil {
		t.Errorf("expected nil, got %v", image)
	}
}

func TestMockClient_CreateSSHKey_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	id, err := m.CreateSSHKey(ctx, "test-key", "ssh-rsa ...", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if id != "mock-key-id" {
		t.Errorf("expected 'mock-key-id', got %q", id)
	}
}

func TestMockClient_DeleteSSHKey_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.DeleteSSHKey(ctx, "test-key")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_EnsureNetwork_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	network, err := m.EnsureNetwork(ctx, "test-network", "10.0.0.0/8", "eu-central", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if network == nil {
		t.Fatal("expected network, got nil")
	}
	if network.ID != 1 { //nolint:staticcheck // t.Fatal above ensures network is not nil
		t.Errorf("expected ID 1, got %d", network.ID)
	}
}

func TestMockClient_EnsureSubnet_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.EnsureSubnet(ctx, &hcloud.Network{ID: 1}, "10.0.0.0/24", "eu-central", hcloud.NetworkSubnetTypeCloud)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_DeleteNetwork_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.DeleteNetwork(ctx, "test-network")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_GetNetwork_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	network, err := m.GetNetwork(ctx, "test-network")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if network != nil {
		t.Errorf("expected nil, got %v", network)
	}
}

func TestMockClient_EnsureFirewall_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	firewall, err := m.EnsureFirewall(ctx, "test-fw", nil, nil, "")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if firewall == nil {
		t.Fatal("expected firewall, got nil")
	}
	if firewall.ID != 1 { //nolint:staticcheck // t.Fatal above ensures firewall is not nil
		t.Errorf("expected ID 1, got %d", firewall.ID)
	}
}

func TestMockClient_DeleteFirewall_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.DeleteFirewall(ctx, "test-fw")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_GetFirewall_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	firewall, err := m.GetFirewall(ctx, "test-fw")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if firewall != nil {
		t.Errorf("expected nil, got %v", firewall)
	}
}

func TestMockClient_EnsureLoadBalancer_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	lb, err := m.EnsureLoadBalancer(ctx, "test-lb", "fsn1", "lb11", hcloud.LoadBalancerAlgorithmTypeRoundRobin, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if lb == nil {
		t.Fatal("expected load balancer, got nil")
	}
	if lb.ID != 1 { //nolint:staticcheck // t.Fatal above ensures lb is not nil
		t.Errorf("expected ID 1, got %d", lb.ID)
	}
}

func TestMockClient_ConfigureService_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.ConfigureService(ctx, &hcloud.LoadBalancer{ID: 1}, hcloud.LoadBalancerAddServiceOpts{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_AddTarget_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.AddTarget(ctx, &hcloud.LoadBalancer{ID: 1}, hcloud.LoadBalancerTargetTypeServer, "server1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_AttachToNetwork_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.AttachToNetwork(ctx, &hcloud.LoadBalancer{ID: 1}, &hcloud.Network{ID: 1}, net.ParseIP("10.0.0.5"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_DeleteLoadBalancer_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.DeleteLoadBalancer(ctx, "test-lb")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_GetLoadBalancer_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	lb, err := m.GetLoadBalancer(ctx, "test-lb")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if lb != nil {
		t.Errorf("expected nil, got %v", lb)
	}
}

func TestMockClient_EnsurePlacementGroup_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	pg, err := m.EnsurePlacementGroup(ctx, "test-pg", "spread", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if pg == nil {
		t.Fatal("expected placement group, got nil")
	}
	if pg.ID != 1 { //nolint:staticcheck // t.Fatal above ensures pg is not nil
		t.Errorf("expected ID 1, got %d", pg.ID)
	}
}

func TestMockClient_DeletePlacementGroup_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.DeletePlacementGroup(ctx, "test-pg")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_GetPlacementGroup_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	pg, err := m.GetPlacementGroup(ctx, "test-pg")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if pg != nil {
		t.Errorf("expected nil, got %v", pg)
	}
}

func TestMockClient_EnsureFloatingIP_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	fip, err := m.EnsureFloatingIP(ctx, "test-fip", "fsn1", "ipv4", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if fip == nil {
		t.Fatal("expected floating IP, got nil")
	}
	if fip.ID != 1 { //nolint:staticcheck // t.Fatal above ensures fip is not nil
		t.Errorf("expected ID 1, got %d", fip.ID)
	}
}

func TestMockClient_DeleteFloatingIP_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.DeleteFloatingIP(ctx, "test-fip")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_GetFloatingIP_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	fip, err := m.GetFloatingIP(ctx, "test-fip")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if fip != nil {
		t.Errorf("expected nil, got %v", fip)
	}
}

func TestMockClient_EnsureCertificate_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	cert, err := m.EnsureCertificate(ctx, "test-cert", "cert-data", "key-data", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cert == nil {
		t.Fatal("expected certificate, got nil")
	}
	if cert.ID != 1 { //nolint:staticcheck // t.Fatal above ensures cert is not nil
		t.Errorf("expected ID 1, got %d", cert.ID)
	}
}

func TestMockClient_GetCertificate_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	cert, err := m.GetCertificate(ctx, "test-cert")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cert != nil {
		t.Errorf("expected nil, got %v", cert)
	}
}

func TestMockClient_DeleteCertificate_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.DeleteCertificate(ctx, "test-cert")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_SetServerRDNS_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.SetServerRDNS(ctx, 123, "1.2.3.4", "example.com")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_SetLoadBalancerRDNS_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.SetLoadBalancerRDNS(ctx, 123, "1.2.3.4", "example.com")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_GetPublicIP_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	ip, err := m.GetPublicIP(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ip != "127.0.0.1" {
		t.Errorf("expected '127.0.0.1', got %q", ip)
	}
}

func TestMockClient_CleanupByLabel_Default(t *testing.T) {
	m := &MockClient{}
	ctx := context.Background()

	err := m.CleanupByLabel(ctx, map[string]string{"key": "value"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockClient_CustomFuncs(t *testing.T) {
	// Test that custom functions are called correctly
	ctx := context.Background()
	customErr := errors.New("custom error")

	t.Run("GetNetwork custom func", func(t *testing.T) {
		m := &MockClient{
			GetNetworkFunc: func(_ context.Context, name string) (*hcloud.Network, error) {
				return &hcloud.Network{ID: 42, Name: name}, nil
			},
		}
		network, err := m.GetNetwork(ctx, "custom-network")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if network.ID != 42 {
			t.Errorf("expected ID 42, got %d", network.ID)
		}
	})

	t.Run("DeleteServer custom func with error", func(t *testing.T) {
		m := &MockClient{
			DeleteServerFunc: func(_ context.Context, _ string) error {
				return customErr
			},
		}
		err := m.DeleteServer(ctx, "test")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("GetServersByLabel custom func", func(t *testing.T) {
		expectedServer := &hcloud.Server{ID: 100, Name: "test-server"}
		m := &MockClient{
			GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloud.Server, error) {
				return []*hcloud.Server{expectedServer}, nil
			},
		}
		servers, err := m.GetServersByLabel(ctx, map[string]string{"key": "value"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(servers) != 1 {
			t.Errorf("expected 1 server, got %d", len(servers))
		}
		if servers[0].ID != 100 {
			t.Errorf("expected server ID 100, got %d", servers[0].ID)
		}
	})

	t.Run("GetServerIP custom func", func(t *testing.T) {
		m := &MockClient{
			GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
				return "10.0.0.1", nil
			},
		}
		ip, err := m.GetServerIP(ctx, "test")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if ip != "10.0.0.1" {
			t.Errorf("expected '10.0.0.1', got %q", ip)
		}
	})

	t.Run("GetServerID custom func", func(t *testing.T) {
		m := &MockClient{
			GetServerIDFunc: func(_ context.Context, _ string) (string, error) {
				return "999", nil
			},
		}
		id, err := m.GetServerID(ctx, "test")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if id != "999" {
			t.Errorf("expected '999', got %q", id)
		}
	})

	t.Run("EnableRescue custom func", func(t *testing.T) {
		m := &MockClient{
			EnableRescueFunc: func(_ context.Context, _ string, _ []string) (string, error) {
				return "custom-password", nil
			},
		}
		pwd, err := m.EnableRescue(ctx, "123", nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if pwd != "custom-password" {
			t.Errorf("expected 'custom-password', got %q", pwd)
		}
	})

	t.Run("ResetServer custom func", func(t *testing.T) {
		m := &MockClient{
			ResetServerFunc: func(_ context.Context, _ string) error {
				return customErr
			},
		}
		err := m.ResetServer(ctx, "123")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("PoweroffServer custom func", func(t *testing.T) {
		m := &MockClient{
			PoweroffServerFunc: func(_ context.Context, _ string) error {
				return customErr
			},
		}
		err := m.PoweroffServer(ctx, "123")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("CreateSnapshot custom func", func(t *testing.T) {
		m := &MockClient{
			CreateSnapshotFunc: func(_ context.Context, _, _ string, _ map[string]string) (string, error) {
				return "custom-snapshot", nil
			},
		}
		id, err := m.CreateSnapshot(ctx, "123", "desc", nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if id != "custom-snapshot" {
			t.Errorf("expected 'custom-snapshot', got %q", id)
		}
	})

	t.Run("DeleteImage custom func", func(t *testing.T) {
		m := &MockClient{
			DeleteImageFunc: func(_ context.Context, _ string) error {
				return customErr
			},
		}
		err := m.DeleteImage(ctx, "123")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("GetSnapshotByLabels custom func", func(t *testing.T) {
		m := &MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
				return &hcloud.Image{ID: 42}, nil
			},
		}
		img, err := m.GetSnapshotByLabels(ctx, nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if img.ID != 42 {
			t.Errorf("expected ID 42, got %d", img.ID)
		}
	})

	t.Run("CreateSSHKey custom func", func(t *testing.T) {
		m := &MockClient{
			CreateSSHKeyFunc: func(_ context.Context, _, _ string, _ map[string]string) (string, error) {
				return "custom-key", nil
			},
		}
		id, err := m.CreateSSHKey(ctx, "key", "pub", nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if id != "custom-key" {
			t.Errorf("expected 'custom-key', got %q", id)
		}
	})

	t.Run("DeleteSSHKey custom func", func(t *testing.T) {
		m := &MockClient{
			DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
				return customErr
			},
		}
		err := m.DeleteSSHKey(ctx, "key")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("EnsureNetwork custom func", func(t *testing.T) {
		m := &MockClient{
			EnsureNetworkFunc: func(_ context.Context, _, _, _ string, _ map[string]string) (*hcloud.Network, error) {
				return &hcloud.Network{ID: 42}, nil
			},
		}
		net, err := m.EnsureNetwork(ctx, "net", "10.0.0.0/8", "eu-central", nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if net.ID != 42 {
			t.Errorf("expected ID 42, got %d", net.ID)
		}
	})

	t.Run("EnsureSubnet custom func", func(t *testing.T) {
		m := &MockClient{
			EnsureSubnetFunc: func(_ context.Context, _ *hcloud.Network, _, _ string, _ hcloud.NetworkSubnetType) error {
				return customErr
			},
		}
		err := m.EnsureSubnet(ctx, nil, "10.0.0.0/24", "eu-central", hcloud.NetworkSubnetTypeCloud)
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("DeleteNetwork custom func", func(t *testing.T) {
		m := &MockClient{
			DeleteNetworkFunc: func(_ context.Context, _ string) error {
				return customErr
			},
		}
		err := m.DeleteNetwork(ctx, "net")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("EnsureFirewall custom func", func(t *testing.T) {
		m := &MockClient{
			EnsureFirewallFunc: func(_ context.Context, _ string, _ []hcloud.FirewallRule, _ map[string]string, _ string) (*hcloud.Firewall, error) {
				return &hcloud.Firewall{ID: 42}, nil
			},
		}
		fw, err := m.EnsureFirewall(ctx, "fw", nil, nil, "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if fw.ID != 42 {
			t.Errorf("expected ID 42, got %d", fw.ID)
		}
	})

	t.Run("DeleteFirewall custom func", func(t *testing.T) {
		m := &MockClient{
			DeleteFirewallFunc: func(_ context.Context, _ string) error {
				return customErr
			},
		}
		err := m.DeleteFirewall(ctx, "fw")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("GetFirewall custom func", func(t *testing.T) {
		m := &MockClient{
			GetFirewallFunc: func(_ context.Context, _ string) (*hcloud.Firewall, error) {
				return &hcloud.Firewall{ID: 42}, nil
			},
		}
		fw, err := m.GetFirewall(ctx, "fw")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if fw.ID != 42 {
			t.Errorf("expected ID 42, got %d", fw.ID)
		}
	})

	t.Run("EnsureLoadBalancer custom func", func(t *testing.T) {
		m := &MockClient{
			EnsureLoadBalancerFunc: func(_ context.Context, _, _, _ string, _ hcloud.LoadBalancerAlgorithmType, _ map[string]string) (*hcloud.LoadBalancer, error) {
				return &hcloud.LoadBalancer{ID: 42}, nil
			},
		}
		lb, err := m.EnsureLoadBalancer(ctx, "lb", "fsn1", "lb11", hcloud.LoadBalancerAlgorithmTypeRoundRobin, nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if lb.ID != 42 {
			t.Errorf("expected ID 42, got %d", lb.ID)
		}
	})

	t.Run("ConfigureService custom func", func(t *testing.T) {
		m := &MockClient{
			ConfigureServiceFunc: func(_ context.Context, _ *hcloud.LoadBalancer, _ hcloud.LoadBalancerAddServiceOpts) error {
				return customErr
			},
		}
		err := m.ConfigureService(ctx, nil, hcloud.LoadBalancerAddServiceOpts{})
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("AddTarget custom func", func(t *testing.T) {
		m := &MockClient{
			AddTargetFunc: func(_ context.Context, _ *hcloud.LoadBalancer, _ hcloud.LoadBalancerTargetType, _ string) error {
				return customErr
			},
		}
		err := m.AddTarget(ctx, nil, hcloud.LoadBalancerTargetTypeServer, "")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("AttachToNetwork custom func", func(t *testing.T) {
		m := &MockClient{
			AttachToNetworkFunc: func(_ context.Context, _ *hcloud.LoadBalancer, _ *hcloud.Network, _ net.IP) error {
				return customErr
			},
		}
		err := m.AttachToNetwork(ctx, nil, nil, nil)
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("DeleteLoadBalancer custom func", func(t *testing.T) {
		m := &MockClient{
			DeleteLoadBalancerFunc: func(_ context.Context, _ string) error {
				return customErr
			},
		}
		err := m.DeleteLoadBalancer(ctx, "lb")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("GetLoadBalancer custom func", func(t *testing.T) {
		m := &MockClient{
			GetLoadBalancerFunc: func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
				return &hcloud.LoadBalancer{ID: 42}, nil
			},
		}
		lb, err := m.GetLoadBalancer(ctx, "lb")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if lb.ID != 42 {
			t.Errorf("expected ID 42, got %d", lb.ID)
		}
	})

	t.Run("EnsurePlacementGroup custom func", func(t *testing.T) {
		m := &MockClient{
			EnsurePlacementGroupFunc: func(_ context.Context, _, _ string, _ map[string]string) (*hcloud.PlacementGroup, error) {
				return &hcloud.PlacementGroup{ID: 42}, nil
			},
		}
		pg, err := m.EnsurePlacementGroup(ctx, "pg", "spread", nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if pg.ID != 42 {
			t.Errorf("expected ID 42, got %d", pg.ID)
		}
	})

	t.Run("DeletePlacementGroup custom func", func(t *testing.T) {
		m := &MockClient{
			DeletePlacementGroupFunc: func(_ context.Context, _ string) error {
				return customErr
			},
		}
		err := m.DeletePlacementGroup(ctx, "pg")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("GetPlacementGroup custom func", func(t *testing.T) {
		m := &MockClient{
			GetPlacementGroupFunc: func(_ context.Context, _ string) (*hcloud.PlacementGroup, error) {
				return &hcloud.PlacementGroup{ID: 42}, nil
			},
		}
		pg, err := m.GetPlacementGroup(ctx, "pg")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if pg.ID != 42 {
			t.Errorf("expected ID 42, got %d", pg.ID)
		}
	})

	t.Run("EnsureFloatingIP custom func", func(t *testing.T) {
		m := &MockClient{
			EnsureFloatingIPFunc: func(_ context.Context, _, _, _ string, _ map[string]string) (*hcloud.FloatingIP, error) {
				return &hcloud.FloatingIP{ID: 42}, nil
			},
		}
		fip, err := m.EnsureFloatingIP(ctx, "fip", "fsn1", "ipv4", nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if fip.ID != 42 {
			t.Errorf("expected ID 42, got %d", fip.ID)
		}
	})

	t.Run("DeleteFloatingIP custom func", func(t *testing.T) {
		m := &MockClient{
			DeleteFloatingIPFunc: func(_ context.Context, _ string) error {
				return customErr
			},
		}
		err := m.DeleteFloatingIP(ctx, "fip")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("GetFloatingIP custom func", func(t *testing.T) {
		m := &MockClient{
			GetFloatingIPFunc: func(_ context.Context, _ string) (*hcloud.FloatingIP, error) {
				return &hcloud.FloatingIP{ID: 42}, nil
			},
		}
		fip, err := m.GetFloatingIP(ctx, "fip")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if fip.ID != 42 {
			t.Errorf("expected ID 42, got %d", fip.ID)
		}
	})

	t.Run("EnsureCertificate custom func", func(t *testing.T) {
		m := &MockClient{
			EnsureCertificateFunc: func(_ context.Context, _, _, _ string, _ map[string]string) (*hcloud.Certificate, error) {
				return &hcloud.Certificate{ID: 42}, nil
			},
		}
		cert, err := m.EnsureCertificate(ctx, "cert", "data", "key", nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if cert.ID != 42 {
			t.Errorf("expected ID 42, got %d", cert.ID)
		}
	})

	t.Run("GetCertificate custom func", func(t *testing.T) {
		m := &MockClient{
			GetCertificateFunc: func(_ context.Context, _ string) (*hcloud.Certificate, error) {
				return &hcloud.Certificate{ID: 42}, nil
			},
		}
		cert, err := m.GetCertificate(ctx, "cert")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if cert.ID != 42 {
			t.Errorf("expected ID 42, got %d", cert.ID)
		}
	})

	t.Run("DeleteCertificate custom func", func(t *testing.T) {
		m := &MockClient{
			DeleteCertificateFunc: func(_ context.Context, _ string) error {
				return customErr
			},
		}
		err := m.DeleteCertificate(ctx, "cert")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("SetServerRDNS custom func", func(t *testing.T) {
		m := &MockClient{
			SetServerRDNSFunc: func(_ context.Context, _ int64, _, _ string) error {
				return customErr
			},
		}
		err := m.SetServerRDNS(ctx, 123, "1.2.3.4", "example.com")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("SetLoadBalancerRDNS custom func", func(t *testing.T) {
		m := &MockClient{
			SetLoadBalancerRDNSFunc: func(_ context.Context, _ int64, _, _ string) error {
				return customErr
			},
		}
		err := m.SetLoadBalancerRDNS(ctx, 123, "1.2.3.4", "example.com")
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})

	t.Run("GetPublicIP custom func", func(t *testing.T) {
		m := &MockClient{
			GetPublicIPFunc: func(_ context.Context) (string, error) {
				return "10.0.0.1", nil
			},
		}
		ip, err := m.GetPublicIP(ctx)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if ip != "10.0.0.1" {
			t.Errorf("expected '10.0.0.1', got %q", ip)
		}
	})

	t.Run("CleanupByLabel custom func", func(t *testing.T) {
		m := &MockClient{
			CleanupByLabelFunc: func(_ context.Context, _ map[string]string) error {
				return customErr
			},
		}
		err := m.CleanupByLabel(ctx, map[string]string{"key": "value"})
		if !errors.Is(err, customErr) {
			t.Errorf("expected custom error, got %v", err)
		}
	})
}
