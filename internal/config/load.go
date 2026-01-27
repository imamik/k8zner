package config

import (
	"fmt"
	"os"
	"regexp"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

// hcloudS3URLRegex matches Hetzner Object Storage URLs
// Format: bucket.region.your-objectstorage.com or https://bucket.region.your-objectstorage.com
var hcloudS3URLRegex = regexp.MustCompile(`^(?:https?://)?([^.]+)\.([^.]+)\.your-objectstorage\.com\.?$`)

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

	// Default cluster access mode to "public" (matches Terraform behavior)
	if cfg.ClusterAccess == "" {
		cfg.ClusterAccess = "public"
	}

	// Default OIDC claim settings (matches Terraform behavior)
	if cfg.Kubernetes.OIDC.Enabled {
		if cfg.Kubernetes.OIDC.UsernameClaim == "" {
			cfg.Kubernetes.OIDC.UsernameClaim = "sub"
		}
		if cfg.Kubernetes.OIDC.GroupsClaim == "" {
			cfg.Kubernetes.OIDC.GroupsClaim = "groups"
		}
	}

	// Default CCM to enabled (matches Terraform behavior and provides cloud integration)
	if !cfg.Addons.CCM.Enabled {
		cfg.Addons.CCM.Enabled = shouldEnableCCMByDefault(rawConfig)
	}

	// Default Gateway API CRDs to enabled (matches Terraform behavior)
	if !cfg.Addons.GatewayAPICRDs.Enabled {
		cfg.Addons.GatewayAPICRDs.Enabled = shouldEnableAddonByDefault(rawConfig, "gateway_api_crds")
	}

	// Default Prometheus Operator CRDs to enabled (matches Terraform behavior)
	if !cfg.Addons.PrometheusOperatorCRDs.Enabled {
		cfg.Addons.PrometheusOperatorCRDs.Enabled = shouldEnableAddonByDefault(rawConfig, "prometheus_operator_crds")
	}

	// Default Talos CCM to enabled (matches Terraform behavior)
	if !cfg.Addons.TalosCCM.Enabled {
		cfg.Addons.TalosCCM.Enabled = shouldEnableAddonByDefault(rawConfig, "talos_ccm")
	}

	// Default Talos CCM version (matches Terraform default)
	if cfg.Addons.TalosCCM.Enabled && cfg.Addons.TalosCCM.Version == "" {
		cfg.Addons.TalosCCM.Version = "v1.11.0"
	}

	// Default image builder configuration (matches Terraform packer_* defaults)
	if cfg.Talos.ImageBuilder.AMD64.ServerType == "" {
		cfg.Talos.ImageBuilder.AMD64.ServerType = "cpx11"
	}
	if cfg.Talos.ImageBuilder.AMD64.ServerLocation == "" {
		cfg.Talos.ImageBuilder.AMD64.ServerLocation = "ash"
	}
	if cfg.Talos.ImageBuilder.ARM64.ServerType == "" {
		cfg.Talos.ImageBuilder.ARM64.ServerType = "cax11"
	}
	if cfg.Talos.ImageBuilder.ARM64.ServerLocation == "" {
		cfg.Talos.ImageBuilder.ARM64.ServerLocation = "nbg1"
	}

	// Parse Talos Backup S3 Hcloud URL if provided
	applyTalosBackupS3Defaults(&cfg)

	// Default ingress load balancer pool count to 1 if not specified
	for i := range cfg.IngressLoadBalancerPools {
		if cfg.IngressLoadBalancerPools[i].Count == 0 {
			cfg.IngressLoadBalancerPools[i].Count = 1
		}
	}

	// Apply auto-calculated defaults (matches Terraform behavior)
	applyAutoCalculatedDefaults(&cfg)

	// Apply Cloudflare environment variable overrides (following HCLOUD_TOKEN pattern)
	applyCloudflareEnvVars(&cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

// applyCloudflareEnvVars applies Cloudflare configuration from environment variables.
// This follows the same pattern as HCLOUD_TOKEN for consistency.
func applyCloudflareEnvVars(cfg *Config) {
	// CF_API_TOKEN takes precedence if config value is empty
	if cfg.Addons.Cloudflare.APIToken == "" {
		if token := os.Getenv("CF_API_TOKEN"); token != "" {
			cfg.Addons.Cloudflare.APIToken = token
		}
	}

	// CF_DOMAIN takes precedence if config value is empty
	if cfg.Addons.Cloudflare.Domain == "" {
		if domain := os.Getenv("CF_DOMAIN"); domain != "" {
			cfg.Addons.Cloudflare.Domain = domain
		}
	}
}

// shouldEnableCCMByDefault determines if CCM should be enabled when not explicitly configured.
// Returns true if the CCM enabled field was not explicitly set to false in the raw config.
func shouldEnableCCMByDefault(rawConfig map[string]interface{}) bool {
	return shouldEnableAddonByDefault(rawConfig, "ccm")
}

// shouldEnableAddonByDefault determines if an addon should be enabled when not explicitly configured.
// Returns true if the addon's enabled field was not explicitly set in the raw config.
func shouldEnableAddonByDefault(rawConfig map[string]interface{}, addonKey string) bool {
	addonsMap, ok := rawConfig["addons"].(map[string]interface{})
	if !ok {
		return true // No addons section, default to enabled
	}

	addonMap, ok := addonsMap[addonKey].(map[string]interface{})
	if !ok {
		return true // No addon section, default to enabled
	}

	_, explicitlySet := addonMap["enabled"]
	return !explicitlySet // Default to enabled if not explicitly set
}

// applyTalosBackupS3Defaults parses S3HcloudURL and sets derived values.
// This is a convenience feature matching terraform/talos_backup.tf
func applyTalosBackupS3Defaults(cfg *Config) {
	backup := &cfg.Addons.TalosBackup

	// Parse Hcloud URL if provided
	if backup.S3HcloudURL != "" {
		matches := hcloudS3URLRegex.FindStringSubmatch(backup.S3HcloudURL)
		if len(matches) == 3 {
			// Extract bucket and region from URL
			bucket := matches[1]
			region := matches[2]

			// Only set if not already configured (explicit config takes precedence)
			if backup.S3Bucket == "" {
				backup.S3Bucket = bucket
			}
			if backup.S3Region == "" {
				backup.S3Region = region
			}
			if backup.S3Endpoint == "" {
				backup.S3Endpoint = fmt.Sprintf("https://%s.your-objectstorage.com", region)
			}
		}
	}
}

// applyAutoCalculatedDefaults sets computed defaults based on cluster configuration.
// These match Terraform's local value calculations.
func applyAutoCalculatedDefaults(cfg *Config) {
	// Calculate schedulable worker count
	workerCount := 0
	for _, pool := range cfg.Workers {
		workerCount += pool.Count
	}

	// Metrics server replicas: 1 for ≤1 schedulable, 2 for >1
	if cfg.Addons.MetricsServer.Enabled && cfg.Addons.MetricsServer.Replicas == nil {
		replicas := 2
		if workerCount <= 1 {
			replicas = 1
		}
		cfg.Addons.MetricsServer.Replicas = &replicas
	}

	// Metrics server schedule on control plane: default true when no workers
	if cfg.Addons.MetricsServer.Enabled && cfg.Addons.MetricsServer.ScheduleOnControlPlane == nil {
		scheduleOnCP := workerCount == 0
		cfg.Addons.MetricsServer.ScheduleOnControlPlane = &scheduleOnCP
	}

	// Ingress NGINX replicas: 2 for <3 workers, 3 for ≥3 workers
	if cfg.Addons.IngressNginx.Enabled && cfg.Addons.IngressNginx.Replicas == nil {
		if cfg.Addons.IngressNginx.Kind != "DaemonSet" {
			replicas := 2
			if workerCount >= 3 {
				replicas = 3
			}
			cfg.Addons.IngressNginx.Replicas = &replicas
		}
	}

	// Firewall IP defaults: use current IPv4/IPv6 when cluster_access is public
	if cfg.ClusterAccess == "public" {
		if cfg.Firewall.UseCurrentIPv4 == nil {
			useIPv4 := true
			cfg.Firewall.UseCurrentIPv4 = &useIPv4
		}
		if cfg.Firewall.UseCurrentIPv6 == nil {
			useIPv6 := true
			cfg.Firewall.UseCurrentIPv6 = &useIPv6
		}
	}
}
