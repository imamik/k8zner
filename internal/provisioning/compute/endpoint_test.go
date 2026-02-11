package compute

import (
	"context"
	"net"
	"testing"

	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareControlPlaneEndpoint_WithLB(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetLoadBalancerFunc = func(_ context.Context, name string) (*hcloud.LoadBalancer, error) {
		assert.Contains(t, name, "test-cluster")
		return &hcloud.LoadBalancer{
			ID: 1,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("5.6.7.8"),
				},
			},
			PrivateNet: []hcloud.LoadBalancerPrivateNet{
				{IP: net.ParseIP("10.0.64.1")},
			},
		}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.prepareControlPlaneEndpoint(ctx)

	require.NoError(t, err)
	assert.Contains(t, ctx.State.SANs, "5.6.7.8")
	assert.Contains(t, ctx.State.SANs, "10.0.64.1")
	assert.Len(t, ctx.State.SANs, 2)
}

func TestPrepareControlPlaneEndpoint_NoLB(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return nil, nil // No LB
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.prepareControlPlaneEndpoint(ctx)

	require.NoError(t, err)
	assert.Empty(t, ctx.State.SANs)
}

func TestPrepareControlPlaneEndpoint_LBWithMultiplePrivateNets(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID: 1,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("1.2.3.4"),
				},
			},
			PrivateNet: []hcloud.LoadBalancerPrivateNet{
				{IP: net.ParseIP("10.0.0.1")},
				{IP: net.ParseIP("10.0.0.2")},
			},
		}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.prepareControlPlaneEndpoint(ctx)

	require.NoError(t, err)
	assert.Len(t, ctx.State.SANs, 3) // 1 public + 2 private
	assert.Contains(t, ctx.State.SANs, "1.2.3.4")
	assert.Contains(t, ctx.State.SANs, "10.0.0.1")
	assert.Contains(t, ctx.State.SANs, "10.0.0.2")
}

func TestPrepareControlPlaneEndpoint_LBError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return nil, assert.AnError
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.prepareControlPlaneEndpoint(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get load balancer")
}

func TestPrepareControlPlaneEndpoint_SetsEndpoint(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID: 1,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{
					IP: net.ParseIP("5.6.7.8"),
				},
			},
		}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	mockTalos := ctx.Talos.(*mockTalosProducer)
	p := NewProvisioner()

	err := p.prepareControlPlaneEndpoint(ctx)

	require.NoError(t, err)
	assert.Equal(t, "https://5.6.7.8:6443", mockTalos.endpoint)
}

func TestPrepareControlPlaneEndpoint_LBWithNoPublicIP(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetLoadBalancerFunc = func(_ context.Context, _ string) (*hcloud.LoadBalancer, error) {
		return &hcloud.LoadBalancer{
			ID: 1,
			PublicNet: hcloud.LoadBalancerPublicNet{
				IPv4: hcloud.LoadBalancerPublicNetIPv4{},
			},
			PrivateNet: []hcloud.LoadBalancerPrivateNet{
				{IP: net.ParseIP("10.0.64.1")},
			},
		}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	err := p.prepareControlPlaneEndpoint(ctx)

	require.NoError(t, err)
	// Only private net IPs should be in SANs
	assert.Contains(t, ctx.State.SANs, "10.0.64.1")
}
