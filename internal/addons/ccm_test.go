package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/config"
)

func TestBuildCCMValues(t *testing.T) {
	t.Parallel()
	// Helper to create bool pointer

	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name                string
		cfg                 *config.Config
		expectedClusterCIDR string
		checkEnvVars        bool
	}{
		{
			name: "default configuration",
			cfg: &config.Config{
				Location: "fsn1",
				Network: config.NetworkConfig{
					IPv4CIDR: "10.0.0.0/16",
				},
				Addons: config.AddonsConfig{
					CCM: config.CCMConfig{
						Enabled:              true,
						NetworkRoutesEnabled: boolPtr(true),
						LoadBalancers: config.CCMLoadBalancerConfig{
							Enabled:               boolPtr(true),
							Type:                  "lb11",
							Algorithm:             "least_connections",
							UsePrivateIP:          boolPtr(true),
							DisablePrivateIngress: boolPtr(true),
							DisablePublicNetwork:  boolPtr(false),
							DisableIPv6:           boolPtr(false),
							UsesProxyProtocol:     boolPtr(false),
							HealthCheck: config.CCMHealthCheckConfig{
								Interval: 3,
								Timeout:  3,
								Retries:  3,
							},
						},
					},
				},
			},
			expectedClusterCIDR: "10.0.0.0/16",
			checkEnvVars:        true,
		},
		{
			name: "with pod CIDR override",
			cfg: &config.Config{
				Location: "nbg1",
				Network: config.NetworkConfig{
					IPv4CIDR:    "10.0.0.0/16",
					PodIPv4CIDR: "10.244.0.0/16",
				},
				Addons: config.AddonsConfig{
					CCM: config.CCMConfig{
						Enabled: true,
						LoadBalancers: config.CCMLoadBalancerConfig{
							Enabled: boolPtr(true),
						},
					},
				},
			},
			expectedClusterCIDR: "10.244.0.0/16",
			checkEnvVars:        false,
		},
		{
			name: "with native routing CIDR",
			cfg: &config.Config{
				Location: "hel1",
				Network: config.NetworkConfig{
					IPv4CIDR:              "10.0.0.0/16",
					NativeRoutingIPv4CIDR: "10.96.0.0/12",
				},
				Addons: config.AddonsConfig{
					CCM: config.CCMConfig{
						Enabled: true,
						LoadBalancers: config.CCMLoadBalancerConfig{
							Enabled: boolPtr(true),
						},
					},
				},
			},
			expectedClusterCIDR: "10.96.0.0/12",
			checkEnvVars:        false,
		},
		{
			name: "custom load balancer settings",
			cfg: &config.Config{
				Location: "fsn1",
				Network: config.NetworkConfig{
					IPv4CIDR: "10.0.0.0/16",
				},
				Addons: config.AddonsConfig{
					CCM: config.CCMConfig{
						Enabled:              true,
						NetworkRoutesEnabled: boolPtr(false),
						LoadBalancers: config.CCMLoadBalancerConfig{
							Enabled:               boolPtr(true),
							Location:              "nbg1",
							Type:                  "lb21",
							Algorithm:             "round_robin",
							UsePrivateIP:          boolPtr(false),
							DisablePrivateIngress: boolPtr(false),
							DisablePublicNetwork:  boolPtr(true),
							DisableIPv6:           boolPtr(true),
							UsesProxyProtocol:     boolPtr(true),
							HealthCheck: config.CCMHealthCheckConfig{
								Interval: 5,
								Timeout:  10,
								Retries:  5,
							},
						},
					},
				},
			},
			expectedClusterCIDR: "10.0.0.0/16",
			checkEnvVars:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			values := buildCCMValues(tt.cfg)

			// Check kind
			assert.Equal(t, "DaemonSet", values["kind"])

			// Check node selector
			nodeSelector, ok := values["nodeSelector"].(helm.Values)
			require.True(t, ok)
			assert.Contains(t, nodeSelector, "node-role.kubernetes.io/control-plane")

			// Check networking configuration
			networking, ok := values["networking"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, true, networking["enabled"])
			assert.Equal(t, tt.expectedClusterCIDR, networking["clusterCIDR"])

			// Check network secret ref
			network, ok := networking["network"].(helm.Values)
			require.True(t, ok)
			valueFrom, ok := network["valueFrom"].(helm.Values)
			require.True(t, ok)
			secretKeyRef, ok := valueFrom["secretKeyRef"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, "hcloud", secretKeyRef["name"])
			assert.Equal(t, "network", secretKeyRef["key"])

			// Check env vars are present
			env, ok := values["env"].(helm.Values)
			require.True(t, ok, "env should be present in values")

			// Verify location is set (either custom or fallback to cluster location)
			location, ok := env["HCLOUD_LOAD_BALANCERS_LOCATION"].(helm.Values)
			require.True(t, ok)
			assert.NotEmpty(t, location["value"])
		})
	}
}

func TestBuildCCMEnvVars(t *testing.T) {
	t.Parallel()
	boolPtr := func(b bool) *bool { return &b }

	cfg := &config.Config{
		Location: "fsn1",
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{
				Enabled:              true,
				NetworkRoutesEnabled: boolPtr(true),
				LoadBalancers: config.CCMLoadBalancerConfig{
					Enabled:               boolPtr(true),
					Location:              "nbg1",
					Type:                  "lb21",
					Algorithm:             "round_robin",
					UsePrivateIP:          boolPtr(true),
					DisablePrivateIngress: boolPtr(true),
					DisablePublicNetwork:  boolPtr(false),
					DisableIPv6:           boolPtr(false),
					UsesProxyProtocol:     boolPtr(true),
					HealthCheck: config.CCMHealthCheckConfig{
						Interval: 5,
						Timeout:  10,
						Retries:  3,
					},
				},
			},
		},
	}

	env := buildCCMEnvVars(cfg, &cfg.Addons.CCM.LoadBalancers)

	// Check all expected env vars
	expectedVars := map[string]string{
		"HCLOUD_LOAD_BALANCERS_ENABLED":                 "true",
		"HCLOUD_LOAD_BALANCERS_LOCATION":                "nbg1",
		"HCLOUD_LOAD_BALANCERS_TYPE":                    "lb21",
		"HCLOUD_LOAD_BALANCERS_ALGORITHM_TYPE":          "round_robin",
		"HCLOUD_LOAD_BALANCERS_USE_PRIVATE_IP":          "true",
		"HCLOUD_LOAD_BALANCERS_DISABLE_PRIVATE_INGRESS": "true",
		"HCLOUD_LOAD_BALANCERS_DISABLE_PUBLIC_NETWORK":  "false",
		"HCLOUD_LOAD_BALANCERS_DISABLE_IPV6":            "false",
		"HCLOUD_LOAD_BALANCERS_USES_PROXYPROTOCOL":      "true",
		"HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_INTERVAL":   "5s",
		"HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_TIMEOUT":    "10s",
		"HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_RETRIES":    "3",
		"HCLOUD_NETWORK_ROUTES_ENABLED":                 "true",
	}

	for key, expectedValue := range expectedVars {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			envVar, ok := env[key].(helm.Values)
			require.True(t, ok, "env var %s should be present", key)
			assert.Equal(t, expectedValue, envVar["value"], "env var %s should have correct value", key)
		})
	}
}

func TestBuildCCMEnvVarsLocationFallback(t *testing.T) {
	t.Parallel()
	boolPtr := func(b bool) *bool { return &b }

	// Test that location falls back to cluster location when not set
	cfg := &config.Config{
		Location: "hel1",
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{
				Enabled: true,
				LoadBalancers: config.CCMLoadBalancerConfig{
					Enabled:  boolPtr(true),
					Location: "", // Not set - should fall back to cluster location
				},
			},
		},
	}

	env := buildCCMEnvVars(cfg, &cfg.Addons.CCM.LoadBalancers)

	location, ok := env["HCLOUD_LOAD_BALANCERS_LOCATION"].(helm.Values)
	require.True(t, ok)
	assert.Equal(t, "hel1", location["value"], "should fall back to cluster location")
}

func TestBuildCCMValues_Tolerations(t *testing.T) {
	t.Parallel()
	// Tolerations are critical for CCM to function properly.
	// Without them, CCM cannot schedule on control plane nodes due to taints,
	// creating a chicken-egg problem where CCM can't initialize nodes.
	// See: https://kubernetes.io/blog/2025/02/14/cloud-controller-manager-chicken-egg-problem/

	cfg := &config.Config{
		Location: "fsn1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{
				Enabled: true,
			},
		},
	}

	values := buildCCMValues(cfg)

	// Check tolerations are present (4 = BootstrapTolerations: control-plane, master, ccm-uninitialized, not-ready)
	tolerations, ok := values["tolerations"].([]helm.Values)
	require.True(t, ok, "tolerations must be present in CCM values")
	require.Len(t, tolerations, 4, "CCM should use BootstrapTolerations (4 entries)")

	// Verify control-plane toleration
	assert.Equal(t, "node-role.kubernetes.io/control-plane", tolerations[0]["key"])
	assert.Equal(t, "NoSchedule", tolerations[0]["effect"])
	assert.Equal(t, "Exists", tolerations[0]["operator"])

	// Verify master toleration (legacy)
	assert.Equal(t, "node-role.kubernetes.io/master", tolerations[1]["key"])

	// Verify uninitialized toleration (critical for bootstrap)
	assert.Equal(t, "node.cloudprovider.kubernetes.io/uninitialized", tolerations[2]["key"])
	assert.Equal(t, "true", tolerations[2]["value"])
	assert.Equal(t, "NoSchedule", tolerations[2]["effect"])

	// Verify not-ready toleration (helps during bootstrap)
	assert.Equal(t, "node.kubernetes.io/not-ready", tolerations[3]["key"])
	assert.Equal(t, "Exists", tolerations[3]["operator"])
}

func TestGetLBSubnetIPRange(t *testing.T) {
	t.Parallel()
	t.Run("returns subnet when config is valid", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{
			Network: config.NetworkConfig{
				IPv4CIDR: "10.0.0.0/16",
			},
		}
		_ = cfg.CalculateSubnets()
		result := getLBSubnetIPRange(cfg)
		assert.NotEmpty(t, result)
	})

	t.Run("returns empty on error", func(t *testing.T) {
		t.Parallel()
		// Empty config with no IPv4CIDR should cause GetSubnetForRole to fail
		cfg := &config.Config{}
		result := getLBSubnetIPRange(cfg)
		assert.Equal(t, "", result)
	})
}

func TestGetClusterCIDR(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cfg      *config.Config
		expected string
	}{
		{
			name: "uses pod CIDR when set",
			cfg: &config.Config{
				Network: config.NetworkConfig{
					IPv4CIDR:    "10.0.0.0/16",
					PodIPv4CIDR: "10.244.0.0/16",
				},
			},
			expected: "10.244.0.0/16",
		},
		{
			name: "uses native routing CIDR when pod CIDR not set",
			cfg: &config.Config{
				Network: config.NetworkConfig{
					IPv4CIDR:              "10.0.0.0/16",
					NativeRoutingIPv4CIDR: "10.96.0.0/12",
				},
			},
			expected: "10.96.0.0/12",
		},
		{
			name: "uses network CIDR as fallback",
			cfg: &config.Config{
				Network: config.NetworkConfig{
					IPv4CIDR: "10.0.0.0/16",
				},
			},
			expected: "10.0.0.0/16",
		},
		{
			name: "uses default when nothing set",
			cfg: &config.Config{
				Network: config.NetworkConfig{},
			},
			expected: "10.244.0.0/16",
		},
		{
			name: "pod CIDR takes precedence over native routing CIDR",
			cfg: &config.Config{
				Network: config.NetworkConfig{
					IPv4CIDR:              "10.0.0.0/16",
					PodIPv4CIDR:           "10.244.0.0/16",
					NativeRoutingIPv4CIDR: "10.96.0.0/12",
				},
			},
			expected: "10.244.0.0/16",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getClusterCIDR(tt.cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}
