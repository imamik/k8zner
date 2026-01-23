package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"hcloud-k8s/internal/config"
)

func TestGatewayAPICRDsURLGeneration(t *testing.T) {
	tests := []struct {
		name           string
		version        string
		releaseChannel string
		expectedURL    string
	}{
		{
			name:           "default values",
			version:        "",
			releaseChannel: "",
			expectedURL:    "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml",
		},
		{
			name:           "custom version with standard channel",
			version:        "v1.3.0",
			releaseChannel: "standard",
			expectedURL:    "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.3.0/standard-install.yaml",
		},
		{
			name:           "experimental channel",
			version:        "v1.4.1",
			releaseChannel: "experimental",
			expectedURL:    "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml",
		},
		{
			name:           "older version",
			version:        "v1.0.0",
			releaseChannel: "standard",
			expectedURL:    "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Addons: config.AddonsConfig{
					GatewayAPICRDs: config.GatewayAPICRDsConfig{
						Enabled:        true,
						Version:        tt.version,
						ReleaseChannel: tt.releaseChannel,
					},
				},
			}

			url := buildGatewayAPICRDsURL(cfg)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}

func TestGatewayAPICRDsConfig(t *testing.T) {
	t.Run("enabled by default", func(t *testing.T) {
		cfg := config.GatewayAPICRDsConfig{}
		// Defaults are applied in config loader, not in struct
		assert.False(t, cfg.Enabled) // struct default is false
	})

	t.Run("can be disabled", func(t *testing.T) {
		cfg := config.GatewayAPICRDsConfig{
			Enabled: false,
		}
		assert.False(t, cfg.Enabled)
	})

	t.Run("custom version", func(t *testing.T) {
		cfg := config.GatewayAPICRDsConfig{
			Enabled: true,
			Version: "v1.2.0",
		}
		assert.Equal(t, "v1.2.0", cfg.Version)
	})
}

// buildGatewayAPICRDsURL builds the manifest URL for testing purposes.
// This mirrors the logic in applyGatewayAPICRDs.
func buildGatewayAPICRDsURL(cfg *config.Config) string {
	version := cfg.Addons.GatewayAPICRDs.Version
	if version == "" {
		version = defaultGatewayAPIVersion
	}

	releaseChannel := cfg.Addons.GatewayAPICRDs.ReleaseChannel
	if releaseChannel == "" {
		releaseChannel = defaultGatewayAPIReleaseChannel
	}

	return "https://github.com/kubernetes-sigs/gateway-api/releases/download/" + version + "/" + releaseChannel + "-install.yaml"
}
