package infrastructure

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyLoadBalancerRDNS_KubeAPIIPv4(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		RDNS: config.RDNSConfig{
			ClusterRDNSIPv4: "{{ hostname }}.example.com",
		},
	}

	var capturedIP, capturedDNS string
	mockInfra.SetLoadBalancerRDNSFunc = func(_ context.Context, _ int64, ipAddress, dnsPtr string) error {
		capturedIP = ipAddress
		capturedDNS = dnsPtr
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.applyLoadBalancerRDNS(ctx, 1, "test-cluster-kube", "1.2.3.4", "", "kube-api")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3.4", capturedIP)
	assert.Equal(t, "test-cluster-kube.example.com", capturedDNS)
}

func TestApplyLoadBalancerRDNS_KubeAPIIPv6(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		RDNS: config.RDNSConfig{
			ClusterRDNSIPv6: "{{ hostname }}.v6.example.com",
		},
	}

	var capturedIP, capturedDNS string
	mockInfra.SetLoadBalancerRDNSFunc = func(_ context.Context, _ int64, ipAddress, dnsPtr string) error {
		capturedIP = ipAddress
		capturedDNS = dnsPtr
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.applyLoadBalancerRDNS(ctx, 1, "test-cluster-kube", "", "2001:db8::1", "kube-api")
	require.NoError(t, err)
	assert.Equal(t, "2001:db8::1", capturedIP)
	assert.Equal(t, "test-cluster-kube.v6.example.com", capturedDNS)
}

func TestApplyLoadBalancerRDNS_KubeAPIBothIPv4AndIPv6(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		RDNS: config.RDNSConfig{
			ClusterRDNS: "{{ hostname }}.example.com",
		},
	}

	var calls []string
	mockInfra.SetLoadBalancerRDNSFunc = func(_ context.Context, _ int64, ipAddress, _ string) error {
		calls = append(calls, ipAddress)
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.applyLoadBalancerRDNS(ctx, 1, "test-cluster-kube", "1.2.3.4", "2001:db8::1", "kube-api")
	require.NoError(t, err)
	assert.Len(t, calls, 2)
	assert.Contains(t, calls, "1.2.3.4")
	assert.Contains(t, calls, "2001:db8::1")
}

func TestApplyLoadBalancerRDNS_NoTemplateConfigured(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
	}

	// SetLoadBalancerRDNSFunc should not be called
	mockInfra.SetLoadBalancerRDNSFunc = func(_ context.Context, _ int64, _, _ string) error {
		t.Fatal("SetLoadBalancerRDNS should not be called when no RDNS is configured")
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.applyLoadBalancerRDNS(ctx, 1, "test-cluster-kube", "1.2.3.4", "2001:db8::1", "kube-api")
	require.NoError(t, err)
}

func TestApplyLoadBalancerRDNS_EmptyIPsSkipped(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		RDNS: config.RDNSConfig{
			ClusterRDNS: "{{ hostname }}.example.com",
		},
	}

	mockInfra.SetLoadBalancerRDNSFunc = func(_ context.Context, _ int64, _, _ string) error {
		t.Fatal("SetLoadBalancerRDNS should not be called for empty IPs")
		return nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.applyLoadBalancerRDNS(ctx, 1, "test-cluster-kube", "", "", "kube-api")
	require.NoError(t, err)
}

func TestApplyLoadBalancerRDNS_SetRDNSIPv4Error(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		RDNS: config.RDNSConfig{
			ClusterRDNSIPv4: "{{ hostname }}.example.com",
		},
	}

	mockInfra.SetLoadBalancerRDNSFunc = func(_ context.Context, _ int64, _, _ string) error {
		return fmt.Errorf("API RDNS error")
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.applyLoadBalancerRDNS(ctx, 1, "test-cluster-kube", "1.2.3.4", "", "kube-api")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set IPv4 RDNS")
}

func TestApplyLoadBalancerRDNS_SetRDNSIPv6Error(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		RDNS: config.RDNSConfig{
			ClusterRDNSIPv6: "{{ hostname }}.example.com",
		},
	}

	mockInfra.SetLoadBalancerRDNSFunc = func(_ context.Context, _ int64, _, _ string) error {
		return fmt.Errorf("API RDNS error")
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.applyLoadBalancerRDNS(ctx, 1, "test-cluster-kube", "", "2001:db8::1", "kube-api")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set IPv6 RDNS")
}

func TestApplyLoadBalancerRDNS_InvalidIPv4TemplateError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		RDNS: config.RDNSConfig{
			// Template with unresolved variable causes RenderTemplate error
			ClusterRDNSIPv4: "{{ unknown-var }}.example.com",
		},
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.applyLoadBalancerRDNS(ctx, 1, "test-cluster-kube", "1.2.3.4", "", "kube-api")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to render IPv4 RDNS template")
}

func TestApplyLoadBalancerRDNS_InvalidIPv6TemplateError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := &config.Config{
		ClusterName: "test-cluster",
		RDNS: config.RDNSConfig{
			ClusterRDNSIPv6: "{{ unknown-var }}.example.com",
		},
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.applyLoadBalancerRDNS(ctx, 1, "test-cluster-kube", "", "2001:db8::1", "kube-api")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to render IPv6 RDNS template")
}

// --- Full LB provisioning with RDNS ---

func TestProvisionLoadBalancers_WithRDNS(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)
	cfg.RDNS = config.RDNSConfig{
		ClusterRDNSIPv4: "{{ hostname }}.example.com",
	}

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	ctx := createTestContext(t, mockInfra, cfg)
	ctx.State.Network = &hcloud.Network{ID: 1, IPRange: ipNet}

	setupLBMock(mockInfra)

	var rdnsCalled bool
	mockInfra.SetLoadBalancerRDNSFunc = func(_ context.Context, _ int64, _, _ string) error {
		rdnsCalled = true
		return nil
	}
	mockInfra.GetLoadBalancerFunc = func(_ context.Context, name string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID:   1,
			Name: name,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{IP: net.ParseIP("5.6.7.8")},
			},
		}, nil
	}

	p := NewProvisioner()
	err := p.ProvisionLoadBalancers(ctx)

	require.NoError(t, err)
	assert.True(t, rdnsCalled, "RDNS should have been set")
	assert.NotNil(t, ctx.State.LoadBalancer)
}

func TestProvisionLoadBalancers_GetLoadBalancerRefreshError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	ctx := createTestContext(t, mockInfra, cfg)
	ctx.State.Network = &hcloud.Network{ID: 1, IPRange: ipNet}

	setupLBMock(mockInfra)

	// GetLoadBalancer fails, should fall back to local object
	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return nil, fmt.Errorf("refresh failed")
	}

	p := NewProvisioner()
	err := p.ProvisionLoadBalancers(ctx)

	require.NoError(t, err)
	// Should fall back to local object
	assert.NotNil(t, ctx.State.LoadBalancer)
}

func TestProvisionLoadBalancers_AddTargetError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	ctx := createTestContext(t, mockInfra, cfg)
	ctx.State.Network = &hcloud.Network{ID: 1, IPRange: ipNet}

	mockInfra.EnsureLoadBalancerFunc = func(_ context.Context, name, _, _ string, _ hcloud.LoadBalancerAlgorithmType, _ map[string]string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID:   1,
			Name: name,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{IP: net.ParseIP("5.6.7.8")},
			},
		}, nil
	}
	mockInfra.ConfigureServiceFunc = func(_ context.Context, _ *hcloud.LoadBalancer, _ hcloud.LoadBalancerAddServiceOpts) error {
		return nil
	}
	mockInfra.AttachToNetworkFunc = func(_ context.Context, _ *hcloud.LoadBalancer, _ *hcloud.Network, _ net.IP) error {
		return nil
	}
	mockInfra.AddTargetFunc = func(_ context.Context, _ *hcloud.LoadBalancer, _ hcloud.LoadBalancerTargetType, _ string) error {
		return fmt.Errorf("target add failed")
	}

	p := NewProvisioner()
	err := p.ProvisionLoadBalancers(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add target")
}

func TestProvisionLoadBalancers_TalosAPIServiceError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := newTestConfig(t)

	_, ipNet, _ := net.ParseCIDR("10.0.0.0/16")
	ctx := createTestContext(t, mockInfra, cfg)
	ctx.State.Network = &hcloud.Network{ID: 1, IPRange: ipNet}

	mockInfra.EnsureLoadBalancerFunc = func(_ context.Context, name, _, _ string, _ hcloud.LoadBalancerAlgorithmType, _ map[string]string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID:   1,
			Name: name,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{IP: net.ParseIP("5.6.7.8")},
			},
		}, nil
	}

	serviceCallCount := 0
	mockInfra.ConfigureServiceFunc = func(_ context.Context, _ *hcloud.LoadBalancer, svc hcloud.LoadBalancerAddServiceOpts) error {
		serviceCallCount++
		// First service (6443) succeeds, second (50000) fails
		if serviceCallCount == 2 {
			return fmt.Errorf("talos service config failed")
		}
		return nil
	}

	p := NewProvisioner()
	err := p.ProvisionLoadBalancers(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "talos service config failed")
}
