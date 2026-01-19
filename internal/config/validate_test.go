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

// Test ClusterAccess validation
func TestValidateClusterAccess_Valid(t *testing.T) {
	for mode := range ValidClusterAccessModes {
		cfg := &Config{
			ClusterName:   "test",
			HCloudToken:   "token",
			Location:      "nbg1",
			ClusterAccess: mode,
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
		assert.NoError(t, err, "cluster_access %q should be valid", mode)
	}
}

func TestValidateClusterAccess_Invalid(t *testing.T) {
	cfg := &Config{
		ClusterName:   "test",
		HCloudToken:   "token",
		Location:      "nbg1",
		ClusterAccess: "invalid-mode",
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid cluster_access")
	assert.Contains(t, err.Error(), "invalid-mode")
}

// Test IngressLoadBalancerPools validation
func TestValidateIngressLoadBalancerPools_Valid(t *testing.T) {
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
		IngressLoadBalancerPools: []IngressLoadBalancerPool{
			{
				Name:      "internal",
				Location:  "nbg1",
				Type:      "lb11",
				Algorithm: "round_robin",
				Count:     2,
			},
			{
				Name:      "external",
				Location:  "fsn1",
				Type:      "lb21",
				Algorithm: "least_connections",
			},
		},
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidateIngressLoadBalancerPools_MissingName(t *testing.T) {
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
		IngressLoadBalancerPools: []IngressLoadBalancerPool{
			{Location: "nbg1"},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestValidateIngressLoadBalancerPools_DuplicateName(t *testing.T) {
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
		IngressLoadBalancerPools: []IngressLoadBalancerPool{
			{Name: "pool1", Location: "nbg1"},
			{Name: "pool1", Location: "fsn1"},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate name")
}

func TestValidateIngressLoadBalancerPools_InvalidLocation(t *testing.T) {
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
		IngressLoadBalancerPools: []IngressLoadBalancerPool{
			{Name: "pool1", Location: "invalid-loc"},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid location")
}

func TestValidateIngressLoadBalancerPools_InvalidType(t *testing.T) {
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
		IngressLoadBalancerPools: []IngressLoadBalancerPool{
			{Name: "pool1", Location: "nbg1", Type: "invalid-type"},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
}

func TestValidateIngressLoadBalancerPools_InvalidAlgorithm(t *testing.T) {
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
		IngressLoadBalancerPools: []IngressLoadBalancerPool{
			{Name: "pool1", Location: "nbg1", Algorithm: "random"},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid algorithm")
}

// Test IngressNginx validation
func TestValidateIngressNginx_ValidKinds(t *testing.T) {
	for kind := range ValidIngressNginxKinds {
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
			Addons: AddonsConfig{
				IngressNginx: IngressNginxConfig{
					Enabled: true,
					Kind:    kind,
				},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err, "kind %q should be valid", kind)
	}
}

func TestValidateIngressNginx_InvalidKind(t *testing.T) {
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
		Addons: AddonsConfig{
			IngressNginx: IngressNginxConfig{
				Enabled: true,
				Kind:    "StatefulSet",
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid kind")
}

func TestValidateIngressNginx_ReplicasWithDaemonSet(t *testing.T) {
	replicas := 3
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
		Addons: AddonsConfig{
			IngressNginx: IngressNginxConfig{
				Enabled:  true,
				Kind:     "DaemonSet",
				Replicas: &replicas,
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "replicas must not be set when kind is 'DaemonSet'")
}

func TestValidateIngressNginx_InvalidExternalTrafficPolicy(t *testing.T) {
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
		Addons: AddonsConfig{
			IngressNginx: IngressNginxConfig{
				Enabled:               true,
				ExternalTrafficPolicy: "Invalid",
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid external_traffic_policy")
}

// Test Cilium validation
func TestValidateCilium_ValidBPFDatapathModes(t *testing.T) {
	for mode := range ValidCiliumBPFDatapathModes {
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
			Addons: AddonsConfig{
				Cilium: CiliumConfig{
					Enabled:         true,
					BPFDatapathMode: mode,
				},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err, "bpf_datapath_mode %q should be valid", mode)
	}
}

func TestValidateCilium_InvalidBPFDatapathMode(t *testing.T) {
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
		Addons: AddonsConfig{
			Cilium: CiliumConfig{
				Enabled:         true,
				BPFDatapathMode: "invalid-mode",
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bpf_datapath_mode")
}

func TestValidateCilium_ValidPolicyCIDRMatchModes(t *testing.T) {
	for mode := range ValidCiliumPolicyCIDRMatchModes {
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
			Addons: AddonsConfig{
				Cilium: CiliumConfig{
					Enabled:             true,
					PolicyCIDRMatchMode: mode,
				},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err, "policy_cidr_match_mode %q should be valid", mode)
	}
}

func TestValidateCilium_InvalidPolicyCIDRMatchMode(t *testing.T) {
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
		Addons: AddonsConfig{
			Cilium: CiliumConfig{
				Enabled:             true,
				PolicyCIDRMatchMode: "invalid",
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid policy_cidr_match_mode")
}

func TestValidateCilium_InvalidGatewayAPIExternalTrafficPolicy(t *testing.T) {
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
		Addons: AddonsConfig{
			Cilium: CiliumConfig{
				Enabled:                         true,
				GatewayAPIExternalTrafficPolicy: "Invalid",
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid gateway_api_external_traffic_policy")
}

// Test CCM validation
func TestValidateCCM_ValidLBTypes(t *testing.T) {
	for lbType := range ValidCCMLBTypes {
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
			Addons: AddonsConfig{
				CCM: CCMConfig{
					Enabled: true,
					LoadBalancers: CCMLoadBalancerConfig{
						Type: lbType,
					},
				},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err, "lb type %q should be valid", lbType)
	}
}

func TestValidateCCM_InvalidLBType(t *testing.T) {
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
		Addons: AddonsConfig{
			CCM: CCMConfig{
				Enabled: true,
				LoadBalancers: CCMLoadBalancerConfig{
					Type: "lb99",
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid load_balancers.type")
}

func TestValidateCCM_ValidAlgorithms(t *testing.T) {
	for algo := range ValidCCMLBAlgorithms {
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
			Addons: AddonsConfig{
				CCM: CCMConfig{
					Enabled: true,
					LoadBalancers: CCMLoadBalancerConfig{
						Algorithm: algo,
					},
				},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err, "algorithm %q should be valid", algo)
	}
}

func TestValidateCCM_InvalidAlgorithm(t *testing.T) {
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
		Addons: AddonsConfig{
			CCM: CCMConfig{
				Enabled: true,
				LoadBalancers: CCMLoadBalancerConfig{
					Algorithm: "random",
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid load_balancers.algorithm")
}

func TestValidateCCM_HealthCheckIntervalOutOfRange(t *testing.T) {
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
		Addons: AddonsConfig{
			CCM: CCMConfig{
				Enabled: true,
				LoadBalancers: CCMLoadBalancerConfig{
					HealthCheck: CCMHealthCheckConfig{
						Interval: 100, // Out of range (3-60)
					},
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interval must be between 3 and 60")
}

func TestValidateCCM_HealthCheckRetriesOutOfRange(t *testing.T) {
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
		Addons: AddonsConfig{
			CCM: CCMConfig{
				Enabled: true,
				LoadBalancers: CCMLoadBalancerConfig{
					HealthCheck: CCMHealthCheckConfig{
						Retries: 20, // Out of range (1-10)
					},
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retries must be between 1 and 10")
}

// Test TalosBackup S3 URL validation
func TestValidateTalosBackup_ValidS3HcloudURL(t *testing.T) {
	validURLs := []string{
		"mybucket.fsn1.your-objectstorage.com",
		"https://mybucket.fsn1.your-objectstorage.com",
		"http://mybucket.nbg1.your-objectstorage.com",
		"mybucket.hel1.your-objectstorage.com.",
	}

	for _, url := range validURLs {
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
			Addons: AddonsConfig{
				TalosBackup: TalosBackupConfig{
					S3HcloudURL: url,
				},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err, "s3_hcloud_url %q should be valid", url)
	}
}

func TestValidateTalosBackup_InvalidS3HcloudURL(t *testing.T) {
	invalidURLs := []string{
		"s3.amazonaws.com/mybucket",
		"mybucket.s3.amazonaws.com",
		"example.com",
		"just-random-text",
		"mybucket.your-objectstorage.com", // Missing region
	}

	for _, url := range invalidURLs {
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
			Addons: AddonsConfig{
				TalosBackup: TalosBackupConfig{
					S3HcloudURL: url,
				},
			},
		}
		err := cfg.Validate()
		require.Error(t, err, "s3_hcloud_url %q should be invalid", url)
		assert.Contains(t, err.Error(), "invalid s3_hcloud_url")
	}
}

func TestValidateTalosBackup_EmptyS3HcloudURL(t *testing.T) {
	// Empty URL should be valid (it's optional)
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
		Addons: AddonsConfig{
			TalosBackup: TalosBackupConfig{
				S3HcloudURL: "",
			},
		},
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}
