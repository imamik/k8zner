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
				CertManager: CertManagerConfig{Enabled: true},
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
			CertManager: CertManagerConfig{Enabled: true},
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
			CertManager: CertManagerConfig{Enabled: true},
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
			CertManager: CertManagerConfig{Enabled: true},
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
						Retries: 10, // Out of range (0-5)
					},
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retries must be between 0 and 5")
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

// Test cluster name validation
func TestValidateClusterName_Valid(t *testing.T) {
	validNames := []string{
		"a",
		"a1",
		"test",
		"my-cluster",
		"cluster123",
		"a-b-c-d",
		"abcdefghijklmnopqrstuvwxyz12345", // 31 chars
	}
	for _, name := range validNames {
		cfg := &Config{
			ClusterName: name,
			HCloudToken: "token",
			Location:    "nbg1",
			Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
			ControlPlane: ControlPlaneConfig{
				NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err, "cluster name %q should be valid", name)
	}
}

func TestValidateClusterName_Invalid(t *testing.T) {
	invalidNames := []string{
		"",                                    // empty
		"-test",                               // starts with hyphen
		"test-",                               // ends with hyphen
		"TEST",                                // uppercase
		"Test",                                // mixed case
		"test_cluster",                        // underscore
		"test.cluster",                        // dot
		"abcdefghijklmnopqrstuvwxyz123456789", // too long (>32)
	}
	for _, name := range invalidNames {
		cfg := &Config{
			ClusterName: name,
			HCloudToken: "token",
			Location:    "nbg1",
			Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
			ControlPlane: ControlPlaneConfig{
				NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
			},
		}
		err := cfg.Validate()
		if name == "" {
			assert.Error(t, err, "empty cluster name should fail")
		} else {
			require.Error(t, err, "cluster name %q should be invalid", name)
			assert.Contains(t, err.Error(), "invalid cluster_name")
		}
	}
}

// Test node pool uniqueness validation
func TestValidateNodePoolUniqueness_DuplicateControlPlane(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{
				{Name: "cp1", ServerType: "cpx22", Count: 1},
				{Name: "cp1", ServerType: "cpx22", Count: 1}, // duplicate
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate control plane pool name")
}

func TestValidateNodePoolUniqueness_DuplicateWorker(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Workers: []WorkerNodePool{
			{Name: "worker", ServerType: "cpx22", Count: 1},
			{Name: "worker", ServerType: "cpx22", Count: 1}, // duplicate
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate worker pool name")
}

func TestValidateNodePoolUniqueness_DuplicateAutoscaler(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Autoscaler: AutoscalerConfig{
			NodePools: []AutoscalerNodePool{
				{Name: "as1", Type: "cpx22", Location: "nbg1", Min: 0, Max: 5},
				{Name: "as1", Type: "cpx22", Location: "nbg1", Min: 0, Max: 5}, // duplicate
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate autoscaler pool name")
}

// Test node count constraints validation
func TestValidateNodeCounts_ControlPlaneExceeds9(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{
				{Name: "cp1", ServerType: "cpx22", Count: 5},
				{Name: "cp2", ServerType: "cpx22", Count: 5}, // total 10
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "total control plane nodes must be <= 9")
}

func TestValidateNodeCounts_TotalExceeds100(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 3}},
		},
		Workers: []WorkerNodePool{
			{Name: "worker", ServerType: "cpx22", Count: 50},
		},
		Autoscaler: AutoscalerConfig{
			NodePools: []AutoscalerNodePool{
				{Name: "as", Type: "cpx22", Location: "nbg1", Min: 0, Max: 50},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "total nodes")
	assert.Contains(t, err.Error(), "must be <= 100")
}

// Test combined name length validation
func TestValidateCombinedNameLengths_Exceeds56(t *testing.T) {
	cfg := &Config{
		ClusterName: "this-is-a-very-long-cluster-name", // 32 chars
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{
				{Name: "this-is-also-a-long-pool-name", ServerType: "cpx22", Count: 1}, // 29 chars, total 62
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "combined length")
	assert.Contains(t, err.Error(), "exceeds")
}

// Test firewall rule cross-validation
func TestValidateFirewallRules_InDirectionRequiresSourceIPs(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Firewall: FirewallConfig{
			ExtraRules: []FirewallRule{
				{Direction: "in", Protocol: "tcp", Port: "443"}, // missing source_ips
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'in' direction requires source_ips")
}

func TestValidateFirewallRules_OutDirectionRequiresDestinationIPs(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Firewall: FirewallConfig{
			ExtraRules: []FirewallRule{
				{Direction: "out", Protocol: "tcp", Port: "443"}, // missing destination_ips
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'out' direction requires destination_ips")
}

func TestValidateFirewallRules_TCPRequiresPort(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Firewall: FirewallConfig{
			ExtraRules: []FirewallRule{
				{Direction: "in", Protocol: "tcp", SourceIPs: []string{"0.0.0.0/0"}}, // missing port
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tcp protocol requires port")
}

func TestValidateFirewallRules_ICMPCannotHavePort(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Firewall: FirewallConfig{
			ExtraRules: []FirewallRule{
				{Direction: "in", Protocol: "icmp", Port: "80", SourceIPs: []string{"0.0.0.0/0"}},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "icmp protocol cannot have port")
}

func TestValidateFirewallRules_ValidRule(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Firewall: FirewallConfig{
			ExtraRules: []FirewallRule{
				{Direction: "in", Protocol: "tcp", Port: "443", SourceIPs: []string{"0.0.0.0/0"}},
				{Direction: "out", Protocol: "tcp", Port: "443", DestinationIPs: []string{"0.0.0.0/0"}},
				{Direction: "in", Protocol: "icmp", SourceIPs: []string{"0.0.0.0/0"}},
			},
		},
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

// Test Cilium dependency chain validation
func TestValidateCilium_EgressGatewayRequiresKubeProxyReplacement(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cilium: CiliumConfig{
				Enabled:                     true,
				EgressGatewayEnabled:        true,
				KubeProxyReplacementEnabled: false,
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "egress_gateway_enabled requires kube_proxy_replacement_enabled=true")
}

func TestValidateCilium_HubbleRelayRequiresHubble(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cilium: CiliumConfig{
				Enabled:            true,
				HubbleRelayEnabled: true,
				HubbleEnabled:      false,
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hubble_relay_enabled requires hubble_enabled=true")
}

func TestValidateCilium_HubbleUIRequiresRelay(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cilium: CiliumConfig{
				Enabled:            true,
				HubbleEnabled:      true,
				HubbleRelayEnabled: false,
				HubbleUIEnabled:    true,
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hubble_ui_enabled requires hubble_relay_enabled=true")
}

func TestValidateCilium_IPSecKeyIDOutOfRange(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cilium: CiliumConfig{
				Enabled:        true,
				EncryptionType: "ipsec",
				IPSecKeyID:     16, // Out of range (1-15)
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ipsec_key_id must be 1-15")
}

func TestValidateCilium_IPSecKeySizeInvalid(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cilium: CiliumConfig{
				Enabled:        true,
				EncryptionType: "ipsec",
				IPSecKeySize:   64, // Invalid (must be 128, 192, or 256)
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ipsec_key_size must be 128, 192, or 256")
}

// Test OIDC required fields validation
func TestValidateOIDC_RequiresIssuerURL(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Kubernetes: KubernetesConfig{
			OIDC: OIDCConfig{
				Enabled:   true,
				ClientID:  "client-id",
				IssuerURL: "", // missing
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc.issuer_url is required")
}

func TestValidateOIDC_RequiresClientID(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Kubernetes: KubernetesConfig{
			OIDC: OIDCConfig{
				Enabled:   true,
				IssuerURL: "https://issuer.example.com",
				ClientID:  "", // missing
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc.client_id is required")
}

func TestValidateOIDC_DuplicateGroupMapping(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Kubernetes: KubernetesConfig{
			OIDC: OIDCConfig{
				Enabled:   true,
				IssuerURL: "https://issuer.example.com",
				ClientID:  "client-id",
			},
		},
		Addons: AddonsConfig{
			OIDCRBAC: OIDCRBACConfig{
				GroupMappings: []OIDCRBACGroupMapping{
					{Group: "admins", ClusterRoles: []string{"cluster-admin"}},
					{Group: "admins", ClusterRoles: []string{"cluster-admin"}}, // duplicate
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate OIDC group mapping")
}

// Test autoscaler min/max validation
func TestValidateAutoscaler_MaxLessThanMin(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Autoscaler: AutoscalerConfig{
			NodePools: []AutoscalerNodePool{
				{Name: "as", Type: "cpx22", Location: "nbg1", Min: 5, Max: 3},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max")
	assert.Contains(t, err.Error(), "min")
}

func TestValidateAutoscaler_NegativeMin(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Autoscaler: AutoscalerConfig{
			NodePools: []AutoscalerNodePool{
				{Name: "as", Type: "cpx22", Location: "nbg1", Min: -1, Max: 5},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min cannot be negative")
}

// Test CSI passphrase validation
func TestValidateCSI_PassphraseTooShort(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			CSI: CSIConfig{
				Enabled:              true,
				EncryptionPassphrase: "short", // <8 chars
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encryption_passphrase must be 8-512 characters")
}

func TestValidateCSI_PassphraseNonPrintableASCII(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			CSI: CSIConfig{
				Enabled:              true,
				EncryptionPassphrase: "test\x00pass", // contains null byte
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-printable ASCII")
}

func TestValidateCSI_ValidPassphrase(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			CSI: CSIConfig{
				Enabled:              true,
				EncryptionPassphrase: "my-secure-passphrase-123!",
			},
		},
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

// Test Ingress NGINX cert-manager dependency
func TestValidateIngressNginx_RequiresCertManager(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			IngressNginx: IngressNginxConfig{Enabled: true},
			CertManager:  CertManagerConfig{Enabled: false},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ingress_nginx requires cert_manager to be enabled")
}

// Test kubelet mount validations
func TestValidateTalosKubeletMounts_DuplicateDestination(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Talos: TalosConfig{
			Machine: TalosMachineConfig{
				KubeletExtraMounts: []TalosKubeletMount{
					{Source: "/data1", Destination: "/mnt/data"},
					{Source: "/data2", Destination: "/mnt/data"}, // duplicate destination
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate destination")
}

func TestValidateTalosKubeletMounts_LonghornConflict(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Talos: TalosConfig{
			Machine: TalosMachineConfig{
				KubeletExtraMounts: []TalosKubeletMount{
					{Source: "/my-longhorn", Destination: "/var/lib/longhorn"},
				},
			},
		},
		Addons: AddonsConfig{
			Longhorn: LonghornConfig{Enabled: true},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/var/lib/longhorn conflicts with Longhorn addon")
}

// Test Cloudflare DNS integration validation
func TestValidateCloudflare_Disabled(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare: CloudflareConfig{Enabled: false},
		},
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidateCloudflare_RequiresAPIToken(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare: CloudflareConfig{
				Enabled:  true,
				APIToken: "", // missing
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cloudflare.api_token is required")
}

func TestValidateCloudflare_ValidConfig(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare: CloudflareConfig{
				Enabled:  true,
				APIToken: "test-token",
				Domain:   "example.com",
			},
		},
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidateCloudflare_ExternalDNSRequiresCloudflare(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare:  CloudflareConfig{Enabled: false},
			ExternalDNS: ExternalDNSConfig{Enabled: true},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "external_dns requires cloudflare to be enabled")
}

func TestValidateCloudflare_CertManagerCloudflareRequiresCloudflare(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare: CloudflareConfig{Enabled: false},
			CertManager: CertManagerConfig{
				Enabled: true,
				Cloudflare: CertManagerCloudflareConfig{
					Enabled: true,
					Email:   "test@example.com",
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cert_manager.cloudflare requires cloudflare to be enabled")
}

func TestValidateCloudflare_CertManagerCloudflareRequiresCertManager(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare: CloudflareConfig{
				Enabled:  true,
				APIToken: "test-token",
			},
			CertManager: CertManagerConfig{
				Enabled: false,
				Cloudflare: CertManagerCloudflareConfig{
					Enabled: true,
					Email:   "test@example.com",
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cert_manager must be enabled")
}

func TestValidateCloudflare_CertManagerCloudflareRequiresEmail(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare: CloudflareConfig{
				Enabled:  true,
				APIToken: "test-token",
			},
			CertManager: CertManagerConfig{
				Enabled: true,
				Cloudflare: CertManagerCloudflareConfig{
					Enabled: true,
					Email:   "", // missing
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "email is required")
}

func TestValidateCloudflare_CertManagerCloudflareInvalidEmail(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare: CloudflareConfig{
				Enabled:  true,
				APIToken: "test-token",
			},
			CertManager: CertManagerConfig{
				Enabled: true,
				Cloudflare: CertManagerCloudflareConfig{
					Enabled: true,
					Email:   "notanemail", // invalid - no @
				},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid email format")
}

func TestValidateCloudflare_ValidFullConfig(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare: CloudflareConfig{
				Enabled:  true,
				APIToken: "test-token",
				Domain:   "example.com",
			},
			ExternalDNS: ExternalDNSConfig{
				Enabled: true,
				Policy:  "sync",
				Sources: []string{"ingress"},
			},
			CertManager: CertManagerConfig{
				Enabled: true,
				Cloudflare: CertManagerCloudflareConfig{
					Enabled:    true,
					Email:      "admin@example.com",
					Production: false,
				},
			},
		},
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidateExternalDNS_ValidPolicies(t *testing.T) {
	for policy := range ValidExternalDNSPolicies {
		cfg := &Config{
			ClusterName: "test",
			HCloudToken: "token",
			Location:    "nbg1",
			Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
			ControlPlane: ControlPlaneConfig{
				NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
			},
			Addons: AddonsConfig{
				Cloudflare: CloudflareConfig{
					Enabled:  true,
					APIToken: "test-token",
				},
				ExternalDNS: ExternalDNSConfig{
					Enabled: true,
					Policy:  policy,
				},
			},
		}
		err := cfg.Validate()
		assert.NoError(t, err, "policy %q should be valid", policy)
	}
}

func TestValidateExternalDNS_InvalidPolicy(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare: CloudflareConfig{
				Enabled:  true,
				APIToken: "test-token",
			},
			ExternalDNS: ExternalDNSConfig{
				Enabled: true,
				Policy:  "invalid-policy",
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid policy")
}

func TestValidateExternalDNS_ValidSources(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare: CloudflareConfig{
				Enabled:  true,
				APIToken: "test-token",
			},
			ExternalDNS: ExternalDNSConfig{
				Enabled: true,
				Sources: []string{"ingress", "service"},
			},
		},
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidateExternalDNS_InvalidSource(t *testing.T) {
	cfg := &Config{
		ClusterName: "test",
		HCloudToken: "token",
		Location:    "nbg1",
		Network:     NetworkConfig{IPv4CIDR: "10.0.0.0/16", Zone: "eu-central"},
		ControlPlane: ControlPlaneConfig{
			NodePools: []ControlPlaneNodePool{{Name: "cp", ServerType: "cpx22", Count: 1}},
		},
		Addons: AddonsConfig{
			Cloudflare: CloudflareConfig{
				Enabled:  true,
				APIToken: "test-token",
			},
			ExternalDNS: ExternalDNSConfig{
				Enabled: true,
				Sources: []string{"invalid-source"},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid source")
}
