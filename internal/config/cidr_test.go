package config

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBigIntFromIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ip       net.IP
		expected uint64
	}{
		{
			name:     "4-byte IPv4",
			ip:       net.IP{192, 168, 1, 1},
			expected: 3232235777, // 192*2^24 + 168*2^16 + 1*2^8 + 1
		},
		{
			name:     "16-byte IPv4-mapped (::ffff:192.168.1.1)",
			ip:       net.ParseIP("192.168.1.1"), // Returns 16-byte representation
			expected: 3232235777,
		},
		{
			name:     "pure IPv6 returns 0",
			ip:       net.ParseIP("2001:db8::1"),
			expected: 0,
		},
		{
			name:     "all zeros IPv4",
			ip:       net.IP{0, 0, 0, 0},
			expected: 0,
		},
		{
			name:     "all ones IPv4",
			ip:       net.IP{255, 255, 255, 255},
			expected: 4294967295, // 2^32 - 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := bigIntFromIP(tt.ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIpFromBigInt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		val      uint64
		expected net.IP
	}{
		{
			name:     "192.168.1.1",
			val:      3232235777,
			expected: net.IP{192, 168, 1, 1},
		},
		{
			name:     "all zeros",
			val:      0,
			expected: net.IP{0, 0, 0, 0},
		},
		{
			name:     "all ones",
			val:      4294967295,
			expected: net.IP{255, 255, 255, 255},
		},
		{
			name:     "10.0.0.1",
			val:      167772161, // 10*2^24 + 0*2^16 + 0*2^8 + 1
			expected: net.IP{10, 0, 0, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ipFromBigInt(tt.val)
			assert.Equal(t, tt.expected, result)
		})
	}
}
