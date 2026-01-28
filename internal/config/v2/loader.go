package v2

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultConfigFilename is the default configuration filename.
const DefaultConfigFilename = "k8zner.yaml"

// Load loads and validates a configuration from a file.
func Load(path string) (*Config, error) {
	cfg, err := LoadWithoutValidation(path)
	if err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// LoadWithoutValidation loads a configuration from a file without validation.
// This is useful for tooling that needs to read partially valid configs.
func LoadWithoutValidation(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return parseConfig(data)
}

// LoadFromBytes loads and validates a configuration from bytes.
func LoadFromBytes(data []byte) (*Config, error) {
	cfg, err := parseConfig(data)
	if err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// LoadFromBytesWithoutValidation loads a configuration from bytes without validation.
func LoadFromBytesWithoutValidation(data []byte) (*Config, error) {
	return parseConfig(data)
}

// parseConfig parses YAML data into a Config struct.
func parseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	return &cfg, nil
}

// DefaultConfigPath returns the default path for the config file.
// It looks in the current working directory.
func DefaultConfigPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return DefaultConfigFilename
	}
	return filepath.Join(cwd, DefaultConfigFilename)
}

// FindConfigFile searches for a config file in common locations.
// It checks: current directory, then walks up to find k8zner.yaml.
func FindConfigFile() (string, error) {
	// Check current directory first
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check current directory
	path := filepath.Join(cwd, DefaultConfigFilename)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// Walk up the directory tree
	dir := cwd
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent

		path := filepath.Join(dir, DefaultConfigFilename)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("config file %s not found", DefaultConfigFilename)
}

// Save writes a configuration to a file.
func Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
