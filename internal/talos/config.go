// Package talos provides functionality for generating and managing Talos Linux configurations.
package talos

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	hcloud_config "github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	talos_config "github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1"
)

// ConfigGenerator handles Talos configuration generation.
type ConfigGenerator struct {
	clusterName       string
	kubernetesVersion string
	talosVersion      string
	endpoint          string
	cniType           string
	registryMirrors   []hcloud_config.RegistryMirror
	secretsBundle     *secrets.Bundle
}

// NewConfigGenerator creates a new ConfigGenerator.
func NewConfigGenerator(clusterName, kubernetesVersion, talosVersion, endpoint, cniType string, registryMirrors []hcloud_config.RegistryMirror, secretsFile string) (*ConfigGenerator, error) {
	vc, err := talos_config.ParseContractFromVersion(talosVersion)
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
		cniType:           cniType,
		registryMirrors:   registryMirrors,
		secretsBundle:     sb,
	}, nil
}

// SetEndpoint updates the control plane endpoint.
func (g *ConfigGenerator) SetEndpoint(endpoint string) {
	g.endpoint = endpoint
}

// GenerateControlPlaneConfig generates the configuration for a control plane node.
func (g *ConfigGenerator) GenerateControlPlaneConfig(san []string) ([]byte, error) {
	vc, err := talos_config.ParseContractFromVersion(g.talosVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse version contract: %w", err)
	}

	opts := []generate.Option{
		generate.WithVersionContract(vc),
		generate.WithSecretsBundle(g.secretsBundle),
		generate.WithAdditionalSubjectAltNames(san),
		generate.WithInstallDisk("/dev/sda"),
	}

	// Disable CNI if requested (required for Cilium/external CNI)
	if g.cniType == "none" || g.cniType == "cilium" {
		opts = append(opts, generate.WithClusterCNIConfig(&v1alpha1.CNIConfig{CNIName: "none"}))
	}

	// Add Registry Mirrors
	for _, m := range g.registryMirrors {
		opts = append(opts, generate.WithRegistryMirror(m.Endpoint, m.Mirrors...))
	}

	input, err := generate.NewInput(
		g.clusterName,
		g.endpoint,
		g.kubernetesVersion,
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create input: %w", err)
	}

	p, err := input.Config(machine.TypeControlPlane)
	if err != nil {
		return nil, fmt.Errorf("failed to generate control plane config: %w", err)
	}

	// Enable external cloud provider
	p, err = p.PatchV1Alpha1(func(cfg *v1alpha1.Config) error {
		if cfg.ClusterConfig == nil {
			cfg.ClusterConfig = &v1alpha1.ClusterConfig{}
		}
		if cfg.ClusterConfig.ExternalCloudProviderConfig == nil {
			cfg.ClusterConfig.ExternalCloudProviderConfig = &v1alpha1.ExternalCloudProviderConfig{}
		}
		enabled := true
		cfg.ClusterConfig.ExternalCloudProviderConfig.ExternalEnabled = &enabled
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to patch control plane config: %w", err)
	}

	bytes, err := p.Bytes()
	if err != nil {
		return nil, err
	}

	return stripComments(bytes), nil
}

// GenerateWorkerConfig generates the configuration for a worker node.
func (g *ConfigGenerator) GenerateWorkerConfig() ([]byte, error) {
	vc, err := talos_config.ParseContractFromVersion(g.talosVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse version contract: %w", err)
	}

	opts := []generate.Option{
		generate.WithVersionContract(vc),
		generate.WithSecretsBundle(g.secretsBundle),
		generate.WithInstallDisk("/dev/sda"),
	}

	// Disable CNI if requested
	if g.cniType == "none" || g.cniType == "cilium" {
		opts = append(opts, generate.WithClusterCNIConfig(&v1alpha1.CNIConfig{CNIName: "none"}))
	}

	// Add Registry Mirrors
	for _, m := range g.registryMirrors {
		opts = append(opts, generate.WithRegistryMirror(m.Endpoint, m.Mirrors...))
	}

	input, err := generate.NewInput(
		g.clusterName,
		g.endpoint,
		g.kubernetesVersion,
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create input: %w", err)
	}

	p, err := input.Config(machine.TypeWorker)
	if err != nil {
		return nil, fmt.Errorf("failed to generate worker config: %w", err)
	}

	// Enable external cloud provider
	p, err = p.PatchV1Alpha1(func(cfg *v1alpha1.Config) error {
		if cfg.ClusterConfig == nil {
			cfg.ClusterConfig = &v1alpha1.ClusterConfig{}
		}
		if cfg.ClusterConfig.ExternalCloudProviderConfig == nil {
			cfg.ClusterConfig.ExternalCloudProviderConfig = &v1alpha1.ExternalCloudProviderConfig{}
		}
		enabled := true
		cfg.ClusterConfig.ExternalCloudProviderConfig.ExternalEnabled = &enabled
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to patch worker config: %w", err)
	}

	bytes, err := p.Bytes()
	if err != nil {
		return nil, err
	}

	return stripComments(bytes), nil
}

// GetClientConfig returns the talosconfig for the cluster.
func (g *ConfigGenerator) GetClientConfig() ([]byte, error) {
	vc, err := talos_config.ParseContractFromVersion(g.talosVersion)
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

// GetKubeconfig retrieves the admin kubeconfig from a control plane node.
func (g *ConfigGenerator) GetKubeconfig(ctx context.Context, nodeIP string) ([]byte, error) {
	clientCfgBytes, err := g.GetClientConfig()
	if err != nil {
		return nil, err
	}

	cfg, err := clientconfig.FromBytes(clientCfgBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to load client config: %w", err)
	}

	talosClient, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(nodeIP))
	if err != nil {
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	kubeconfig, err := talosClient.Kubeconfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	return kubeconfig, nil
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
