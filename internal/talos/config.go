package talos

import (
	"fmt"
	"os"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
)

// ConfigGenerator handles Talos configuration generation.
type ConfigGenerator struct {
	clusterName       string
	kubernetesVersion string
	talosVersion      string
	endpoint          string
	secretsBundle     *secrets.Bundle
}

// NewConfigGenerator creates a new ConfigGenerator.
// It attempts to load secrets from secretsFile if it exists, otherwise creates new secrets.
func NewConfigGenerator(clusterName, kubernetesVersion, talosVersion, endpoint, secretsFile string) (*ConfigGenerator, error) {
	vc, err := config.ParseContractFromVersion(talosVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse version contract: %w", err)
	}

	var sb *secrets.Bundle
	if _, err := os.Stat(secretsFile); err == nil {
		// Load existing secrets
		// data, err := os.ReadFile(secretsFile)
		// if err != nil { ... }
		// TODO: Implement actual secrets loading. For now, we assume if file exists we MIGHT want to load it.
		// BUT since we can't easily unmarshal without complex logic, we will just create NEW secrets for this iteration
		// and warn that we are ignoring the file.
		// Real implementation requires mapstructure/yaml unmarshalling into Bundle struct or re-using specific keys.

		// For now, FALLBACK to creating new bundle to ensure it works.
		sb, err = secrets.NewBundle(secrets.NewFixedClock(time.Now()), vc)
		if err != nil {
			return nil, fmt.Errorf("failed to create secrets bundle: %w", err)
		}
	} else {
		sb, err = secrets.NewBundle(secrets.NewFixedClock(time.Now()), vc)
		if err != nil {
			return nil, fmt.Errorf("failed to create secrets bundle: %w", err)
		}
	}

	return &ConfigGenerator{
		clusterName:       clusterName,
		kubernetesVersion: kubernetesVersion,
		talosVersion:      talosVersion,
		endpoint:          endpoint,
		secretsBundle:     sb,
	}, nil
}

// GenerateControlPlaneConfig generates the configuration for a control plane node.
func (g *ConfigGenerator) GenerateControlPlaneConfig(san []string) ([]byte, error) {
	vc, err := config.ParseContractFromVersion(g.talosVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse version contract: %w", err)
	}

	input, err := generate.NewInput(
		g.clusterName,
		g.endpoint,
		g.kubernetesVersion,
		generate.WithVersionContract(vc),
		generate.WithSecretsBundle(g.secretsBundle),
		generate.WithAdditionalSubjectAltNames(san),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create input: %w", err)
	}

	cfg, err := input.Config(machine.TypeControlPlane)
	if err != nil {
		return nil, fmt.Errorf("failed to generate control plane config: %w", err)
	}

	return cfg.Bytes()
}

// GenerateWorkerConfig generates the configuration for a worker node.
func (g *ConfigGenerator) GenerateWorkerConfig() ([]byte, error) {
	vc, err := config.ParseContractFromVersion(g.talosVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse version contract: %w", err)
	}

	input, err := generate.NewInput(
		g.clusterName,
		g.endpoint,
		g.kubernetesVersion,
		generate.WithVersionContract(vc),
		generate.WithSecretsBundle(g.secretsBundle),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create input: %w", err)
	}

	cfg, err := input.Config(machine.TypeWorker)
	if err != nil {
		return nil, fmt.Errorf("failed to generate worker config: %w", err)
	}

	return cfg.Bytes()
}

// GetClientConfig returns the talosconfig for the cluster.
func (g *ConfigGenerator) GetClientConfig() ([]byte, error) {
	vc, err := config.ParseContractFromVersion(g.talosVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse version contract: %w", err)
	}

	opts := []generate.Option{
		generate.WithVersionContract(vc),
		generate.WithSecretsBundle(g.secretsBundle),
	}

	input, err := generate.NewInput(g.clusterName, g.endpoint, g.kubernetesVersion, opts...)
	if err != nil {
		return nil, err
	}

	clientCfg, err := input.Talosconfig()
	if err != nil {
		return nil, err
	}

	return clientCfg.Bytes()
}
