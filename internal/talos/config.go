// Package talos provides functionality for generating and managing Talos Linux configurations.
package talos

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
// It attempts to load secrets from secretsFile if it exists, otherwise creates new secrets and saves them.
func NewConfigGenerator(clusterName, kubernetesVersion, talosVersion, endpoint, secretsFile string) (*ConfigGenerator, error) {
	vc, err := config.ParseContractFromVersion(talosVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse version contract: %w", err)
	}

	var sb *secrets.Bundle
	if secretsFile != "" {
		if _, err := os.Stat(secretsFile); err == nil {
			// Load existing secrets using machinery's loader (expects JSON compatible with Bundle struct)
			sb, err = secrets.LoadBundle(secretsFile)
			if err != nil {
				return nil, fmt.Errorf("failed to load secrets bundle: %w", err)
			}

			// Validate loaded bundle (basic check)
			if sb == nil {
				return nil, fmt.Errorf("loaded secrets bundle is nil")
			}
			// Re-inject clock because loaded bundle might have it reset/nil or need valid clock
			sb.Clock = secrets.NewFixedClock(time.Now())

		} else {
			// Create new
			sb, err = secrets.NewBundle(secrets.NewFixedClock(time.Now()), vc)
			if err != nil {
				return nil, fmt.Errorf("failed to create secrets bundle: %w", err)
			}

			// Save secrets
			// We marshal the whole bundle which is compatible with LoadBundle
			data, err := json.MarshalIndent(sb, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("failed to marshal secrets bundle: %w", err)
			}
			if err := os.WriteFile(secretsFile, data, 0600); err != nil {
				return nil, fmt.Errorf("failed to write secrets file: %w", err)
			}
		}
	} else {
		// In-memory only
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

// SetEndpoint updates the control plane endpoint.
func (g *ConfigGenerator) SetEndpoint(endpoint string) {
	g.endpoint = endpoint
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
		generate.WithInstallDisk("/dev/sda"), // Hetzner Cloud uses /dev/sda
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create input: %w", err)
	}

	cfg, err := input.Config(machine.TypeControlPlane)
	if err != nil {
		return nil, fmt.Errorf("failed to generate control plane config: %w", err)
	}

	bytes, err := cfg.Bytes()
	if err != nil {
		return nil, err
	}

	return stripComments(bytes), nil
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
		generate.WithInstallDisk("/dev/sda"), // Hetzner Cloud uses /dev/sda
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create input: %w", err)
	}

	cfg, err := input.Config(machine.TypeWorker)
	if err != nil {
		return nil, fmt.Errorf("failed to generate worker config: %w", err)
	}

	bytes, err := cfg.Bytes()
	if err != nil {
		return nil, err
	}

	return stripComments(bytes), nil
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

	bytes, err := clientCfg.Bytes()
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

func stripComments(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		result = append(result, line)
	}
	return []byte(strings.Join(result, "\n"))
}
