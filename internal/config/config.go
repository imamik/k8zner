package config

import (
	"fmt"
	"os"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

// Config holds the application configuration.
type Config struct {
	ClusterName string `mapstructure:"cluster_name" yaml:"cluster_name"`
	HCloudToken string `mapstructure:"hcloud_token" yaml:"hcloud_token"`

	// Network Configuration
	ControlPlane ControlPlaneConfig `mapstructure:"control_plane" yaml:"control_plane"`

	// Talos Configuration
	Talos TalosConfig `mapstructure:"talos" yaml:"talos"`
}

type ControlPlaneConfig struct {
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint"`
}

type TalosConfig struct {
	Version string `mapstructure:"version" yaml:"version"` // Talos version (e.g., v1.7.0)
	K8sVersion string `mapstructure:"k8s_version" yaml:"k8s_version"` // K8s version (e.g., v1.30.0)
}

// LoadFile reads and parses the configuration from a YAML file.
func LoadFile(path string) (*Config, error) {
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

	// Validate
	if cfg.ClusterName == "" {
		return nil, fmt.Errorf("cluster_name is required")
	}

	return &cfg, nil
}
