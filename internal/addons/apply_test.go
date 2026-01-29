package addons

import (
	"context"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestApply_EmptyKubeconfig(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons:      config.AddonsConfig{CCM: config.CCMConfig{Enabled: true}},
	}
	err := Apply(context.Background(), cfg, nil, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig is required")
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

	err := Apply(context.Background(), cfg, kubeconfig, 1)
	assert.NoError(t, err)
}

func TestHasEnabledAddons(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected bool
	}{
		{
			name:     "no addons enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{}},
			expected: false,
		},
		{
			name:     "traefik enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{Traefik: config.TraefikConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "cilium enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{Cilium: config.CiliumConfig{Enabled: true}}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, hasEnabledAddons(tt.cfg))
		})
	}
}

func TestGetControlPlaneCount(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected int
	}{
		{
			name: "single node",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{{Name: "cp", Count: 1}},
				},
			},
			expected: 1,
		},
		{
			name: "ha cluster",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{{Name: "cp", Count: 3}},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, getControlPlaneCount(tt.cfg))
		})
	}
}
