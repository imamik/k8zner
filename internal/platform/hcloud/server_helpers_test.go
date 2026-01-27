package hcloud

import (
	"net"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func TestServerIPv4(t *testing.T) {
	tests := []struct {
		name     string
		server   *hcloud.Server
		expected string
	}{
		{
			name:     "nil server",
			server:   nil,
			expected: "",
		},
		{
			name: "server with IPv4",
			server: &hcloud.Server{
				PublicNet: hcloud.ServerPublicNet{
					IPv4: hcloud.ServerPublicNetIPv4{
						IP: net.ParseIP("203.0.113.42"),
					},
				},
			},
			expected: "203.0.113.42",
		},
		{
			name: "server without IPv4",
			server: &hcloud.Server{
				PublicNet: hcloud.ServerPublicNet{
					IPv4: hcloud.ServerPublicNetIPv4{},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ServerIPv4(tt.server)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestServerIPv6(t *testing.T) {
	tests := []struct {
		name     string
		server   *hcloud.Server
		expected string
	}{
		{
			name:     "nil server",
			server:   nil,
			expected: "",
		},
		{
			name: "server with IPv6",
			server: &hcloud.Server{
				PublicNet: hcloud.ServerPublicNet{
					IPv6: hcloud.ServerPublicNetIPv6{
						IP: net.ParseIP("2001:db8::1"),
					},
				},
			},
			expected: "2001:db8::1",
		},
		{
			name: "server without IPv6",
			server: &hcloud.Server{
				PublicNet: hcloud.ServerPublicNet{
					IPv6: hcloud.ServerPublicNetIPv6{},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ServerIPv6(tt.server)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestLoadBalancerIPv4(t *testing.T) {
	tests := []struct {
		name     string
		lb       *hcloud.LoadBalancer
		expected string
	}{
		{
			name:     "nil load balancer",
			lb:       nil,
			expected: "",
		},
		{
			name: "load balancer with IPv4",
			lb: &hcloud.LoadBalancer{
				PublicNet: hcloud.LoadBalancerPublicNet{
					IPv4: hcloud.LoadBalancerPublicNetIPv4{
						IP: net.ParseIP("203.0.113.100"),
					},
				},
			},
			expected: "203.0.113.100",
		},
		{
			name: "load balancer without IPv4",
			lb: &hcloud.LoadBalancer{
				PublicNet: hcloud.LoadBalancerPublicNet{
					IPv4: hcloud.LoadBalancerPublicNetIPv4{},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LoadBalancerIPv4(tt.lb)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestLoadBalancerIPv6(t *testing.T) {
	tests := []struct {
		name     string
		lb       *hcloud.LoadBalancer
		expected string
	}{
		{
			name:     "nil load balancer",
			lb:       nil,
			expected: "",
		},
		{
			name: "load balancer with IPv6",
			lb: &hcloud.LoadBalancer{
				PublicNet: hcloud.LoadBalancerPublicNet{
					IPv6: hcloud.LoadBalancerPublicNetIPv6{
						IP: net.ParseIP("2001:db8::100"),
					},
				},
			},
			expected: "2001:db8::100",
		},
		{
			name: "load balancer without IPv6",
			lb: &hcloud.LoadBalancer{
				PublicNet: hcloud.LoadBalancerPublicNet{
					IPv6: hcloud.LoadBalancerPublicNetIPv6{},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LoadBalancerIPv6(tt.lb)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestResolvePlacementGroup is already tested in real_client_test.go
