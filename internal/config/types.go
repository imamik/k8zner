// Package config defines the configuration structure and methods for the application.
package config

// Config holds the application configuration.
type Config struct {
	ClusterName string   `mapstructure:"cluster_name" yaml:"cluster_name"`
	HCloudToken string   `mapstructure:"hcloud_token" yaml:"hcloud_token"`
	Location    string   `mapstructure:"location" yaml:"location"` // e.g. nbg1, fsn1, hel1
	SSHKeys     []string `mapstructure:"ssh_keys" yaml:"ssh_keys"` // List of SSH key names/IDs
	TestID      string   `mapstructure:"test_id" yaml:"test_id"`   // Optional test ID for E2E test resource tracking

	// Cluster Access Mode
	// Defines how the cluster is accessed externally (public or private IPs).
	// Default: "public"
	ClusterAccess string `mapstructure:"cluster_access" yaml:"cluster_access"`

	// GracefulDestroy enables graceful node drain before deletion.
	// Default: false
	GracefulDestroy bool `mapstructure:"graceful_destroy" yaml:"graceful_destroy"`

	// HealthcheckEnabled enables cluster health checks.
	// Default: true
	HealthcheckEnabled *bool `mapstructure:"healthcheck_enabled" yaml:"healthcheck_enabled"`

	// DeleteProtection prevents accidental cluster deletion.
	// Default: false
	DeleteProtection bool `mapstructure:"delete_protection" yaml:"delete_protection"`

	// KubeconfigPath specifies where to write the kubeconfig file.
	KubeconfigPath string `mapstructure:"kubeconfig_path" yaml:"kubeconfig_path"`

	// TalosconfigPath specifies where to write the talosconfig file.
	TalosconfigPath string `mapstructure:"talosconfig_path" yaml:"talosconfig_path"`

	// TalosctlVersionCheckEnabled verifies talosctl version compatibility.
	// Default: true
	TalosctlVersionCheckEnabled *bool `mapstructure:"talosctl_version_check_enabled" yaml:"talosctl_version_check_enabled"`

	// TalosctlRetryCount specifies retry attempts for talosctl operations.
	// Default: 5
	TalosctlRetryCount int `mapstructure:"talosctl_retry_count" yaml:"talosctl_retry_count"`

	// Network Configuration
	Network NetworkConfig `mapstructure:"network" yaml:"network"`

	// Firewall Configuration
	Firewall FirewallConfig `mapstructure:"firewall" yaml:"firewall"`

	// Control Plane
	ControlPlane ControlPlaneConfig `mapstructure:"control_plane" yaml:"control_plane"`

	// Workers
	Workers []WorkerNodePool `mapstructure:"workers" yaml:"workers"`

	// Load Balancer (Ingress)
	Ingress IngressConfig `mapstructure:"ingress" yaml:"ingress"`

	// Talos Configuration
	Talos TalosConfig `mapstructure:"talos" yaml:"talos"`

	// Kubernetes Configuration
	Kubernetes KubernetesConfig `mapstructure:"kubernetes" yaml:"kubernetes"`

	// Addons Configuration
	Addons AddonsConfig `mapstructure:"addons" yaml:"addons"`
}

// NetworkConfig defines the network-related configuration.
type NetworkConfig struct {
	IPv4CIDR              string `mapstructure:"ipv4_cidr" yaml:"ipv4_cidr"`
	NodeIPv4CIDR          string `mapstructure:"node_ipv4_cidr" yaml:"node_ipv4_cidr"`
	NodeIPv4SubnetMask    int    `mapstructure:"node_ipv4_subnet_mask_size" yaml:"node_ipv4_subnet_mask_size"`
	ServiceIPv4CIDR       string `mapstructure:"service_ipv4_cidr" yaml:"service_ipv4_cidr"`
	PodIPv4CIDR           string `mapstructure:"pod_ipv4_cidr" yaml:"pod_ipv4_cidr"`
	NativeRoutingIPv4CIDR string `mapstructure:"native_routing_ipv4_cidr" yaml:"native_routing_ipv4_cidr"`
	Zone                  string `mapstructure:"zone" yaml:"zone"` // e.g. eu-central
}

// FirewallConfig defines the firewall-related configuration.
type FirewallConfig struct {
	UseCurrentIPv4 *bool          `mapstructure:"use_current_ipv4" yaml:"use_current_ipv4"`
	UseCurrentIPv6 *bool          `mapstructure:"use_current_ipv6" yaml:"use_current_ipv6"`
	APISource      []string       `mapstructure:"api_source" yaml:"api_source"`
	KubeAPISource  []string       `mapstructure:"kube_api_source" yaml:"kube_api_source"`
	TalosAPISource []string       `mapstructure:"talos_api_source" yaml:"talos_api_source"`
	ExtraRules     []FirewallRule `mapstructure:"extra_rules" yaml:"extra_rules"`
}

// FirewallRule defines a single firewall rule.
type FirewallRule struct {
	Description    string   `mapstructure:"description" yaml:"description"`
	Direction      string   `mapstructure:"direction" yaml:"direction"` // in, out
	SourceIPs      []string `mapstructure:"source_ips" yaml:"source_ips"`
	DestinationIPs []string `mapstructure:"destination_ips" yaml:"destination_ips"`
	Protocol       string   `mapstructure:"protocol" yaml:"protocol"` // tcp, udp, icmp, gre, esp
	Port           string   `mapstructure:"port" yaml:"port"`
}

// ControlPlaneConfig defines the control plane configuration.
type ControlPlaneConfig struct {
	NodePools []ControlPlaneNodePool `mapstructure:"nodepools" yaml:"nodepools"`
}

// ControlPlaneNodePool defines a node pool for the control plane.
type ControlPlaneNodePool struct {
	Name       string            `mapstructure:"name" yaml:"name"`
	Location   string            `mapstructure:"location" yaml:"location"`
	ServerType string            `mapstructure:"type" yaml:"type"`
	Count      int               `mapstructure:"count" yaml:"count"`
	Labels     map[string]string `mapstructure:"labels" yaml:"labels"`
	Image      string            `mapstructure:"image" yaml:"image"` // Optional override
	Backups    bool              `mapstructure:"backups" yaml:"backups"`
}

// WorkerNodePool defines a node pool for workers.
type WorkerNodePool struct {
	Name           string            `mapstructure:"name" yaml:"name"`
	Location       string            `mapstructure:"location" yaml:"location"`
	ServerType     string            `mapstructure:"type" yaml:"type"`
	Count          int               `mapstructure:"count" yaml:"count"`
	Labels         map[string]string `mapstructure:"labels" yaml:"labels"`
	PlacementGroup bool              `mapstructure:"placement_group" yaml:"placement_group"`
	Image          string            `mapstructure:"image" yaml:"image"` // Optional override
	Backups        bool              `mapstructure:"backups" yaml:"backups"`
}

// IngressConfig defines the ingress load balancer configuration.
// The ingress LB is created automatically by Traefik's LoadBalancer Service via CCM.
// This config only controls the LB type used for cost estimation.
type IngressConfig struct {
	LoadBalancerType string `mapstructure:"load_balancer_type" yaml:"load_balancer_type"`
}

// TalosConfig defines the Talos-specific configuration.
type TalosConfig struct {
	Version     string        `mapstructure:"version" yaml:"version"`
	SchematicID string        `mapstructure:"schematic_id" yaml:"schematic_id"`
	Extensions  []string      `mapstructure:"extensions" yaml:"extensions"`
	Upgrade     UpgradeConfig `mapstructure:"upgrade" yaml:"upgrade"`

	// ImageBuilder configures the server used for building Talos images.
	ImageBuilder ImageBuilderConfig `mapstructure:"image_builder" yaml:"image_builder"`

	// Machine-level configuration options (talos_* variables)
	Machine TalosMachineConfig `mapstructure:"machine" yaml:"machine"`
}

// TalosMachineConfig defines Talos machine-level configuration.
type TalosMachineConfig struct {
	// Disk Encryption (LUKS2 with nodeID key)
	StateEncryption     *bool `mapstructure:"state_encryption" yaml:"state_encryption"`
	EphemeralEncryption *bool `mapstructure:"ephemeral_encryption" yaml:"ephemeral_encryption"`

	// Network Configuration
	IPv6Enabled       *bool `mapstructure:"ipv6_enabled" yaml:"ipv6_enabled"`
	PublicIPv4Enabled *bool `mapstructure:"public_ipv4_enabled" yaml:"public_ipv4_enabled"`
	PublicIPv6Enabled *bool `mapstructure:"public_ipv6_enabled" yaml:"public_ipv6_enabled"`

	// DNS Configuration
	CoreDNSEnabled *bool `mapstructure:"coredns_enabled" yaml:"coredns_enabled"`

	// Discovery Services
	DiscoveryKubernetesEnabled *bool `mapstructure:"discovery_kubernetes_enabled" yaml:"discovery_kubernetes_enabled"`
	DiscoveryServiceEnabled    *bool `mapstructure:"discovery_service_enabled" yaml:"discovery_service_enabled"`

	// Config Apply Mode (auto, reboot, no_reboot, staged)
	ConfigApplyMode string `mapstructure:"config_apply_mode" yaml:"config_apply_mode"`
}

// UpgradeConfig defines the upgrade-related configuration.
type UpgradeConfig struct {
	Debug      bool   `mapstructure:"debug" yaml:"debug"`
	Force      bool   `mapstructure:"force" yaml:"force"`
	Insecure   bool   `mapstructure:"insecure" yaml:"insecure"`
	RebootMode string `mapstructure:"reboot_mode" yaml:"reboot_mode"`
	Stage      bool   `mapstructure:"stage" yaml:"stage"`
}

// ImageBuilderConfig defines the server configuration for building Talos images.
type ImageBuilderConfig struct {
	AMD64 ImageBuilderArchConfig `mapstructure:"amd64" yaml:"amd64"`
	ARM64 ImageBuilderArchConfig `mapstructure:"arm64" yaml:"arm64"`
}

// ImageBuilderArchConfig defines architecture-specific image builder settings.
type ImageBuilderArchConfig struct {
	// ServerType specifies the Hetzner server type for building images.
	// Defaults: AMD64="cpx11", ARM64="cax11"
	ServerType string `mapstructure:"server_type" yaml:"server_type"`

	// ServerLocation specifies the Hetzner location for the build server.
	// Defaults: AMD64="ash", ARM64="nbg1"
	ServerLocation string `mapstructure:"server_location" yaml:"server_location"`
}

// KubernetesConfig defines the Kubernetes-specific configuration.
type KubernetesConfig struct {
	Version string `mapstructure:"version" yaml:"version"`

	// Cluster-level configuration
	Domain              string `mapstructure:"domain" yaml:"domain"`
	AllowSchedulingOnCP *bool  `mapstructure:"allow_scheduling_on_control_planes" yaml:"allow_scheduling_on_control_planes"`

	// API Server Load Balancer for high availability.
	APILoadBalancerEnabled bool `mapstructure:"api_load_balancer_enabled" yaml:"api_load_balancer_enabled"`

	// API Server Load Balancer Public Network enables the public interface.
	APILoadBalancerPublicNetwork *bool `mapstructure:"api_load_balancer_public_network" yaml:"api_load_balancer_public_network"`
}

// WorkerCount returns the total number of worker nodes across all pools.
func (c *Config) WorkerCount() int {
	count := 0
	for _, pool := range c.Workers {
		count += pool.Count
	}
	return count
}

// IsPrivateFirst returns true if the cluster should use private-first architecture.
func (c *Config) IsPrivateFirst() bool {
	return c.ClusterAccess == "private"
}

// ShouldEnablePublicIPv4 returns whether servers should have public IPv4 enabled.
// Returns false for private-first mode, true otherwise.
func (c *Config) ShouldEnablePublicIPv4() bool {
	// If explicitly configured in Talos machine config, use that
	if c.Talos.Machine.PublicIPv4Enabled != nil {
		return *c.Talos.Machine.PublicIPv4Enabled
	}
	// Otherwise, disable in private-first mode
	return !c.IsPrivateFirst()
}

// ShouldEnablePublicIPv6 returns whether servers should have public IPv6 enabled.
// Returns true by default (for debugging/fallback access).
func (c *Config) ShouldEnablePublicIPv6() bool {
	// If explicitly configured in Talos machine config, use that
	if c.Talos.Machine.PublicIPv6Enabled != nil {
		return *c.Talos.Machine.PublicIPv6Enabled
	}
	// Default to enabled (IPv6 is free and useful for debugging)
	return true
}
