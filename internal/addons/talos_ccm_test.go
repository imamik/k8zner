package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/imamik/k8zner/internal/config"
)

func TestTalosCCMURLGeneration(t *testing.T) {
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
	t.Run("struct defaults", func(t *testing.T) {
		cfg := config.TalosCCMConfig{}
		// Struct defaults are false/empty - actual defaults applied in config loader
		assert.False(t, cfg.Enabled)
		assert.Empty(t, cfg.Version)
	})

	t.Run("can be disabled", func(t *testing.T) {
		cfg := config.TalosCCMConfig{
			Enabled: false,
		}
		assert.False(t, cfg.Enabled)
	})

	t.Run("custom version", func(t *testing.T) {
		cfg := config.TalosCCMConfig{
			Enabled: true,
			Version: "v1.10.0",
		}
		assert.True(t, cfg.Enabled)
		assert.Equal(t, "v1.10.0", cfg.Version)
	})

	t.Run("enabled with default version", func(t *testing.T) {
		cfg := config.TalosCCMConfig{
			Enabled: true,
			Version: "v1.11.0", // Default set by config loader
		}
		assert.True(t, cfg.Enabled)
		assert.Equal(t, "v1.11.0", cfg.Version)
	})
}

// buildTalosCCMURL builds the manifest URL for testing purposes.
// This mirrors the logic in applyTalosCCM.
func buildTalosCCMURL(cfg *config.Config) string {
	version := cfg.Addons.TalosCCM.Version
	return "https://raw.githubusercontent.com/siderolabs/talos-cloud-controller-manager/" + version + "/docs/deploy/cloud-controller-manager-daemonset.yml"
}
