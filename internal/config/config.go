package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ClusterName string     `yaml:"clusterName"`
	Hetzner     Hetzner    `yaml:"hetzner"`
	Nodes       Nodes      `yaml:"nodes"`
	Talos       Talos      `yaml:"talos"`
	Kubernetes  Kubernetes `yaml:"kubernetes"`
}

type Hetzner struct {
	Region      string   `yaml:"region"`
	NetworkZone string   `yaml:"networkZone"`
	SSHKeys     []string `yaml:"sshKeys"`
	Firewall    Firewall `yaml:"firewall"`
}

type Firewall struct {
	APISource []string `yaml:"apiSource"`
}

type Nodes struct {
	ControlPlane ControlPlane `yaml:"controlPlane"`
	Workers      Workers      `yaml:"workers"`
}

type ControlPlane struct {
	Count      int    `yaml:"count"`
	Type       string `yaml:"type"`
	FloatingIP bool   `yaml:"floatingIp"`
}

type Workers struct {
	Nodepools []Nodepool `yaml:"nodepools"`
}

type Nodepool struct {
	Name           string `yaml:"name"`
	Count          int    `yaml:"count"`
	Type           string `yaml:"type"`
	PlacementGroup bool   `yaml:"placementGroup"`
}

type Talos struct {
	Version string `yaml:"version"`
}

type Kubernetes struct {
	Version string `yaml:"version"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.ClusterName == "" {
		return fmt.Errorf("clusterName is required")
	}
	if c.Hetzner.Region == "" {
		return fmt.Errorf("hetzner.region is required")
	}
	// Add more validation as needed
	return nil
}
