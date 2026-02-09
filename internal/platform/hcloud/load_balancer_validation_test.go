package hcloud

import (
	"context"
	"net"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachToNetwork_NilIPValidation(t *testing.T) {
	t.Parallel()
	client := NewRealClient("test-token")

	// Create mock load balancer and network objects
	lb := &hcloud.LoadBalancer{
		ID:         1,
		Name:       "test-lb",
		PrivateNet: []hcloud.LoadBalancerPrivateNet{},
	}

	network := &hcloud.Network{
		ID:   1,
		Name: "test-network",
	}

	ctx := context.Background()

	// Test with nil IP - should return validation error before hitting API
	err := client.AttachToNetwork(ctx, lb, network, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ip parameter is required")
}

func TestAttachToNetwork_ValidIPFormat(t *testing.T) {
	t.Parallel()
	client := NewRealClient("test-token")

	lb := &hcloud.LoadBalancer{
		ID:         1,
		Name:       "test-lb",
		PrivateNet: []hcloud.LoadBalancerPrivateNet{},
	}

	network := &hcloud.Network{
		ID:   1,
		Name: "test-network",
	}

	ctx := context.Background()

	// Test with valid IP - should pass validation (will fail at API call level)
	// This verifies the validation passes when IP is provided
	validIP := net.ParseIP("10.0.0.5")
	err := client.AttachToNetwork(ctx, lb, network, validIP)
	// Error expected due to API call, but should NOT be validation error
	if err != nil {
		assert.NotContains(t, err.Error(), "ip parameter is required")
	}
}
