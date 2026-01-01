package talos

import (
	"fmt"

	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
)

// ConfigGenerator generates Talos machine configurations.
type ConfigGenerator struct {
	clusterName string
	endpoint    string
	secrets     *secrets.Bundle
}

// NewConfigGenerator creates a new ConfigGenerator.
func NewConfigGenerator(clusterName, endpoint string) *ConfigGenerator {
	return &ConfigGenerator{
		clusterName: clusterName,
		endpoint:    endpoint,
	}
}

// Generate generates a machine configuration.
func (g *ConfigGenerator) Generate(roleName, nodeIP string, certSANs []string) (config.Provider, error) {
	if g.secrets == nil {
		clock := secrets.NewClock()
		bundle, err := secrets.NewBundle(clock, nil) // nil VersionContract means current version
		if err != nil {
			return nil, fmt.Errorf("failed to create secrets bundle: %w", err)
		}
		g.secrets = bundle
	}

	machineType := machine.TypeControlPlane
	if roleName == "worker" {
		machineType = machine.TypeWorker
	}

	input, err := generate.NewInput(
		g.clusterName,
		g.endpoint,
		"v1.30.0", // Default k8s version, should be configurable but hardcoded for now as per minimal implementation
		generate.WithSecretsBundle(g.secrets),
		generate.WithAdditionalSubjectAltNames(certSANs),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create generate input: %w", err)
	}

	cfg, err := input.Config(machineType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate config: %w", err)
	}

	return cfg, nil
}
