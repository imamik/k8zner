// Package talos provides functionality for generating and managing Talos Linux configurations.
package talos

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"gopkg.in/yaml.v3"
)

// SecretsBundle is a type alias for the Talos secrets bundle.
type SecretsBundle = secrets.Bundle

// Generator handles Talos configuration generation.
type Generator struct {
	clusterName       string
	kubernetesVersion string
	talosVersion      string
	endpoint          string
	secretsBundle     *secrets.Bundle
	machineOpts       *MachineConfigOptions
}

// NewGenerator creates a new Generator.
func NewGenerator(clusterName, kubernetesVersion, talosVersion, endpoint string, sb *secrets.Bundle) *Generator {
	// Strip 'v' prefix from kubernetesVersion if present
	// Talos machinery adds the 'v' prefix automatically, so we need to provide the version without it
	kubernetesVersion = strings.TrimPrefix(kubernetesVersion, "v")

	return &Generator{
		clusterName:       clusterName,
		kubernetesVersion: kubernetesVersion,
		talosVersion:      talosVersion,
		endpoint:          endpoint,
		secretsBundle:     sb,
		machineOpts:       &MachineConfigOptions{}, // Default empty options
	}
}

// SetMachineConfigOptions sets the machine configuration options.
// These options control disk encryption, network settings, and other machine-level config.
func (g *Generator) SetMachineConfigOptions(opts any) {
	if opts == nil {
		return
	}
	// Type assert to *MachineConfigOptions
	if machineOpts, ok := opts.(*MachineConfigOptions); ok && machineOpts != nil {
		g.machineOpts = machineOpts
	}
}

// LoadSecrets loads Talos secrets from a file.
func LoadSecrets(path string) (*secrets.Bundle, error) {
	sb, err := secrets.LoadBundle(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load secrets bundle: %w", err)
	}

	if sb == nil {
		return nil, fmt.Errorf("loaded secrets bundle is nil")
	}

	// Re-inject clock
	sb.Clock = secrets.NewFixedClock(time.Now())
	return sb, nil
}

// SaveSecrets saves Talos secrets to a file.
// Uses YAML format to match what Talos machinery's LoadBundle expects.
func SaveSecrets(path string, sb *secrets.Bundle) error {
	data, err := yaml.Marshal(sb)
	if err != nil {
		return fmt.Errorf("failed to marshal secrets bundle: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write secrets file: %w", err)
	}

	return nil
}

// NewSecrets creates a new Talos secrets bundle.
func NewSecrets(talosVersion string) (*secrets.Bundle, error) {
	vc, err := config.ParseContractFromVersion(talosVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse version contract: %w", err)
	}

	sb, err := secrets.NewBundle(secrets.NewFixedClock(time.Now()), vc)
	if err != nil {
		return nil, fmt.Errorf("failed to create secrets bundle: %w", err)
	}

	return sb, nil
}

// GetOrGenerateSecrets attempts to load secrets from path, or generates and saves them if they don't exist.
func GetOrGenerateSecrets(path string, talosVersion string) (*SecretsBundle, error) {
	if _, err := os.Stat(path); err == nil {
		return LoadSecrets(path)
	}

	sb, err := NewSecrets(talosVersion)
	if err != nil {
		return nil, err
	}

	if err := SaveSecrets(path, sb); err != nil {
		return nil, err
	}

	return sb, nil
}

// SetEndpoint updates the control plane endpoint.
func (g *Generator) SetEndpoint(endpoint string) {
	g.endpoint = endpoint
}

// GenerateControlPlaneConfig generates the configuration for a control plane node.
// If hostname is provided, it will be set in the machine config.
// serverID is the Hetzner server ID, used to set the nodeid label for CCM integration.
func (g *Generator) GenerateControlPlaneConfig(san []string, hostname string, serverID int64) ([]byte, error) {
	opts := []generate.Option{
		generate.WithAdditionalSubjectAltNames(san),
	}

	baseConfig, err := g.generateBaseConfig(machine.TypeControlPlane, opts...)
	if err != nil {
		return nil, err
	}

	// Build installer image URL
	installerImage := g.getInstallerImageURL()

	// Build and apply enhanced patch with all machine config options
	patch := buildControlPlanePatch(hostname, serverID, g.machineOpts, installerImage, san)
	return applyConfigPatch(baseConfig, patch)
}

// GenerateWorkerConfig generates the configuration for a worker node.
// If hostname is provided, it will be set in the machine config.
// serverID is the Hetzner server ID, used to set the nodeid label for CCM integration.
func (g *Generator) GenerateWorkerConfig(hostname string, serverID int64) ([]byte, error) {
	baseConfig, err := g.generateBaseConfig(machine.TypeWorker)
	if err != nil {
		return nil, err
	}

	// Build installer image URL
	installerImage := g.getInstallerImageURL()

	// Build and apply enhanced patch with all machine config options
	patch := buildWorkerPatch(hostname, serverID, g.machineOpts, installerImage, nil)
	return applyConfigPatch(baseConfig, patch)
}


// generateBaseConfig generates the base Talos config without custom patches.
// This is used as the foundation that patches are applied to.
func (g *Generator) generateBaseConfig(machineType machine.Type, extraOpts ...generate.Option) ([]byte, error) {
	vc, err := config.ParseContractFromVersion(g.talosVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse version contract: %w", err)
	}

	opts := []generate.Option{
		generate.WithVersionContract(vc),
		generate.WithSecretsBundle(g.secretsBundle),
		generate.WithInstallDisk("/dev/sda"), // Hetzner Cloud uses /dev/sda
	}
	opts = append(opts, extraOpts...)

	input, err := generate.NewInput(
		g.clusterName,
		g.endpoint,
		g.kubernetesVersion,
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create input: %w", err)
	}

	cfg, err := input.Config(machineType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate %s config: %w", machineType, err)
	}

	bytes, err := cfg.Bytes()
	if err != nil {
		return nil, err
	}

	return stripComments(bytes), nil
}

// getInstallerImageURL returns the Talos installer image URL.
// Uses factory.talos.dev if a schematic ID is configured, otherwise uses the default image.
func (g *Generator) getInstallerImageURL() string {
	if g.machineOpts != nil && g.machineOpts.SchematicID != "" {
		// Use factory.talos.dev with schematic ID for custom images with extensions
		return fmt.Sprintf("factory.talos.dev/installer/%s:%s", g.machineOpts.SchematicID, g.talosVersion)
	}
	// Default installer image
	return fmt.Sprintf("ghcr.io/siderolabs/installer:%s", g.talosVersion)
}

// applyConfigPatch applies a patch map to the base config using deep merge.
func applyConfigPatch(baseConfig []byte, patch map[string]any) ([]byte, error) {
	var configMap map[string]any
	if err := yaml.Unmarshal(baseConfig, &configMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal base config: %w", err)
	}

	// Deep merge the patch into the config
	deepMerge(configMap, patch)

	return yaml.Marshal(configMap)
}

// deepMerge recursively merges src into dst.
// For maps, it merges recursively. For other types, src overwrites dst.
func deepMerge(dst, src map[string]any) {
	for key, srcVal := range src {
		if dstVal, exists := dst[key]; exists {
			// Both exist, check if we should merge recursively
			srcMap, srcIsMap := srcVal.(map[string]any)
			dstMap, dstIsMap := dstVal.(map[string]any)
			if srcIsMap && dstIsMap {
				deepMerge(dstMap, srcMap)
				continue
			}
		}
		// Either key doesn't exist in dst, or types don't match for recursive merge
		dst[key] = srcVal
	}
}

// GetClientConfig returns the talosconfig for the cluster.
func (g *Generator) GetClientConfig() ([]byte, error) {
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
