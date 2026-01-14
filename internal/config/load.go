package config

import (
	"fmt"
	"os"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

// LoadFile reads and parses the configuration from a YAML file.
func LoadFile(path string) (*Config, error) {
	// #nosec G304
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal(data, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
	}

	var cfg Config
	if err := mapstructure.Decode(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	// Set defaults
	if cfg.Network.IPv4CIDR == "" {
		cfg.Network.IPv4CIDR = "10.0.0.0/16"
	}
	if cfg.Network.Zone == "" {
		cfg.Network.Zone = "eu-central"
	}

	// Default CCM to enabled (matches Terraform behavior and provides cloud integration)
	if !cfg.Addons.CCM.Enabled {
		cfg.Addons.CCM.Enabled = shouldEnableCCMByDefault(rawConfig)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

// shouldEnableCCMByDefault determines if CCM should be enabled when not explicitly configured.
// Returns true if the CCM enabled field was not explicitly set to false in the raw config.
func shouldEnableCCMByDefault(rawConfig map[string]interface{}) bool {
	// Check if addons.ccm.enabled was explicitly set to false
	addonsMap, ok := rawConfig["addons"].(map[string]interface{})
	if !ok {
		return true // No addons section, default to enabled
	}

	ccmMap, ok := addonsMap["ccm"].(map[string]interface{})
	if !ok {
		return true // No ccm section, default to enabled
	}

	_, explicitlySet := ccmMap["enabled"]
	return !explicitlySet // Default to enabled if not explicitly set
}
