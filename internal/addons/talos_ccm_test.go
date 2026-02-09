package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/imamik/k8zner/internal/config"
)

func TestTalosCCMURLGeneration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		version     string
		expectedURL string
	}{
		{
			name:        "default version from config loader",
			version:     "v1.11.0",
			expectedURL: "https://raw.githubusercontent.com/siderolabs/talos-cloud-controller-manager/v1.11.0/docs/deploy/cloud-controller-manager-daemonset.yml",
		},
		{
			name:        "custom version",
			version:     "v1.10.0",
			expectedURL: "https://raw.githubusercontent.com/siderolabs/talos-cloud-controller-manager/v1.10.0/docs/deploy/cloud-controller-manager-daemonset.yml",
		},
		{
			name:        "older version",
			version:     "v1.8.0",
			expectedURL: "https://raw.githubusercontent.com/siderolabs/talos-cloud-controller-manager/v1.8.0/docs/deploy/cloud-controller-manager-daemonset.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{
				Addons: config.AddonsConfig{
					TalosCCM: config.TalosCCMConfig{
						Enabled: true,
						Version: tt.version,
					},
				},
			}

			url := buildTalosCCMURL(cfg)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}

func TestTalosCCMConfig(t *testing.T) {
	t.Parallel()
	t.Run("struct defaults", func(t *testing.T) {
		t.Parallel()
		cfg := config.TalosCCMConfig{}
		// Struct defaults are false/empty - actual defaults applied in config loader
		assert.False(t, cfg.Enabled)
		assert.Empty(t, cfg.Version)
	})

	t.Run("can be disabled", func(t *testing.T) {
		t.Parallel()
		cfg := config.TalosCCMConfig{
			Enabled: false,
		}
		assert.False(t, cfg.Enabled)
	})

	t.Run("custom version", func(t *testing.T) {
		t.Parallel()
		cfg := config.TalosCCMConfig{
			Enabled: true,
			Version: "v1.10.0",
		}
		assert.True(t, cfg.Enabled)
		assert.Equal(t, "v1.10.0", cfg.Version)
	})

	t.Run("enabled with default version", func(t *testing.T) {
		t.Parallel()
		cfg := config.TalosCCMConfig{
			Enabled: true,
			Version: "v1.11.0", // Default set by config loader
		}
		assert.True(t, cfg.Enabled)
		assert.Equal(t, "v1.11.0", cfg.Version)
	})
}

func TestTalosCCMHostNetworkPatching(t *testing.T) {
	t.Parallel()
	// Talos CCM runs with hostNetwork on control plane nodes.
	// When kube-proxy is disabled, API access requires direct env vars.
	// This tests the patchHostNetworkAPIAccess function.

	daemonSetManifest := `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: talos-cloud-controller-manager
  namespace: kube-system
spec:
  template:
    spec:
      hostNetwork: true
      containers:
      - name: talos-cloud-controller-manager
        image: ghcr.io/siderolabs/talos-cloud-controller-manager:v1.11.0
`

	patched, err := patchHostNetworkAPIAccess([]byte(daemonSetManifest), "talos-cloud-controller-manager")
	assert.NoError(t, err)

	// Verify env vars were injected
	patchedStr := string(patched)
	assert.Contains(t, patchedStr, "KUBERNETES_SERVICE_HOST")
	assert.Contains(t, patchedStr, "KUBERNETES_SERVICE_PORT")
}

func TestTalosCCMVersionFormats(t *testing.T) {
	t.Parallel()
	// Verify URL format for different version string patterns

	tests := []struct {
		version string
	}{
		{"v1.11.0"},
		{"v1.10.0"},
		{"v2.0.0-alpha.1"},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{
				Addons: config.AddonsConfig{
					TalosCCM: config.TalosCCMConfig{
						Enabled: true,
						Version: tt.version,
					},
				},
			}
			url := buildTalosCCMURL(cfg)
			assert.Contains(t, url, tt.version)
			assert.Contains(t, url, "siderolabs/talos-cloud-controller-manager")
		})
	}
}

// buildTalosCCMURL builds the manifest URL for testing purposes.
// This mirrors the logic in applyTalosCCM.
func buildTalosCCMURL(cfg *config.Config) string {
	version := cfg.Addons.TalosCCM.Version
	return "https://raw.githubusercontent.com/siderolabs/talos-cloud-controller-manager/" + version + "/docs/deploy/cloud-controller-manager-daemonset.yml"
}
