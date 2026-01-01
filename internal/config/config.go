package config

import (
	"errors"
	"fmt"

	"github.com/mitchellh/mapstructure"
)

// Config holds the application configuration.
type Config struct {
	HCloudToken string `mapstructure:"hcloud_token"`
	ClusterName string `mapstructure:"cluster_name"`
}

// Load parses the configuration from a map.
func Load(input map[string]interface{}) (*Config, error) {
	var cfg Config
	if err := mapstructure.Decode(input, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}
	return &cfg, nil
}

// Validate checks the configuration for errors.
func Validate(cfg *Config) error {
	if cfg.HCloudToken == "" {
		return errors.New("hcloud_token is required")
	}
	if cfg.ClusterName == "" {
		return errors.New("cluster_name is required")
	}
	return nil
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	return Validate(c)
}
