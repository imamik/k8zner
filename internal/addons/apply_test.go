package addons

import (
	"context"
	"testing"

	"k8zner/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestApply_EmptyKubeconfig(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{Enabled: true},
		},
	}

	err := Apply(context.Background(), cfg, nil, 1, "", 0, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig is required")
}

func TestApply_NoCCMConfigured(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{Enabled: false},
			CSI: config.CSIConfig{Enabled: false},
		},
	}

	// Even with valid kubeconfig, if addons are disabled, Apply should succeed
	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters: []
contexts: []
current-context: ""
users: []`)

	err := Apply(context.Background(), cfg, kubeconfig, 1, "", 0, nil)
	// Should succeed since no addons are enabled
	assert.NoError(t, err)
}

func TestApply_NoAddonsConfigured(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{Enabled: false},
			CSI: config.CSIConfig{Enabled: false},
		},
	}

	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters: []
contexts: []
current-context: ""
users: []`)

	err := Apply(context.Background(), cfg, kubeconfig, 1, "", 0, nil)
	assert.NoError(t, err)
}

func TestGetControlPlaneCount(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected int
	}{
		{
			name: "single pool single node",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Name: "cp-1", Count: 1},
					},
				},
			},
			expected: 1,
		},
		{
			name: "single pool multiple nodes",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Name: "cp-1", Count: 3},
					},
				},
			},
			expected: 3,
		},
		{
			name: "multiple pools",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Name: "cp-1", Count: 1},
						{Name: "cp-2", Count: 2},
					},
				},
			},
			expected: 3,
		},
		{
			name: "empty pools",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getControlPlaneCount(tt.cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}
