package config

import (
	"fmt"
	"os"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

// LoadFile reads and parses the configuration from a YAML file.
func LoadFile(path string) (*Config, error) {
	// #nosec G304
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

	// Set defaults
	if cfg.Network.IPv4CIDR == "" {
		cfg.Network.IPv4CIDR = "10.0.0.0/16"
	}
	if cfg.Network.Zone == "" {
		cfg.Network.Zone = "eu-central"
	}

	// Default CCM to enabled (matches Terraform behavior and provides cloud integration)
	if !cfg.Addons.CCM.Enabled {
		cfg.Addons.CCM.Enabled = shouldEnableCCMByDefault(rawConfig)
	}

	// Default CNI (Cilium) settings
	applyCNIDefaults(&cfg, rawConfig)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

// shouldEnableCCMByDefault determines if CCM should be enabled when not explicitly configured.
// Returns true if the CCM enabled field was not explicitly set to false in the raw config.
func shouldEnableCCMByDefault(rawConfig map[string]interface{}) bool {
	// Check if addons.ccm.enabled was explicitly set to false
	addonsMap, ok := rawConfig["addons"].(map[string]interface{})
	if !ok {
		return true // No addons section, default to enabled
	}

	ccmMap, ok := addonsMap["ccm"].(map[string]interface{})
	if !ok {
		return true // No ccm section, default to enabled
	}

	_, explicitlySet := ccmMap["enabled"]
	if !explicitlySet {
		return true // CCM section exists but enabled not set, default to enabled
	}

	return false // Explicitly set to false, respect it
}

// applyCNIDefaults sets sensible defaults for Cilium CNI configuration.
func applyCNIDefaults(cfg *Config, rawConfig map[string]interface{}) {
	cni := &cfg.Kubernetes.CNI

	// Default CNI to enabled unless explicitly disabled
	if !cni.Enabled {
		cni.Enabled = shouldEnableCNIByDefault(rawConfig)
	}

	// Default Helm version
	if cni.HelmVersion == "" {
		cni.HelmVersion = "1.18.5"
	}

	// Default to kube-proxy replacement (eBPF)
	if !cni.KubeProxyReplacement {
		cni.KubeProxyReplacement = shouldEnableKubeProxyReplacementByDefault(rawConfig)
	}

	// Default routing mode
	if cni.RoutingMode == "" {
		cni.RoutingMode = "native"
	}

	// Default BPF datapath mode
	if cni.BPFDatapathMode == "" {
		cni.BPFDatapathMode = "veth"
	}

	// Default encryption to enabled with WireGuard
	if !cni.Encryption.Enabled {
		cni.Encryption.Enabled = shouldEnableEncryptionByDefault(rawConfig)
	}
	if cni.Encryption.Type == "" {
		cni.Encryption.Type = "wireguard"
	}

	// IPSec defaults (only used when Type == "ipsec")
	if cni.Encryption.IPSec.Algorithm == "" {
		cni.Encryption.IPSec.Algorithm = "rfc4106(gcm(aes))"
	}
	if cni.Encryption.IPSec.KeySize == 0 {
		cni.Encryption.IPSec.KeySize = 256
	}
	if cni.Encryption.IPSec.KeyID == 0 {
		cni.Encryption.IPSec.KeyID = 1
	}

	// Gateway API defaults
	if cni.GatewayAPI.ExternalTrafficPolicy == "" {
		cni.GatewayAPI.ExternalTrafficPolicy = "Cluster"
	}
}

// shouldEnableCNIByDefault determines if CNI should be enabled when not explicitly configured.
func shouldEnableCNIByDefault(rawConfig map[string]interface{}) bool {
	kubernetesMap, ok := rawConfig["kubernetes"].(map[string]interface{})
	if !ok {
		return true // No kubernetes section, default to enabled
	}

	cniMap, ok := kubernetesMap["cni"].(map[string]interface{})
	if !ok {
		return true // No cni section, default to enabled
	}

	_, explicitlySet := cniMap["enabled"]
	if !explicitlySet {
		return true // CNI section exists but enabled not set, default to enabled
	}

	return false
}

// shouldEnableKubeProxyReplacementByDefault determines if kube-proxy replacement should be enabled.
func shouldEnableKubeProxyReplacementByDefault(rawConfig map[string]interface{}) bool {
	kubernetesMap, ok := rawConfig["kubernetes"].(map[string]interface{})
	if !ok {
		return true
	}

	cniMap, ok := kubernetesMap["cni"].(map[string]interface{})
	if !ok {
		return true
	}

	_, explicitlySet := cniMap["kube_proxy_replacement"]
	return !explicitlySet
}

// shouldEnableEncryptionByDefault determines if encryption should be enabled.
func shouldEnableEncryptionByDefault(rawConfig map[string]interface{}) bool {
	kubernetesMap, ok := rawConfig["kubernetes"].(map[string]interface{})
	if !ok {
		return true
	}

	cniMap, ok := kubernetesMap["cni"].(map[string]interface{})
	if !ok {
		return true
	}

	encryptionMap, ok := cniMap["encryption"].(map[string]interface{})
	if !ok {
		return true
	}

	_, explicitlySet := encryptionMap["enabled"]
	return !explicitlySet
}
