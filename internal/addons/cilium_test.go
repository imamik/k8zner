package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hcloud-k8s/internal/config"
)

func TestBuildCiliumValues_Defaults(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR:              "10.0.0.0/16",
			NativeRoutingIPv4CIDR: "10.0.0.0/16",
		},
		Kubernetes: config.KubernetesConfig{
			CNI: config.CNIConfig{
				Enabled:              true,
				HelmVersion:          "1.18.5",
				KubeProxyReplacement: true,
				RoutingMode:          "native",
				BPFDatapathMode:      "veth",
				Encryption: config.CiliumEncryptionConfig{
					Enabled: true,
					Type:    "wireguard",
				},
			},
		},
	}

	values := buildCiliumValues(cfg, 3)

	// Verify core IPAM settings
	ipam := values["ipam"].(map[string]any)
	assert.Equal(t, "kubernetes", ipam["mode"])

	// Verify routing
	assert.Equal(t, "native", values["routingMode"])
	assert.Equal(t, "10.0.0.0/16", values["ipv4NativeRoutingCIDR"])

	// Verify KubePrism settings
	assert.Equal(t, kubePrismHost, values["k8sServiceHost"])
	assert.Equal(t, kubePrismPort, values["k8sServicePort"])

	// Verify kube-proxy replacement
	assert.Equal(t, true, values["kubeProxyReplacement"])
	assert.Equal(t, "0.0.0.0:10256", values["kubeProxyReplacementHealthzBindAddr"])
	assert.Equal(t, true, values["installNoConntrackIptablesRules"])

	// Verify BPF settings
	bpf := values["bpf"].(map[string]any)
	assert.Equal(t, true, bpf["masquerade"])
	assert.Equal(t, "veth", bpf["datapathMode"])
	assert.Equal(t, false, bpf["hostLegacyRouting"]) // WireGuard doesn't need legacy routing

	// Verify encryption
	encryption := values["encryption"].(map[string]any)
	assert.Equal(t, true, encryption["enabled"])
	assert.Equal(t, "wireguard", encryption["type"])

	// Verify Talos-specific cgroup settings
	cgroup := values["cgroup"].(map[string]any)
	autoMount := cgroup["autoMount"].(map[string]any)
	assert.Equal(t, false, autoMount["enabled"])
	assert.Equal(t, "/sys/fs/cgroup", cgroup["hostRoot"])

	// Verify operator settings
	operator := values["operator"].(map[string]any)
	assert.Equal(t, 2, operator["replicas"]) // 3 control plane nodes -> 2 replicas
	nodeSelector := operator["nodeSelector"].(map[string]string)
	assert.Equal(t, "", nodeSelector["node-role.kubernetes.io/control-plane"])
}

func TestBuildCiliumValues_IPSecEncryption(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Kubernetes: config.KubernetesConfig{
			CNI: config.CNIConfig{
				Enabled:              true,
				KubeProxyReplacement: true,
				RoutingMode:          "native",
				BPFDatapathMode:      "veth",
				Encryption: config.CiliumEncryptionConfig{
					Enabled: true,
					Type:    "ipsec",
				},
			},
		},
	}

	values := buildCiliumValues(cfg, 1)

	// Verify encryption type
	encryption := values["encryption"].(map[string]any)
	assert.Equal(t, true, encryption["enabled"])
	assert.Equal(t, "ipsec", encryption["type"])

	// Verify IPSec requires host legacy routing
	bpf := values["bpf"].(map[string]any)
	assert.Equal(t, true, bpf["hostLegacyRouting"])
}

func TestBuildCiliumValues_SingleControlPlane(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Kubernetes: config.KubernetesConfig{
			CNI: config.CNIConfig{
				Enabled: true,
			},
		},
	}

	values := buildCiliumValues(cfg, 1)

	// Verify operator replicas for single control plane
	operator := values["operator"].(map[string]any)
	assert.Equal(t, 1, operator["replicas"])
}

func TestBuildCiliumValues_HubbleEnabled(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Kubernetes: config.KubernetesConfig{
			CNI: config.CNIConfig{
				Enabled: true,
				Hubble: config.CiliumHubbleConfig{
					Enabled:      true,
					RelayEnabled: true,
					UIEnabled:    true,
				},
			},
		},
	}

	values := buildCiliumValues(cfg, 1)

	hubble := values["hubble"].(map[string]any)
	assert.Equal(t, true, hubble["enabled"])

	relay := hubble["relay"].(map[string]any)
	assert.Equal(t, true, relay["enabled"])

	ui := hubble["ui"].(map[string]any)
	assert.Equal(t, true, ui["enabled"])
}

func TestBuildCiliumValues_GatewayAPIEnabled(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Kubernetes: config.KubernetesConfig{
			CNI: config.CNIConfig{
				Enabled: true,
				GatewayAPI: config.CiliumGatewayAPIConfig{
					Enabled:               true,
					ExternalTrafficPolicy: "Local",
				},
			},
		},
	}

	values := buildCiliumValues(cfg, 1)

	gatewayAPI := values["gatewayAPI"].(map[string]any)
	assert.Equal(t, true, gatewayAPI["enabled"])
	assert.Equal(t, "Local", gatewayAPI["externalTrafficPolicy"])
	assert.Equal(t, "true", gatewayAPI["gatewayClass"].(map[string]any)["create"])
}

func TestBuildCiliumValues_TunnelMode(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Kubernetes: config.KubernetesConfig{
			CNI: config.CNIConfig{
				Enabled:              true,
				KubeProxyReplacement: true,
				RoutingMode:          "tunnel",
			},
		},
	}

	values := buildCiliumValues(cfg, 1)

	assert.Equal(t, "tunnel", values["routingMode"])
	// In tunnel mode with native routing disabled, installNoConntrackIptablesRules should be false
	assert.Equal(t, false, values["installNoConntrackIptablesRules"])
}

func TestBuildCiliumValues_ExtraHelmValues(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Kubernetes: config.KubernetesConfig{
			CNI: config.CNIConfig{
				Enabled: true,
				ExtraHelmValues: map[string]any{
					"debug": map[string]any{
						"enabled": true,
					},
					"ipam": map[string]any{
						"mode": "cluster-pool", // Override default
					},
				},
			},
		},
	}

	values := buildCiliumValues(cfg, 1)

	// Verify extra values are merged
	debug := values["debug"].(map[string]any)
	assert.Equal(t, true, debug["enabled"])

	// Verify override works
	ipam := values["ipam"].(map[string]any)
	assert.Equal(t, "cluster-pool", ipam["mode"])
}

func TestBuildCiliumValues_KubeProxyReplacementDisabled(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Kubernetes: config.KubernetesConfig{
			CNI: config.CNIConfig{
				Enabled:              true,
				KubeProxyReplacement: false,
				RoutingMode:          "native",
			},
		},
	}

	values := buildCiliumValues(cfg, 1)

	assert.Equal(t, false, values["kubeProxyReplacement"])
	assert.Equal(t, "", values["kubeProxyReplacementHealthzBindAddr"])

	bpf := values["bpf"].(map[string]any)
	assert.Equal(t, false, bpf["masquerade"])
}

func TestMergeMaps(t *testing.T) {
	base := map[string]any{
		"a": "base-a",
		"b": map[string]any{
			"b1": "base-b1",
			"b2": "base-b2",
		},
		"c": "base-c",
	}

	override := map[string]any{
		"a": "override-a",
		"b": map[string]any{
			"b2": "override-b2",
			"b3": "override-b3",
		},
		"d": "override-d",
	}

	result := mergeMaps(base, override)

	assert.Equal(t, "override-a", result["a"])
	assert.Equal(t, "base-c", result["c"])
	assert.Equal(t, "override-d", result["d"])

	resultB := result["b"].(map[string]any)
	assert.Equal(t, "base-b1", resultB["b1"])
	assert.Equal(t, "override-b2", resultB["b2"])
	assert.Equal(t, "override-b3", resultB["b3"])
}

func TestBuildCiliumValues_SecurityContext(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Kubernetes: config.KubernetesConfig{
			CNI: config.CNIConfig{
				Enabled: true,
			},
		},
	}

	values := buildCiliumValues(cfg, 1)

	securityContext := values["securityContext"].(map[string]any)
	capabilities := securityContext["capabilities"].(map[string]any)

	ciliumAgentCaps := capabilities["ciliumAgent"].([]string)
	require.Contains(t, ciliumAgentCaps, "NET_ADMIN")
	require.Contains(t, ciliumAgentCaps, "SYS_ADMIN")

	cleanCiliumStateCaps := capabilities["cleanCiliumState"].([]string)
	require.Contains(t, cleanCiliumStateCaps, "NET_ADMIN")
	require.Contains(t, cleanCiliumStateCaps, "SYS_ADMIN")
}
