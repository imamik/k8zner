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
