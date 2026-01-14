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
func SaveSecrets(path string, sb *secrets.Bundle) error {
	data, err := json.MarshalIndent(sb, "", "  ")
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
func (g *Generator) GenerateControlPlaneConfig(san []string, hostname string) ([]byte, error) {
	opts := []generate.Option{
		generate.WithAdditionalSubjectAltNames(san),
	}

	return g.generateConfig(machine.TypeControlPlane, hostname, opts...)
}

// GenerateWorkerConfig generates the configuration for a worker node.
// If hostname is provided, it will be set in the machine config.
func (g *Generator) GenerateWorkerConfig(hostname string) ([]byte, error) {
	return g.generateConfig(machine.TypeWorker, hostname)
}

// GenerateAutoscalerConfig generates the configuration for an autoscaler node pool.
// The configuration includes node labels and taints for the pool.
func (g *Generator) GenerateAutoscalerConfig(poolName string, labels map[string]string, taints []string) ([]byte, error) {
	bytes, err := g.generateConfig(machine.TypeWorker, "")
	if err != nil {
		return nil, err
	}

	// Apply autoscaler-specific patches (labels and taints)
	bytes, err = applyAutoscalerPatches(bytes, poolName, labels, taints)
	if err != nil {
		return nil, fmt.Errorf("failed to apply autoscaler patches: %w", err)
	}

	return bytes, nil
}

func (g *Generator) generateConfig(machineType machine.Type, hostname string, extraOpts ...generate.Option) ([]byte, error) {
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

	// Apply patches
	isControlPlane := machineType == machine.TypeControlPlane
	bytes, err = applyPatches(bytes, isControlPlane, hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to apply patches: %w", err)
	}

	return stripComments(bytes), nil
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

// applyPatches applies necessary patches for Hetzner Cloud and sets the hostname.
func applyPatches(configBytes []byte, isControlPlane bool, hostname string) ([]byte, error) {
	var configMap map[string]interface{}
	if err := yaml.Unmarshal(configBytes, &configMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 1. External cloud provider (Hetzner CCM)
	cluster := getOrCreate(configMap, "cluster")
	cluster["externalCloudProvider"] = map[string]interface{}{"enabled": true}

	machine := getOrCreate(configMap, "machine")
	kubelet := getOrCreate(machine, "kubelet")
	extraArgs := getOrCreate(kubelet, "extraArgs")
	extraArgs["cloud-provider"] = "external"

	if isControlPlane {
		cm := getOrCreate(cluster, "controllerManager")
		cmExtraArgs := getOrCreate(cm, "extraArgs")
		cmExtraArgs["cloud-provider"] = "external"
	}

	// 2. Hostname
	if hostname != "" {
		network := getOrCreate(machine, "network")
		network["hostname"] = hostname
	}

	return yaml.Marshal(configMap)
}

// applyAutoscalerPatches applies labels and taints for autoscaler node pools.
func applyAutoscalerPatches(configBytes []byte, poolName string, labels map[string]string, taints []string) ([]byte, error) {
	var configMap map[string]interface{}
	if err := yaml.Unmarshal(configBytes, &configMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	machine := getOrCreate(configMap, "machine")

	// Add node labels
	nodeLabels := getOrCreate(machine, "nodeLabels")
	nodeLabels["nodepool"] = poolName
	for k, v := range labels {
		nodeLabels[k] = v
	}

	// Add node taints
	if len(taints) > 0 {
		nodeTaints := getOrCreate(machine, "nodeTaints")
		for _, taint := range taints {
			// Parse taint format: "key=value:effect"
			parts := strings.SplitN(taint, ":", 2)
			if len(parts) != 2 {
				continue
			}
			keyValue := parts[0]
			effect := parts[1]

			kvParts := strings.SplitN(keyValue, "=", 2)
			if len(kvParts) != 2 {
				continue
			}
			key := kvParts[0]
			value := kvParts[1]

			nodeTaints[key] = fmt.Sprintf("%s:%s", value, effect)
		}
	}

	return yaml.Marshal(configMap)
}

func getOrCreate(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key].(map[string]interface{}); ok {
		return v
	}
	newMap := make(map[string]interface{})
	m[key] = newMap
	return newMap
}
