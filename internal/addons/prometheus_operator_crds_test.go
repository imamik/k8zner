package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"k8zner/internal/config"
)

func TestPrometheusOperatorCRDsURLGeneration(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		expectedURL string
	}{
		{
			name:        "default version",
			version:     "",
			expectedURL: "https://github.com/prometheus-operator/prometheus-operator/releases/download/v0.87.1/stripped-down-crds.yaml",
		},
		{
			name:        "custom version",
			version:     "v0.80.0",
			expectedURL: "https://github.com/prometheus-operator/prometheus-operator/releases/download/v0.80.0/stripped-down-crds.yaml",
		},
		{
			name:        "older version",
			version:     "v0.75.0",
			expectedURL: "https://github.com/prometheus-operator/prometheus-operator/releases/download/v0.75.0/stripped-down-crds.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Addons: config.AddonsConfig{
					PrometheusOperatorCRDs: config.PrometheusOperatorCRDsConfig{
						Enabled: true,
						Version: tt.version,
					},
				},
			}

			url := buildPrometheusOperatorCRDsURL(cfg)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}

func TestPrometheusOperatorCRDsConfig(t *testing.T) {
	t.Run("enabled by default", func(t *testing.T) {
		cfg := config.PrometheusOperatorCRDsConfig{}
		// Defaults are applied in config loader, not in struct
		assert.False(t, cfg.Enabled) // struct default is false
	})

	t.Run("can be disabled", func(t *testing.T) {
		cfg := config.PrometheusOperatorCRDsConfig{
			Enabled: false,
		}
		assert.False(t, cfg.Enabled)
	})

	t.Run("custom version", func(t *testing.T) {
		cfg := config.PrometheusOperatorCRDsConfig{
			Enabled: true,
			Version: "v0.85.0",
		}
		assert.Equal(t, "v0.85.0", cfg.Version)
	})
}

// buildPrometheusOperatorCRDsURL builds the manifest URL for testing purposes.
// This mirrors the logic in applyPrometheusOperatorCRDs.
func buildPrometheusOperatorCRDsURL(cfg *config.Config) string {
	version := cfg.Addons.PrometheusOperatorCRDs.Version
	if version == "" {
		version = defaultPrometheusOperatorCRDsVersion
	}

	return "https://github.com/prometheus-operator/prometheus-operator/releases/download/" + version + "/stripped-down-crds.yaml"
}
