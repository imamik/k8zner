package addons

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/config"
)

func TestBuildCiliumValues(t *testing.T) {
	tests := []struct {
		name                     string
		config                   *config.Config
		expectedRoutingMode      string
		expectedEncryptionType   string
		expectedKubeProxyRepl    bool
		expectedHubbleEnabled    bool
		expectedOperatorReplicas int
	}{
		{
			name: "single control plane with wireguard",
			config: &config.Config{
				Network: config.NetworkConfig{
					IPv4CIDR: "10.0.0.0/8",
				},
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 1},
					},
				},
				Addons: config.AddonsConfig{
					Cilium: config.CiliumConfig{
						Enabled:                     true,
						EncryptionEnabled:           true,
						EncryptionType:              "wireguard",
						RoutingMode:                 "native",
						KubeProxyReplacementEnabled: true,
					},
				},
			},
			expectedRoutingMode:      "native",
			expectedEncryptionType:   "wireguard",
			expectedKubeProxyRepl:    true,
			expectedHubbleEnabled:    false,
			expectedOperatorReplicas: 1,
		},
		{
			name: "HA control plane with ipsec and hubble",
			config: &config.Config{
				Network: config.NetworkConfig{
					IPv4CIDR: "10.0.0.0/8",
				},
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 3},
					},
				},
				Addons: config.AddonsConfig{
					Cilium: config.CiliumConfig{
						Enabled:                     true,
						EncryptionEnabled:           true,
						EncryptionType:              "ipsec",
						RoutingMode:                 "native",
						KubeProxyReplacementEnabled: true,
						HubbleEnabled:               true,
						HubbleRelayEnabled:          true,
					},
				},
			},
			expectedRoutingMode:      "native",
			expectedEncryptionType:   "ipsec",
			expectedKubeProxyRepl:    true,
			expectedHubbleEnabled:    true,
			expectedOperatorReplicas: 2,
		},
		{
			name: "no encryption with gateway API",
			config: &config.Config{
				Network: config.NetworkConfig{
					IPv4CIDR: "10.0.0.0/8",
				},
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 1},
					},
				},
				Addons: config.AddonsConfig{
					Cilium: config.CiliumConfig{
						Enabled:           true,
						EncryptionEnabled: false,
						RoutingMode:       "tunnel",
						GatewayAPIEnabled: true,
					},
				},
			},
			expectedRoutingMode:      "tunnel",
			expectedEncryptionType:   "",
			expectedKubeProxyRepl:    false,
			expectedHubbleEnabled:    false,
			expectedOperatorReplicas: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := buildCiliumValues(tt.config)

			// Check routing mode
			assert.Equal(t, tt.expectedRoutingMode, values["routingMode"])

			// Check encryption
			encryption := values["encryption"].(helm.Values)
			assert.Equal(t, tt.config.Addons.Cilium.EncryptionEnabled, encryption["enabled"])
			if tt.config.Addons.Cilium.EncryptionEnabled {
				assert.Equal(t, tt.expectedEncryptionType, encryption["type"])
			}

			// Check kube-proxy replacement
			assert.Equal(t, tt.expectedKubeProxyRepl, values["kubeProxyReplacement"])

			// Check BPF settings
			bpf := values["bpf"].(helm.Values)
			assert.Equal(t, tt.expectedKubeProxyRepl, bpf["masquerade"])
			if tt.config.Addons.Cilium.EncryptionEnabled && tt.config.Addons.Cilium.EncryptionType == "ipsec" {
				assert.True(t, bpf["hostLegacyRouting"].(bool))
			}

			// Check K8s service settings
			assert.Equal(t, "127.0.0.1", values["k8sServiceHost"])
			assert.Equal(t, 7445, values["k8sServicePort"])

			// Check Hubble
			if tt.expectedHubbleEnabled {
				hubble, ok := values["hubble"]
				assert.True(t, ok)
				assert.NotNil(t, hubble)
			}

			// Check Gateway API
			if tt.config.Addons.Cilium.GatewayAPIEnabled {
				gatewayAPI := values["gatewayAPI"].(helm.Values)
				assert.True(t, gatewayAPI["enabled"].(bool))
			}

			// Check operator
			operator := values["operator"].(helm.Values)
			assert.Equal(t, tt.expectedOperatorReplicas, operator["replicas"])
		})
	}
}

func TestBuildCiliumOperatorConfig(t *testing.T) {
	tests := []struct {
		name              string
		controlPlaneCount int
		expectedReplicas  int
		expectPDB         bool
		expectTSC         bool
	}{
		{
			name:              "single control plane",
			controlPlaneCount: 1,
			expectedReplicas:  1,
			expectPDB:         false,
			expectTSC:         false,
		},
		{
			name:              "HA control plane",
			controlPlaneCount: 3,
			expectedReplicas:  2,
			expectPDB:         true,
			expectTSC:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciliumCfg := config.CiliumConfig{}
			config := buildCiliumOperatorConfig(ciliumCfg, tt.controlPlaneCount)

			assert.Equal(t, tt.expectedReplicas, config["replicas"])

			// Check node selector
			nodeSelector := config["nodeSelector"].(helm.Values)
			assert.Contains(t, nodeSelector, "node-role.kubernetes.io/control-plane")

			// Check tolerations (should have control-plane, master, not-ready, and CCM uninitialized)
			tolerations := config["tolerations"].([]helm.Values)
			assert.Len(t, tolerations, 4)
			assert.Equal(t, "node-role.kubernetes.io/control-plane", tolerations[0]["key"])
			assert.Equal(t, "node-role.kubernetes.io/master", tolerations[1]["key"])
			assert.Equal(t, "node.kubernetes.io/not-ready", tolerations[2]["key"])
			assert.Equal(t, "node.cloudprovider.kubernetes.io/uninitialized", tolerations[3]["key"])

			// Check PDB
			if tt.expectPDB {
				pdb, ok := config["podDisruptionBudget"]
				assert.True(t, ok)
				pdbMap := pdb.(helm.Values)
				assert.True(t, pdbMap["enabled"].(bool))
				assert.Equal(t, 1, pdbMap["maxUnavailable"])
			} else {
				_, ok := config["podDisruptionBudget"]
				assert.False(t, ok)
			}

			// Check topology spread constraints
			if tt.expectTSC {
				tsc, ok := config["topologySpreadConstraints"]
				assert.True(t, ok)
				tscList := tsc.([]helm.Values)
				assert.Len(t, tscList, 2) // hostname and zone
			} else {
				_, ok := config["topologySpreadConstraints"]
				assert.False(t, ok)
			}
		})
	}
}

func TestBuildCiliumHubbleConfig(t *testing.T) {
	tests := []struct {
		name          string
		hubbleEnabled bool
		relayEnabled  bool
		uiEnabled     bool
		expectRelay   bool
		expectUI      bool
	}{
		{
			name:          "hubble disabled",
			hubbleEnabled: false,
			relayEnabled:  false,
			uiEnabled:     false,
			expectRelay:   false,
			expectUI:      false,
		},
		{
			name:          "hubble enabled without relay or UI",
			hubbleEnabled: true,
			relayEnabled:  false,
			uiEnabled:     false,
			expectRelay:   false,
			expectUI:      false,
		},
		{
			name:          "hubble with relay",
			hubbleEnabled: true,
			relayEnabled:  true,
			uiEnabled:     false,
			expectRelay:   true,
			expectUI:      false,
		},
		{
			name:          "hubble with relay and UI",
			hubbleEnabled: true,
			relayEnabled:  true,
			uiEnabled:     true,
			expectRelay:   true,
			expectUI:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Addons: config.AddonsConfig{
					Cilium: config.CiliumConfig{
						HubbleEnabled:      tt.hubbleEnabled,
						HubbleRelayEnabled: tt.relayEnabled,
						HubbleUIEnabled:    tt.uiEnabled,
					},
				},
			}

			hubbleConfig := buildCiliumHubbleConfig(cfg)

			assert.True(t, hubbleConfig["enabled"].(bool))

			if tt.expectRelay {
				relay := hubbleConfig["relay"].(helm.Values)
				assert.True(t, relay["enabled"].(bool))
			}

			if tt.expectUI {
				ui := hubbleConfig["ui"].(helm.Values)
				assert.True(t, ui["enabled"].(bool))
			}
		})
	}
}

func TestBuildCiliumIPSecSecret(t *testing.T) {
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				IPSecKeySize:   128,
				IPSecKeyID:     1,
				IPSecAlgorithm: "rfc4106(gcm(aes))",
			},
		},
	}

	secretYAML, err := buildCiliumIPSecSecret(cfg)
	require.NoError(t, err)

	// Parse the YAML
	var secret map[string]any
	err = yaml.Unmarshal([]byte(secretYAML), &secret)
	require.NoError(t, err)

	// Check structure
	assert.Equal(t, "v1", secret["apiVersion"])
	assert.Equal(t, "Secret", secret["kind"])
	assert.Equal(t, "Opaque", secret["type"])

	// Check metadata
	metadata := secret["metadata"].(map[string]any)
	assert.Equal(t, "cilium-ipsec-keys", metadata["name"])
	assert.Equal(t, "kube-system", metadata["namespace"])

	// Check annotations
	annotations := metadata["annotations"].(map[string]any)
	assert.Equal(t, "1", annotations["cilium.io/key-id"])
	assert.Equal(t, "rfc4106(gcm(aes))", annotations["cilium.io/key-algorithm"])
	assert.Equal(t, "128", annotations["cilium.io/key-size"])

	// Check data exists
	data := secret["data"].(map[string]any)
	assert.NotEmpty(t, data["keys"])
}

func TestBuildCiliumIPSecSecretDefaults(t *testing.T) {
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				// No explicit values, should use defaults
			},
		},
	}

	secretYAML, err := buildCiliumIPSecSecret(cfg)
	require.NoError(t, err)

	var secret map[string]any
	err = yaml.Unmarshal([]byte(secretYAML), &secret)
	require.NoError(t, err)

	metadata := secret["metadata"].(map[string]any)
	annotations := metadata["annotations"].(map[string]any)

	// Check defaults
	assert.Equal(t, "1", annotations["cilium.io/key-id"])
	assert.Equal(t, "rfc4106(gcm(aes))", annotations["cilium.io/key-algorithm"])
	assert.Equal(t, "128", annotations["cilium.io/key-size"])
}

func TestGenerateIPSecKey(t *testing.T) {
	tests := []struct {
		name           string
		keySize        int
		expectedLength int // hex string length: (keySize/8 + 4) * 2
	}{
		{
			name:           "128-bit key",
			keySize:        128,
			expectedLength: (128/8 + 4) * 2,
		},
		{
			name:           "192-bit key",
			keySize:        192,
			expectedLength: (192/8 + 4) * 2,
		},
		{
			name:           "256-bit key",
			keySize:        256,
			expectedLength: (256/8 + 4) * 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := generateIPSecKey(tt.keySize)
			require.NoError(t, err)

			assert.Len(t, key, tt.expectedLength)

			// Verify it's valid hex
			_, err = hex.DecodeString(key)
			assert.NoError(t, err)
		})
	}
}

func TestGenerateIPSecKeyRandomness(t *testing.T) {
	// Generate multiple keys and ensure they're all different
	keys := make(map[string]bool)
	for i := 0; i < 10; i++ {
		key, err := generateIPSecKey(128)
		require.NoError(t, err)
		keys[key] = true
	}

	// All keys should be unique
	assert.Len(t, keys, 10)
}

func TestBuildCiliumValuesNativeRoutingCIDR(t *testing.T) {
	tests := []struct {
		name                  string
		networkCIDR           string
		nativeRoutingCIDR     string
		expectedNativeRouting string
	}{
		{
			name:                  "use network CIDR when no explicit native routing CIDR",
			networkCIDR:           "10.0.0.0/8",
			nativeRoutingCIDR:     "",
			expectedNativeRouting: "10.0.0.0/8",
		},
		{
			name:                  "use explicit native routing CIDR when provided",
			networkCIDR:           "10.0.0.0/8",
			nativeRoutingCIDR:     "10.0.0.0/16",
			expectedNativeRouting: "10.0.0.0/16",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Network: config.NetworkConfig{
					IPv4CIDR:              tt.networkCIDR,
					NativeRoutingIPv4CIDR: tt.nativeRoutingCIDR,
				},
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{{Count: 1}},
				},
				Addons: config.AddonsConfig{
					Cilium: config.CiliumConfig{
						Enabled: true,
					},
				},
			}

			values := buildCiliumValues(cfg)
			assert.Equal(t, tt.expectedNativeRouting, values["ipv4NativeRoutingCIDR"])
		})
	}
}

func TestBuildCiliumValuesKubeProxyReplacement(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/8",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:                     true,
				RoutingMode:                 "native",
				KubeProxyReplacementEnabled: true,
			},
		},
	}

	values := buildCiliumValues(cfg)

	// Check kube-proxy replacement settings
	assert.True(t, values["kubeProxyReplacement"].(bool))
	assert.Equal(t, "0.0.0.0:10256", values["kubeProxyReplacementHealthzBindAddr"])
	assert.True(t, values["installNoConntrackIptablesRules"].(bool))

	// Check BPF masquerade
	bpf := values["bpf"].(helm.Values)
	assert.True(t, bpf["masquerade"].(bool))
}

func TestBuildCiliumValuesBPFDatapathMode(t *testing.T) {
	tests := []struct {
		name         string
		datapathMode string
		expected     string
	}{
		{
			name:         "default to veth",
			datapathMode: "",
			expected:     "veth",
		},
		{
			name:         "explicit veth",
			datapathMode: "veth",
			expected:     "veth",
		},
		{
			name:         "netkit mode",
			datapathMode: "netkit",
			expected:     "netkit",
		},
		{
			name:         "netkit-l2 mode",
			datapathMode: "netkit-l2",
			expected:     "netkit-l2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Network: config.NetworkConfig{IPv4CIDR: "10.0.0.0/8"},
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{{Count: 1}},
				},
				Addons: config.AddonsConfig{
					Cilium: config.CiliumConfig{
						Enabled:         true,
						BPFDatapathMode: tt.datapathMode,
					},
				},
			}

			values := buildCiliumValues(cfg)
			bpf := values["bpf"].(helm.Values)
			assert.Equal(t, tt.expected, bpf["datapathMode"])
		})
	}
}

func TestBuildCiliumValuesPolicyCIDRMatchMode(t *testing.T) {
	tests := []struct {
		name          string
		matchMode     string
		expectedValue any
	}{
		{
			name:          "disabled by default",
			matchMode:     "",
			expectedValue: "",
		},
		{
			name:          "nodes mode",
			matchMode:     "nodes",
			expectedValue: []string{"nodes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Network: config.NetworkConfig{IPv4CIDR: "10.0.0.0/8"},
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{{Count: 1}},
				},
				Addons: config.AddonsConfig{
					Cilium: config.CiliumConfig{
						Enabled:             true,
						PolicyCIDRMatchMode: tt.matchMode,
					},
				},
			}

			values := buildCiliumValues(cfg)
			assert.Equal(t, tt.expectedValue, values["policyCIDRMatchMode"])
		})
	}
}

func TestBuildCiliumValuesSocketLBHostNamespaceOnly(t *testing.T) {
	tests := []struct {
		name              string
		hostNamespaceOnly bool
	}{
		{
			name:              "disabled",
			hostNamespaceOnly: false,
		},
		{
			name:              "enabled",
			hostNamespaceOnly: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Network: config.NetworkConfig{IPv4CIDR: "10.0.0.0/8"},
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{{Count: 1}},
				},
				Addons: config.AddonsConfig{
					Cilium: config.CiliumConfig{
						Enabled:                   true,
						SocketLBHostNamespaceOnly: tt.hostNamespaceOnly,
					},
				},
			}

			values := buildCiliumValues(cfg)
			socketLB := values["socketLB"].(helm.Values)
			assert.Equal(t, tt.hostNamespaceOnly, socketLB["hostNamespaceOnly"])
		})
	}
}

func TestBuildCiliumGatewayAPIConfig(t *testing.T) {
	tests := []struct {
		name                    string
		proxyProtocolEnabled    *bool
		externalTrafficPolicy   string
		expectedProxyProtocol   bool
		expectedExternalTraffic string
	}{
		{
			name:                    "defaults",
			proxyProtocolEnabled:    nil,
			externalTrafficPolicy:   "",
			expectedProxyProtocol:   true,
			expectedExternalTraffic: "Cluster",
		},
		{
			name:                    "proxy protocol disabled",
			proxyProtocolEnabled:    boolPtr(false),
			externalTrafficPolicy:   "",
			expectedProxyProtocol:   false,
			expectedExternalTraffic: "Cluster",
		},
		{
			name:                    "local traffic policy",
			proxyProtocolEnabled:    nil,
			externalTrafficPolicy:   "Local",
			expectedProxyProtocol:   true,
			expectedExternalTraffic: "Local",
		},
		{
			name:                    "custom settings",
			proxyProtocolEnabled:    boolPtr(true),
			externalTrafficPolicy:   "Local",
			expectedProxyProtocol:   true,
			expectedExternalTraffic: "Local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciliumCfg := config.CiliumConfig{
				GatewayAPIEnabled:               true,
				GatewayAPIProxyProtocolEnabled:  tt.proxyProtocolEnabled,
				GatewayAPIExternalTrafficPolicy: tt.externalTrafficPolicy,
			}

			values := buildCiliumGatewayAPIConfig(ciliumCfg)

			assert.True(t, values["enabled"].(bool))
			assert.Equal(t, tt.expectedProxyProtocol, values["enableProxyProtocol"])
			assert.Equal(t, tt.expectedExternalTraffic, values["externalTrafficPolicy"])
			assert.True(t, values["enableAppProtocol"].(bool))
			assert.True(t, values["enableAlpn"].(bool))
		})
	}
}

func TestBuildCiliumPrometheusConfig(t *testing.T) {
	tests := []struct {
		name                  string
		serviceMonitorEnabled bool
	}{
		{
			name:                  "service monitor disabled",
			serviceMonitorEnabled: false,
		},
		{
			name:                  "service monitor enabled",
			serviceMonitorEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciliumCfg := config.CiliumConfig{
				ServiceMonitorEnabled: tt.serviceMonitorEnabled,
			}

			values := buildCiliumPrometheusConfig(ciliumCfg)

			assert.True(t, values["enabled"].(bool))

			serviceMonitor := values["serviceMonitor"].(helm.Values)
			assert.Equal(t, tt.serviceMonitorEnabled, serviceMonitor["enabled"])
			assert.Equal(t, tt.serviceMonitorEnabled, serviceMonitor["trustCRDsExist"])
			assert.Equal(t, "15s", serviceMonitor["interval"])
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
