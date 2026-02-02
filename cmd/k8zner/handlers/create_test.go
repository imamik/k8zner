package handlers

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
)

func TestGetWorkerCount(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected int
	}{
		{
			name:     "no workers",
			cfg:      &config.Config{Workers: nil},
			expected: 0,
		},
		{
			name:     "empty workers",
			cfg:      &config.Config{Workers: []config.WorkerNodePool{}},
			expected: 0,
		},
		{
			name: "single worker pool",
			cfg: &config.Config{
				Workers: []config.WorkerNodePool{
					{Count: 3},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getWorkerCount(tt.cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetWorkerSize(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected string
	}{
		{
			name:     "no workers returns default",
			cfg:      &config.Config{Workers: nil},
			expected: v2.DefaultWorkerServerType,
		},
		{
			name:     "empty workers returns default",
			cfg:      &config.Config{Workers: []config.WorkerNodePool{}},
			expected: v2.DefaultWorkerServerType,
		},
		{
			name: "configured worker size",
			cfg: &config.Config{
				Workers: []config.WorkerNodePool{
					{ServerType: "cx33"},
				},
			},
			expected: "cx33",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getWorkerSize(tt.cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetBootstrapNode(t *testing.T) {
	tests := []struct {
		name           string
		pCtx           *provisioning.Context
		expectedName   string
		expectedID     int64
		expectedIP     string
	}{
		{
			name: "empty state",
			pCtx: &provisioning.Context{
				State: &provisioning.State{
					ControlPlaneIPs:       map[string]string{},
					ControlPlaneServerIDs: map[string]int64{},
				},
			},
			expectedName: "",
			expectedID:   0,
			expectedIP:   "",
		},
		{
			name: "single control plane",
			pCtx: &provisioning.Context{
				State: &provisioning.State{
					ControlPlaneIPs:       map[string]string{"cluster-cp-abc12": "10.0.0.1"},
					ControlPlaneServerIDs: map[string]int64{"cluster-cp-abc12": 123},
				},
			},
			expectedName: "cluster-cp-abc12",
			expectedID:   123,
			expectedIP:   "10.0.0.1",
		},
		{
			name: "multiple control planes returns first alphabetically",
			pCtx: &provisioning.Context{
				State: &provisioning.State{
					ControlPlaneIPs: map[string]string{
						"cluster-cp-zzz99": "10.0.0.3",
						"cluster-cp-aaa11": "10.0.0.1",
						"cluster-cp-mmm55": "10.0.0.2",
					},
					ControlPlaneServerIDs: map[string]int64{
						"cluster-cp-zzz99": 3,
						"cluster-cp-aaa11": 1,
						"cluster-cp-mmm55": 2,
					},
				},
			},
			expectedName: "cluster-cp-aaa11",
			expectedID:   1,
			expectedIP:   "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, id, ip := getBootstrapNode(tt.pCtx)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.expectedID, id)
			assert.Equal(t, tt.expectedIP, ip)
		})
	}
}

func TestBuildAddonSpec(t *testing.T) {
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			Traefik:       config.TraefikConfig{Enabled: true},
			CertManager:   config.CertManagerConfig{Enabled: true},
			ExternalDNS:   config.ExternalDNSConfig{Enabled: false},
			ArgoCD:        config.ArgoCDConfig{Enabled: true},
			MetricsServer: config.MetricsServerConfig{Enabled: true},
		},
	}

	spec := buildAddonSpec(cfg)

	require.NotNil(t, spec)
	assert.True(t, spec.Traefik)
	assert.True(t, spec.CertManager)
	assert.False(t, spec.ExternalDNS)
	assert.True(t, spec.ArgoCD)
	assert.True(t, spec.MetricsServer)
}

func TestCreate_MissingHCloudToken(t *testing.T) {
	// Save and restore env
	origToken := os.Getenv("HCLOUD_TOKEN")
	defer func() {
		os.Setenv("HCLOUD_TOKEN", origToken)
	}()

	// Save and restore mocks
	origLoad := loadV2ConfigFile
	origExpand := expandV2Config
	defer func() {
		loadV2ConfigFile = origLoad
		expandV2Config = origExpand
	}()

	// Mock config loading
	loadV2ConfigFile = func(_ string) (*v2.Config, error) {
		return &v2.Config{Name: "test", Region: v2.RegionFalkenstein, Mode: v2.ModeDev, Workers: v2.Worker{Count: 1, Size: v2.SizeCX22}}, nil
	}
	expandV2Config = func(_ *v2.Config) (*config.Config, error) {
		return &config.Config{
			ClusterName: "test",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{{Count: 1}},
			},
		}, nil
	}

	// Clear token
	os.Setenv("HCLOUD_TOKEN", "")

	err := Create(context.Background(), "k8zner.yaml", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HCLOUD_TOKEN")
}

// provisionerMock implements the Provisioner interface for testing.
type provisionerMock struct {
	err error
}

func (m *provisionerMock) Provision(_ *provisioning.Context) error {
	return m.err
}

func TestCleanupOnFailure(t *testing.T) {
	// Save and restore mocks
	origDestroy := newDestroyProvisioner
	defer func() {
		newDestroyProvisioner = origDestroy
	}()

	newDestroyProvisioner = func() Provisioner {
		return &provisionerMock{}
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	infraClient := &hcloud.MockClient{}

	err := cleanupOnFailure(context.Background(), cfg, infraClient)
	require.NoError(t, err)
}
