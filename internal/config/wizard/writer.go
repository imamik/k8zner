package wizard

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/imamik/k8zner/internal/config"
	"gopkg.in/yaml.v3"
)

// Function variable for dependency injection in tests.
var confirmOverwrite = defaultConfirmOverwrite

// WriteConfig writes the config to a YAML file with a descriptive header.
// If fullOutput is false, only essential non-default values are written.
func WriteConfig(cfg *config.Config, outputPath string, fullOutput bool) error {
	var yamlBytes []byte
	var err error

	if fullOutput {
		yamlBytes, err = yaml.Marshal(cfg)
	} else {
		// Create minimal config with only essential fields
		minCfg := buildMinimalConfig(cfg)
		yamlBytes, err = yaml.Marshal(minCfg)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(generateHeader(outputPath, fullOutput))
	sb.WriteString("\n")
	sb.Write(yamlBytes)

	if err := os.WriteFile(outputPath, []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// MinimalConfig represents the minimal configuration for YAML output.
// Only contains fields that are essential or explicitly set by the user.
type MinimalConfig struct {
	ClusterName   string                    `yaml:"cluster_name"`
	Location      string                    `yaml:"location"`
	SSHKeys       []string                  `yaml:"ssh_keys,omitempty"`
	ControlPlane  MinimalControlPlaneConfig `yaml:"control_plane"`
	Workers       []MinimalWorkerNodePool   `yaml:"workers,omitempty"`
	Talos         MinimalTalosConfig        `yaml:"talos"`
	Kubernetes    MinimalKubernetesConfig   `yaml:"kubernetes"`
	Addons        MinimalAddonsConfig       `yaml:"addons,omitempty"`
	Network       *MinimalNetworkConfig     `yaml:"network,omitempty"`
	ClusterAccess string                    `yaml:"cluster_access,omitempty"`
}

// MinimalControlPlaneConfig contains essential control plane settings.
type MinimalControlPlaneConfig struct {
	NodePools []MinimalControlPlaneNodePool `yaml:"nodepools"`
}

// MinimalControlPlaneNodePool contains essential node pool settings.
type MinimalControlPlaneNodePool struct {
	Name       string `yaml:"name"`
	ServerType string `yaml:"type"`
	Count      int    `yaml:"count"`
}

// MinimalWorkerNodePool contains essential worker settings.
type MinimalWorkerNodePool struct {
	Name       string `yaml:"name"`
	ServerType string `yaml:"type"`
	Count      int    `yaml:"count"`
}

// MinimalTalosConfig contains essential Talos settings.
type MinimalTalosConfig struct {
	Version        string             `yaml:"version"`
	MachineEncrypt *MinimalEncryption `yaml:"machine,omitempty"`
}

// MinimalEncryption contains encryption settings if enabled.
type MinimalEncryption struct {
	StateEncryption     *bool `yaml:"state_encryption,omitempty"`
	EphemeralEncryption *bool `yaml:"ephemeral_encryption,omitempty"`
}

// MinimalKubernetesConfig contains essential Kubernetes settings.
type MinimalKubernetesConfig struct {
	Version             string `yaml:"version"`
	AllowSchedulingOnCP *bool  `yaml:"allow_scheduling_on_control_planes,omitempty"`
}

// MinimalAddonsConfig contains only enabled addons.
type MinimalAddonsConfig struct {
	Cilium        *MinimalCiliumConfig `yaml:"cilium,omitempty"`
	CCM           *MinimalAddon        `yaml:"ccm,omitempty"`
	CSI           *MinimalAddon        `yaml:"csi,omitempty"`
	MetricsServer *MinimalAddon        `yaml:"metrics_server,omitempty"`
	CertManager   *MinimalAddon        `yaml:"cert_manager,omitempty"`
	IngressNginx  *MinimalAddon        `yaml:"ingress_nginx,omitempty"`
	Longhorn      *MinimalAddon        `yaml:"longhorn,omitempty"`
	ArgoCD        *MinimalAddon        `yaml:"argocd,omitempty"`
}

// MinimalAddon represents a simple enabled addon.
type MinimalAddon struct {
	Enabled bool `yaml:"enabled"`
}

// MinimalCiliumConfig contains Cilium settings when enabled.
type MinimalCiliumConfig struct {
	Enabled            bool   `yaml:"enabled"`
	EncryptionEnabled  bool   `yaml:"encryption_enabled,omitempty"`
	EncryptionType     string `yaml:"encryption_type,omitempty"`
	HubbleEnabled      bool   `yaml:"hubble_enabled,omitempty"`
	HubbleRelayEnabled bool   `yaml:"hubble_relay_enabled,omitempty"`
	GatewayAPIEnabled  bool   `yaml:"gateway_api_enabled,omitempty"`
}

// MinimalNetworkConfig contains network settings if customized.
type MinimalNetworkConfig struct {
	IPv4CIDR        string `yaml:"ipv4_cidr,omitempty"`
	PodIPv4CIDR     string `yaml:"pod_ipv4_cidr,omitempty"`
	ServiceIPv4CIDR string `yaml:"service_ipv4_cidr,omitempty"`
}

// buildMinimalConfig creates a minimal config from the full config.
func buildMinimalConfig(cfg *config.Config) *MinimalConfig {
	minCfg := &MinimalConfig{
		ClusterName: cfg.ClusterName,
		Location:    cfg.Location,
		SSHKeys:     cfg.SSHKeys,
		Talos: MinimalTalosConfig{
			Version: cfg.Talos.Version,
		},
		Kubernetes: MinimalKubernetesConfig{
			Version:             cfg.Kubernetes.Version,
			AllowSchedulingOnCP: cfg.Kubernetes.AllowSchedulingOnCP,
		},
	}

	// Control plane
	for _, np := range cfg.ControlPlane.NodePools {
		minCfg.ControlPlane.NodePools = append(minCfg.ControlPlane.NodePools, MinimalControlPlaneNodePool{
			Name:       np.Name,
			ServerType: np.ServerType,
			Count:      np.Count,
		})
	}

	// Workers
	for _, wp := range cfg.Workers {
		minCfg.Workers = append(minCfg.Workers, MinimalWorkerNodePool{
			Name:       wp.Name,
			ServerType: wp.ServerType,
			Count:      wp.Count,
		})
	}

	// Addons - only include enabled ones
	addons := MinimalAddonsConfig{}
	hasAddons := false

	if cfg.Addons.Cilium.Enabled {
		addons.Cilium = &MinimalCiliumConfig{
			Enabled:            true,
			EncryptionEnabled:  cfg.Addons.Cilium.EncryptionEnabled,
			EncryptionType:     cfg.Addons.Cilium.EncryptionType,
			HubbleEnabled:      cfg.Addons.Cilium.HubbleEnabled,
			HubbleRelayEnabled: cfg.Addons.Cilium.HubbleRelayEnabled,
			GatewayAPIEnabled:  cfg.Addons.Cilium.GatewayAPIEnabled,
		}
		hasAddons = true
	}
	if cfg.Addons.CCM.Enabled {
		addons.CCM = &MinimalAddon{Enabled: true}
		hasAddons = true
	}
	if cfg.Addons.CSI.Enabled {
		addons.CSI = &MinimalAddon{Enabled: true}
		hasAddons = true
	}
	if cfg.Addons.MetricsServer.Enabled {
		addons.MetricsServer = &MinimalAddon{Enabled: true}
		hasAddons = true
	}
	if cfg.Addons.CertManager.Enabled {
		addons.CertManager = &MinimalAddon{Enabled: true}
		hasAddons = true
	}
	if cfg.Addons.IngressNginx.Enabled {
		addons.IngressNginx = &MinimalAddon{Enabled: true}
		hasAddons = true
	}
	if cfg.Addons.Longhorn.Enabled {
		addons.Longhorn = &MinimalAddon{Enabled: true}
		hasAddons = true
	}
	if cfg.Addons.ArgoCD.Enabled {
		addons.ArgoCD = &MinimalAddon{Enabled: true}
		hasAddons = true
	}

	if hasAddons {
		minCfg.Addons = addons
	}

	// Network config - only if customized
	if cfg.Network.IPv4CIDR != "" || cfg.Network.PodIPv4CIDR != "" || cfg.Network.ServiceIPv4CIDR != "" {
		minCfg.Network = &MinimalNetworkConfig{
			IPv4CIDR:        cfg.Network.IPv4CIDR,
			PodIPv4CIDR:     cfg.Network.PodIPv4CIDR,
			ServiceIPv4CIDR: cfg.Network.ServiceIPv4CIDR,
		}
	}

	// Encryption settings
	if cfg.Talos.Machine.StateEncryption != nil || cfg.Talos.Machine.EphemeralEncryption != nil {
		minCfg.Talos.MachineEncrypt = &MinimalEncryption{
			StateEncryption:     cfg.Talos.Machine.StateEncryption,
			EphemeralEncryption: cfg.Talos.Machine.EphemeralEncryption,
		}
	}

	// Cluster access
	if cfg.ClusterAccess != "" && cfg.ClusterAccess != "public" {
		minCfg.ClusterAccess = cfg.ClusterAccess
	}

	return minCfg
}

// generateHeader creates the YAML file header comment.
func generateHeader(outputPath string, fullOutput bool) string {
	mode := "minimal"
	note := "\n# Note: This is a minimal config. Use --full flag for all options."
	if fullOutput {
		mode = "full"
		note = ""
	}
	return fmt.Sprintf(`# k8zner cluster configuration
# Generated by: k8zner init
# Generated at: %s
# Output mode: %s
# Docs: https://github.com/imamik/k8zner%s
#
# Required environment variable:
#   HCLOUD_TOKEN - Your Hetzner Cloud API token
#
# Usage:
#   export HCLOUD_TOKEN=<your-token>
#   k8zner apply -c %s
`, time.Now().Format(time.RFC3339), mode, note, outputPath)
}

// FileExists checks if a file exists at the given path.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ConfirmOverwrite prompts the user to confirm overwriting an existing file.
func ConfirmOverwrite(path string) (bool, error) {
	return confirmOverwrite(path)
}

// defaultConfirmOverwrite is the default implementation that prompts via stdin.
func defaultConfirmOverwrite(path string) (bool, error) {
	fmt.Printf("\nFile already exists: %s\n", path)
	fmt.Print("Overwrite? (y/n): ")

	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		return false, err
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes", nil
}
