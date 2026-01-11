package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateLocations_ValidLocation(t *testing.T) {
	for location := range ValidLocations {
		cfg := &Config{
			ClusterName: "test",
			HCloudToken: "token",
			Location:    location,
			Network: NetworkConfig{
				IPv4CIDR: "10.0.0.0/16",
				Zone:     "eu-central",
			},
			ControlPlane: ControlPlaneConfig{
				NodePools: []ControlPlaneNodePool{{
					Name:       "cp",
					ServerType: "cpx22",
					Count:      1,
				}},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err, "location %q should be valid", location)
	}
}

func TestValidateLocations_InvalidLocation(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "invalid-location",
		Network: NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{
				Name:       "cp",
				ServerType: "cpx22",
				Count:      1,
			}},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid location")
	assert.Contains(t, err.Error(), "invalid-location")
}

func TestValidateLocations_ValidNetworkZone(t *testing.T) {
	for zone := range ValidNetworkZones {
		cfg := &Config{
			ClusterName: "test",
			HCloudToken: "token",
			Location:    "nbg1",
			Network: NetworkConfig{
				IPv4CIDR: "10.0.0.0/16",
				Zone:     zone,
			},
			ControlPlane: ControlPlaneConfig{
				NodePools: []ControlPlaneNodePool{{
					Name:       "cp",
					ServerType: "cpx22",
					Count:      1,
				}},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err, "network zone %q should be valid", zone)
	}
}

func TestValidateLocations_InvalidNetworkZone(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network: NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "invalid-zone",
		},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{
				Name:       "cp",
				ServerType: "cpx22",
				Count:      1,
			}},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid network zone")
	assert.Contains(t, err.Error(), "invalid-zone")
}

func TestValidateLocations_InvalidControlPlanePoolLocation(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network: NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{
				Name:       "cp",
				ServerType: "cpx22",
				Count:      1,
				Location:   "bad-location",
			}},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "control plane pool")
	assert.Contains(t, err.Error(), "invalid location")
	assert.Contains(t, err.Error(), "bad-location")
}

func TestValidateLocations_InvalidWorkerPoolLocation(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network: NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{
				Name:       "cp",
				ServerType: "cpx22",
				Count:      1,
			}},
		},
		Workers: []WorkerNodePool{{
			Name:       "worker",
			ServerType: "cpx22",
			Count:      1,
			Location:   "bad-location",
		}},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker pool")
	assert.Contains(t, err.Error(), "invalid location")
	assert.Contains(t, err.Error(), "bad-location")
}

func TestValidateLocations_LocationAndZoneBothValid(t *testing.T) {
	// Test valid combinations of location and zone
	testCases := []struct {
		location string
		zone     string
	}{
		{"nbg1", "eu-central"},
		{"fsn1", "eu-central"},
		{"hel1", "eu-central"},
		{"ash", "us-east"},
		{"hil", "us-west"},
		{"sin", "ap-southeast"},
	}

	for _, tc := range testCases {
		cfg := &Config{
			ClusterName: "test",
			HCloudToken: "token",
			Location:    tc.location,
			Network: NetworkConfig{
				IPv4CIDR: "10.0.0.0/16",
				Zone:     tc.zone,
			},
			ControlPlane: ControlPlaneConfig{
				NodePools: []ControlPlaneNodePool{{
					Name:       "cp",
					ServerType: "cpx22",
					Count:      1,
				}},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err, "location=%q zone=%q should be valid", tc.location, tc.zone)
	}
}

// CNI Validation Tests

func newValidConfigWithCNI(cni CNIConfig) *Config {
	return &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network: NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{
				Name:       "cp",
				ServerType: "cpx22",
				Count:      1,
			}},
		},
		Kubernetes: KubernetesConfig{
			CNI: cni,
		},
	}
}

func TestValidateCNI_Disabled(t *testing.T) {
	cfg := newValidConfigWithCNI(CNIConfig{
		Enabled: false,
	})
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidateCNI_ValidRoutingModes(t *testing.T) {
	validModes := []string{"native", "tunnel"}

	for _, mode := range validModes {
		cfg := newValidConfigWithCNI(CNIConfig{
			Enabled:     true,
			RoutingMode: mode,
		})
		err := cfg.Validate()
		assert.NoError(t, err, "routing mode %q should be valid", mode)
	}
}

func TestValidateCNI_InvalidRoutingMode(t *testing.T) {
	cfg := newValidConfigWithCNI(CNIConfig{
		Enabled:     true,
		RoutingMode: "invalid",
	})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid routing_mode")
}

func TestValidateCNI_ValidBPFDatapathModes(t *testing.T) {
	validModes := []string{"veth", "netkit", "netkit-l2"}

	for _, mode := range validModes {
		cfg := newValidConfigWithCNI(CNIConfig{
			Enabled:         true,
			BPFDatapathMode: mode,
		})
		err := cfg.Validate()
		assert.NoError(t, err, "BPF datapath mode %q should be valid", mode)
	}
}

func TestValidateCNI_InvalidBPFDatapathMode(t *testing.T) {
	cfg := newValidConfigWithCNI(CNIConfig{
		Enabled:         true,
		BPFDatapathMode: "invalid",
	})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bpf_datapath_mode")
}

func TestValidateCNI_ValidEncryptionTypes(t *testing.T) {
	validTypes := []string{"wireguard", "ipsec"}

	for _, encType := range validTypes {
		cfg := newValidConfigWithCNI(CNIConfig{
			Enabled: true,
			Encryption: CiliumEncryptionConfig{
				Enabled: true,
				Type:    encType,
			},
		})
		err := cfg.Validate()
		assert.NoError(t, err, "encryption type %q should be valid", encType)
	}
}

func TestValidateCNI_InvalidEncryptionType(t *testing.T) {
	cfg := newValidConfigWithCNI(CNIConfig{
		Enabled: true,
		Encryption: CiliumEncryptionConfig{
			Enabled: true,
			Type:    "invalid",
		},
	})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid encryption type")
}

func TestValidateCNI_IPSecValidKeySizes(t *testing.T) {
	validSizes := []int{128, 192, 256}

	for _, size := range validSizes {
		cfg := newValidConfigWithCNI(CNIConfig{
			Enabled: true,
			Encryption: CiliumEncryptionConfig{
				Enabled: true,
				Type:    "ipsec",
				IPSec: CiliumIPSecConfig{
					KeySize: size,
				},
			},
		})
		err := cfg.Validate()
		assert.NoError(t, err, "IPSec key size %d should be valid", size)
	}
}

func TestValidateCNI_IPSecInvalidKeySize(t *testing.T) {
	cfg := newValidConfigWithCNI(CNIConfig{
		Enabled: true,
		Encryption: CiliumEncryptionConfig{
			Enabled: true,
			Type:    "ipsec",
			IPSec: CiliumIPSecConfig{
				KeySize: 512,
			},
		},
	})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid IPSec key_size")
}

func TestValidateCNI_IPSecValidKeyIDs(t *testing.T) {
	for keyID := 1; keyID <= 15; keyID++ {
		cfg := newValidConfigWithCNI(CNIConfig{
			Enabled: true,
			Encryption: CiliumEncryptionConfig{
				Enabled: true,
				Type:    "ipsec",
				IPSec: CiliumIPSecConfig{
					KeyID: keyID,
				},
			},
		})
		err := cfg.Validate()
		assert.NoError(t, err, "IPSec key ID %d should be valid", keyID)
	}
}

func TestValidateCNI_IPSecInvalidKeyID(t *testing.T) {
	invalidIDs := []int{16, 20, 100}

	for _, keyID := range invalidIDs {
		cfg := newValidConfigWithCNI(CNIConfig{
			Enabled: true,
			Encryption: CiliumEncryptionConfig{
				Enabled: true,
				Type:    "ipsec",
				IPSec: CiliumIPSecConfig{
					KeyID: keyID,
				},
			},
		})
		err := cfg.Validate()
		require.Error(t, err, "IPSec key ID %d should be invalid", keyID)
		assert.Contains(t, err.Error(), "invalid IPSec key_id")
	}
}

func TestValidateCNI_IPSecIncompatibleWithNetkit(t *testing.T) {
	netkitModes := []string{"netkit", "netkit-l2"}

	for _, mode := range netkitModes {
		cfg := newValidConfigWithCNI(CNIConfig{
			Enabled:         true,
			BPFDatapathMode: mode,
			Encryption: CiliumEncryptionConfig{
				Enabled: true,
				Type:    "ipsec",
			},
		})
		err := cfg.Validate()
		require.Error(t, err, "IPSec should be incompatible with %s", mode)
		assert.Contains(t, err.Error(), "not compatible")
	}
}

func TestValidateCNI_WireGuardCompatibleWithNetkit(t *testing.T) {
	cfg := newValidConfigWithCNI(CNIConfig{
		Enabled:         true,
		BPFDatapathMode: "netkit",
		Encryption: CiliumEncryptionConfig{
			Enabled: true,
			Type:    "wireguard",
		},
	})
	err := cfg.Validate()
	assert.NoError(t, err, "WireGuard should be compatible with netkit")
}

func TestValidateCNI_HubbleUIRequiresRelay(t *testing.T) {
	cfg := newValidConfigWithCNI(CNIConfig{
		Enabled: true,
		Hubble: CiliumHubbleConfig{
			Enabled:      true,
			UIEnabled:    true,
			RelayEnabled: false,
		},
	})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hubble.ui_enabled requires hubble.relay_enabled")
}

func TestValidateCNI_HubbleRelayRequiresHubble(t *testing.T) {
	cfg := newValidConfigWithCNI(CNIConfig{
		Enabled: true,
		Hubble: CiliumHubbleConfig{
			Enabled:      false,
			RelayEnabled: true,
		},
	})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hubble.relay_enabled requires hubble.enabled")
}

func TestValidateCNI_HubbleFullStackValid(t *testing.T) {
	cfg := newValidConfigWithCNI(CNIConfig{
		Enabled: true,
		Hubble: CiliumHubbleConfig{
			Enabled:      true,
			RelayEnabled: true,
			UIEnabled:    true,
		},
	})
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidateCNI_ValidGatewayAPITrafficPolicies(t *testing.T) {
	validPolicies := []string{"Cluster", "Local"}

	for _, policy := range validPolicies {
		cfg := newValidConfigWithCNI(CNIConfig{
			Enabled: true,
			GatewayAPI: CiliumGatewayAPIConfig{
				Enabled:               true,
				ExternalTrafficPolicy: policy,
			},
		})
		err := cfg.Validate()
		assert.NoError(t, err, "Gateway API traffic policy %q should be valid", policy)
	}
}

func TestValidateCNI_InvalidGatewayAPITrafficPolicy(t *testing.T) {
	cfg := newValidConfigWithCNI(CNIConfig{
		Enabled: true,
		GatewayAPI: CiliumGatewayAPIConfig{
			Enabled:               true,
			ExternalTrafficPolicy: "Invalid",
		},
	})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid gateway_api.external_traffic_policy")
}
