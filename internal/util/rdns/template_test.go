package rdns

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTemplate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		template  string
		vars      TemplateVars
		want      string
		wantError bool
	}{
		{
			name:     "empty template",
			template: "",
			vars:     TemplateVars{},
			want:     "",
		},
		{
			name:     "simple hostname substitution",
			template: "{{ hostname }}.example.com",
			vars: TemplateVars{
				Hostname: "server-1",
			},
			want: "server-1.example.com",
		},
		{
			name:     "cluster name and role",
			template: "{{ role }}.{{ cluster-name }}.example.com",
			vars: TemplateVars{
				ClusterName: "prod-cluster",
				Role:        "control-plane",
			},
			want: "control-plane.prod-cluster.example.com",
		},
		{
			name:     "ipv4 labels",
			template: "{{ ip-labels }}.in-addr.arpa",
			vars: TemplateVars{
				IPAddress: "192.168.1.10",
				IPType:    "ipv4",
			},
			want: "10.1.168.192.in-addr.arpa",
		},
		{
			name:     "ipv6 labels",
			template: "{{ ip-labels }}.ip6.arpa",
			vars: TemplateVars{
				IPAddress: "2001:db8::1",
				IPType:    "ipv6",
			},
			want: "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa",
		},
		{
			name:     "all variables",
			template: "{{ hostname }}-{{ id }}-{{ pool }}-{{ role }}-{{ ip-type }}.{{ cluster-name }}.{{ cluster-domain }}",
			vars: TemplateVars{
				ClusterDomain: "k8s.example.com",
				ClusterName:   "prod",
				Hostname:      "server-1",
				ID:            12345,
				IPType:        "ipv4",
				Pool:          "worker-1",
				Role:          "worker",
			},
			want: "server-1-12345-worker-1-worker-ipv4.prod.k8s.example.com",
		},
		{
			name:     "pool and role with ip labels",
			template: "{{ pool }}-{{ role }}-{{ ip-labels }}.example.com",
			vars: TemplateVars{
				IPAddress: "10.0.1.5",
				Pool:      "control-plane",
				Role:      "control-plane",
			},
			want: "control-plane-control-plane-5.1.0.10.example.com",
		},
		{
			name:      "unresolved template variable",
			template:  "{{ hostname }}.{{ unknown }}.com",
			vars:      TemplateVars{Hostname: "server"},
			wantError: true,
		},
		{
			name:      "invalid ip address",
			template:  "{{ ip-labels }}.arpa",
			vars:      TemplateVars{IPAddress: "not-an-ip"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := RenderTemplate(tt.template, tt.vars)

			if tt.wantError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateIPLabels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		ipAddr    string
		want      string
		wantError bool
	}{
		{
			name:   "ipv4 simple",
			ipAddr: "1.2.3.4",
			want:   "4.3.2.1",
		},
		{
			name:   "ipv4 with zeros",
			ipAddr: "10.0.0.1",
			want:   "1.0.0.10",
		},
		{
			name:   "ipv4 large numbers",
			ipAddr: "192.168.255.254",
			want:   "254.255.168.192",
		},
		{
			name:   "ipv6 loopback",
			ipAddr: "::1",
			want:   "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0",
		},
		{
			name:   "ipv6 abbreviated",
			ipAddr: "2001:db8::1",
			want:   "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2",
		},
		{
			name:   "ipv6 full",
			ipAddr: "2001:0db8:0000:0000:0000:0000:0000:0001",
			want:   "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2",
		},
		{
			name:   "ipv6 with multiple abbreviations",
			ipAddr: "fe80::dead:beef",
			want:   "f.e.e.b.d.a.e.d.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.e.f",
		},
		{
			name:      "invalid ip",
			ipAddr:    "not-an-ip",
			wantError: true,
		},
		{
			name:      "empty ip",
			ipAddr:    "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := generateIPLabels(tt.ipAddr)

			if tt.wantError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReverseIPv4(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ipv4 string
		want string
	}{
		{"1.2.3.4", "4.3.2.1"},
		{"192.168.1.1", "1.1.168.192"},
		{"10.0.0.1", "1.0.0.10"},
		{"255.255.255.255", "255.255.255.255"},
	}

	for _, tt := range tests {
		t.Run(tt.ipv4, func(t *testing.T) {
			t.Parallel()
			got := reverseIPv4(tt.ipv4)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExpandIPv6(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ip   string
		want string
	}{
		{
			name: "abbreviated",
			ip:   "2001:db8::1",
			want: "2001:0db8:0000:0000:0000:0000:0000:0001",
		},
		{
			name: "loopback",
			ip:   "::1",
			want: "0000:0000:0000:0000:0000:0000:0000:0001",
		},
		{
			name: "already expanded",
			ip:   "2001:0db8:0000:0000:0000:0000:0000:0001",
			want: "2001:0db8:0000:0000:0000:0000:0000:0001",
		},
		{
			name: "multiple abbreviations",
			ip:   "fe80::dead:beef",
			want: "fe80:0000:0000:0000:0000:0000:dead:beef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := parseIP(t, tt.ip)
			got := expandIPv6(ip)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasUnresolvedTemplates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"no templates", "simple.example.com", false},
		{"resolved", "server-1.prod.example.com", false},
		{"with braces but not template", "data{123}.com", false},
		{"unresolved variable", "{{ hostname }}.com", true},
		{"unresolved with spaces", "{{  cluster-name  }}.com", true},
		{"multiple unresolved", "{{ host }}.{{ domain }}.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hasUnresolvedTemplates(tt.s)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRenderTemplateEdgeCases(t *testing.T) {
	t.Parallel(
	// Generate a 253-character DNS name (maximum allowed by RFC 1035)
	// Build a long string first, then truncate to exact length
	)

	longDNS := "server123456789." + // 16 chars
		"very-long-subdomain-name-with-many-characters-to-reach-limit." + // 62 chars
		"another-long-subdomain-with-additional-characters-for-length." + // 62 chars
		"yet-another-subdomain-to-make-sure-we-reach-exactly-253-chars." + // 63 chars
		"and-even-more-characters-to-ensure-we-have-enough-length-here." + // 63 chars
		"example.com" // 11 chars = 277 chars total
	// Truncate to exactly 253 characters (max DNS name length)
	maxLengthDNS := longDNS[:253]

	// Build a shorter template that, after substitution, exceeds max length
	// Template: 230 chars + "{{ hostname }}." (15 chars) = 245 chars
	// Result: 230 chars + "very-long-hostname-that-causes-overflow." (40 chars) = 270 chars
	shortTemplate := longDNS[:230]

	tests := []struct {
		name        string
		template    string
		vars        TemplateVars
		want        string
		wantErr     bool
		errContains string
	}{
		{
			name:     "maximum DNS name length (253 chars)",
			template: maxLengthDNS,
			vars:     TemplateVars{},
			want:     maxLengthDNS,
		},
		{
			name:        "template exceeds maximum length",
			template:    maxLengthDNS + "x", // 254 chars
			vars:        TemplateVars{},
			wantErr:     true,
			errContains: "exceeds maximum DNS name length",
		},
		{
			name:     "result exceeds max length after substitution",
			template: "{{ hostname }}." + shortTemplate, // Template is 245 chars, result will exceed 253
			vars: TemplateVars{
				Hostname: "very-long-hostname-that-causes-overflow",
			},
			wantErr:     true,
			errContains: "exceeds maximum length",
		},
		{
			name:     "empty hostname value",
			template: "{{ hostname }}.example.com",
			vars: TemplateVars{
				Hostname: "",
			},
			want: ".example.com",
		},
		{
			name:     "empty pool and role values",
			template: "{{ pool }}-{{ role }}.example.com",
			vars: TemplateVars{
				Pool: "",
				Role: "",
			},
			want: "-.example.com",
		},
		{
			name:     "empty cluster name",
			template: "server.{{ cluster-name }}.com",
			vars: TemplateVars{
				ClusterName: "",
			},
			want: "server..com",
		},
		{
			name:     "int64 max value for ID",
			template: "server-{{ id }}.example.com",
			vars: TemplateVars{
				ID: 9223372036854775807, // MaxInt64
			},
			want: "server-9223372036854775807.example.com",
		},
		{
			name:     "zero ID value",
			template: "server-{{ id }}.example.com",
			vars: TemplateVars{
				ID: 0,
			},
			want: "server-0.example.com",
		},
		{
			name:     "negative ID value",
			template: "server-{{ id }}.example.com",
			vars: TemplateVars{
				ID: -1,
			},
			want: "server--1.example.com",
		},
		{
			// Empty braces don't match the pattern (requires at least one letter)
			// so they pass through unchanged - this is acceptable behavior
			name:     "malformed template - empty braces passes through",
			template: "server.{{ }}.com",
			vars:     TemplateVars{},
			want:     "server.{{ }}.com",
		},
		{
			// No spaces means it doesn't match replacement pattern {{ hostname }}
			// but it DOES match unresolved pattern (regex allows zero whitespace)
			name:        "malformed template - no spaces detected as unresolved",
			template:    "server.{{hostname}}.com",
			vars:        TemplateVars{Hostname: "test"},
			wantErr:     true,
			errContains: "unresolved template variables",
		},
		{
			// Extra braces: the inner {{ hostname }} gets replaced,
			// leaving {test} which doesn't match unresolved pattern
			name:     "malformed template - extra braces partial replacement",
			template: "server.{{{ hostname }}}.com",
			vars:     TemplateVars{Hostname: "test"},
			want:     "server.{test}.com",
		},
		{
			name:     "special characters in hostname",
			template: "{{ hostname }}.example.com",
			vars: TemplateVars{
				Hostname: "server@#$%",
			},
			want: "server@#$%.example.com",
		},
		{
			name:     "unicode characters in hostname",
			template: "{{ hostname }}.example.com",
			vars: TemplateVars{
				Hostname: "服务器", // "server" in Chinese
			},
			want: "服务器.example.com",
		},
		{
			name:     "unicode characters in cluster name",
			template: "server.{{ cluster-name }}.com",
			vars: TemplateVars{
				ClusterName: "测试集群", // "test cluster" in Chinese
			},
			want: "server.测试集群.com",
		},
		{
			name:     "all empty variables",
			template: "{{ hostname }}.{{ cluster-name }}.{{ pool }}.{{ role }}.com",
			vars:     TemplateVars{},
			want:     "....com",
		},
		{
			name:     "template with no variables",
			template: "static.example.com",
			vars:     TemplateVars{Hostname: "ignored"},
			want:     "static.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := RenderTemplate(tt.template, tt.vars)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Helper to parse IP and fail test if invalid
func parseIP(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	require.NotNil(t, ip, "invalid IP: %s", s)
	return ip
}

func TestResolveTemplate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		templates []string
		want      string
	}{
		{
			name:      "first non-empty",
			templates: []string{"first", "second", "third"},
			want:      "first",
		},
		{
			name:      "skip empty first",
			templates: []string{"", "second", "third"},
			want:      "second",
		},
		{
			name:      "skip multiple empties",
			templates: []string{"", "", "third"},
			want:      "third",
		},
		{
			name:      "all empty",
			templates: []string{"", "", ""},
			want:      "",
		},
		{
			name:      "no templates",
			templates: []string{},
			want:      "",
		},
		{
			name:      "single non-empty",
			templates: []string{"only"},
			want:      "only",
		},
		{
			name:      "single empty",
			templates: []string{""},
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveTemplate(tt.templates...)
			assert.Equal(t, tt.want, got)
		})
	}
}
