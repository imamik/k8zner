package config

import (
	"testing"
)

func TestCIDRSubnet(t *testing.T) {
	tests := []struct {
		prefix  string
		newbits int
		netnum  int
		want    string
		wantErr bool
	}{
		{"10.0.0.0/16", 8, 0, "10.0.0.0/24", false},
		{"10.0.0.0/16", 8, 1, "10.0.1.0/24", false},
		{"10.0.0.0/16", 8, 255, "10.0.255.0/24", false},
		{"10.0.0.0/16", 8, 256, "", true},
		{"10.0.0.0/16", 3, 2, "10.0.64.0/19", false},  // Node CIDR Default
		{"10.0.0.0/16", 3, 3, "10.0.96.0/19", false},  // Service CIDR Default
		{"10.0.0.0/16", 1, 1, "10.0.128.0/17", false}, // Pod CIDR Default
	}

	for _, tt := range tests {
		got, err := CIDRSubnet(tt.prefix, tt.newbits, tt.netnum)
		if (err != nil) != tt.wantErr {
			t.Errorf("CIDRSubnet(%q, %d, %d) error = %v, wantErr %v", tt.prefix, tt.newbits, tt.netnum, err, tt.wantErr)
			return
		}
		if got != tt.want {
			t.Errorf("CIDRSubnet(%q, %d, %d) = %v, want %v", tt.prefix, tt.newbits, tt.netnum, got, tt.want)
		}
	}
}

func TestCalculateSubnets(t *testing.T) {
	cfg := &Config{
		Network: NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
	}

	err := cfg.CalculateSubnets()
	if err != nil {
		t.Fatalf("CalculateSubnets() error = %v", err)
	}

	if cfg.Network.NodeIPv4CIDR != "10.0.64.0/19" {
		t.Errorf("NodeIPv4CIDR = %v, want 10.0.64.0/19", cfg.Network.NodeIPv4CIDR)
	}
	if cfg.Network.ServiceIPv4CIDR != "10.0.96.0/19" {
		t.Errorf("ServiceIPv4CIDR = %v, want 10.0.96.0/19", cfg.Network.ServiceIPv4CIDR)
	}
	if cfg.Network.PodIPv4CIDR != "10.0.128.0/17" {
		t.Errorf("PodIPv4CIDR = %v, want 10.0.128.0/17", cfg.Network.PodIPv4CIDR)
	}
	if cfg.Network.NodeIPv4SubnetMask != 25 {
		t.Errorf("NodeIPv4SubnetMask = %v, want 25", cfg.Network.NodeIPv4SubnetMask)
	}
}

func TestGetSubnetForRole(t *testing.T) {
	cfg := &Config{
		Network: NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
	}
	_ = cfg.CalculateSubnets()

	// Control Plane
	cpSubnet, err := cfg.GetSubnetForRole("control-plane", 0)
	if err != nil {
		t.Fatalf("GetSubnetForRole(control-plane) error = %v", err)
	}
	// Node CIDR is /19. Target is /25. Diff is 6 bits.
	// Index 0.
	// 10.0.64.0/19 -> 10.0.64.0/25
	if cpSubnet != "10.0.64.0/25" {
		t.Errorf("ControlPlane Subnet = %v, want 10.0.64.0/25", cpSubnet)
	}

	// Load Balancer
	lbSubnet, err := cfg.GetSubnetForRole("load-balancer", 0)
	if err != nil {
		t.Fatalf("GetSubnetForRole(load-balancer) error = %v", err)
	}
	// Index 1.
	// 10.0.64.0/25 (size 128). Next one starts at .128
	// 10.0.64.128/25
	if lbSubnet != "10.0.64.128/25" {
		t.Errorf("LoadBalancer Subnet = %v, want 10.0.64.128/25", lbSubnet)
	}

	// Worker 1
	w1Subnet, err := cfg.GetSubnetForRole("worker", 0)
	if err != nil {
		t.Fatalf("GetSubnetForRole(worker, 0) error = %v", err)
	}
	// Index 2 + 0 = 2.
	// 10.0.64.128 + 128 = 10.0.65.0
	// 10.0.65.0/25
	if w1Subnet != "10.0.65.0/25" {
		t.Errorf("Worker 1 Subnet = %v, want 10.0.65.0/25", w1Subnet)
	}
}

func TestCIDRHost(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		hostnum int
		want    string
		wantErr bool
	}{
		{
			name:    "first host in /24",
			prefix:  "10.0.0.0/24",
			hostnum: 1,
			want:    "10.0.0.1",
			wantErr: false,
		},
		{
			name:    "host 10 in /24",
			prefix:  "10.0.0.0/24",
			hostnum: 10,
			want:    "10.0.0.10",
			wantErr: false,
		},
		{
			name:    "last host in /24",
			prefix:  "10.0.0.0/24",
			hostnum: 255,
			want:    "10.0.0.255",
			wantErr: false,
		},
		{
			name:    "negative hostnum counts from end",
			prefix:  "10.0.0.0/24",
			hostnum: -1,
			want:    "10.0.0.255",
			wantErr: false,
		},
		{
			name:    "negative hostnum -2",
			prefix:  "10.0.0.0/24",
			hostnum: -2,
			want:    "10.0.0.254",
			wantErr: false,
		},
		{
			name:    "host in /16 network",
			prefix:  "192.168.0.0/16",
			hostnum: 1000,
			want:    "192.168.3.232",
			wantErr: false,
		},
		{
			name:    "host exceeds max",
			prefix:  "10.0.0.0/24",
			hostnum: 256,
			want:    "",
			wantErr: true,
		},
		{
			name:    "negative host exceeds max",
			prefix:  "10.0.0.0/24",
			hostnum: -257,
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid CIDR",
			prefix:  "invalid",
			hostnum: 1,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CIDRHost(tt.prefix, tt.hostnum)
			if (err != nil) != tt.wantErr {
				t.Errorf("CIDRHost() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CIDRHost() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCIDRSubnet_IPv6Error(t *testing.T) {
	_, err := CIDRSubnet("2001:db8::/32", 8, 0)
	if err == nil {
		t.Error("CIDRSubnet() expected error for IPv6, got nil")
	}
}

func TestCIDRHost_IPv6Error(t *testing.T) {
	_, err := CIDRHost("2001:db8::/32", 1)
	if err == nil {
		t.Error("CIDRHost() expected error for IPv6, got nil")
	}
}

func TestCIDRSubnet_InvalidNewbits(t *testing.T) {
	// Test when newbits would exceed 32 bits total
	_, err := CIDRSubnet("10.0.0.0/24", 16, 0)
	if err == nil {
		t.Error("CIDRSubnet() expected error for newbits exceeding 32, got nil")
	}
}
