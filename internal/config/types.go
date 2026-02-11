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
	// See: terraform/variables.tf cluster_access
	// Defines how the cluster is accessed externally (public or private IPs).
	// Default: "public"
	ClusterAccess string `mapstructure:"cluster_access" yaml:"cluster_access"`

	// GracefulDestroy enables graceful node drain before deletion.
	// See: terraform/variables.tf cluster_graceful_destroy
	// Default: false
	GracefulDestroy bool `mapstructure:"graceful_destroy" yaml:"graceful_destroy"`

	// HealthcheckEnabled enables cluster health checks.
	// See: terraform/variables.tf cluster_healthcheck_enabled
	// Default: true
	HealthcheckEnabled *bool `mapstructure:"healthcheck_enabled" yaml:"healthcheck_enabled"`

	// DeleteProtection prevents accidental cluster deletion.
	// See: terraform/variables.tf cluster_delete_protection
	// Default: false
	DeleteProtection bool `mapstructure:"delete_protection" yaml:"delete_protection"`

	// KubeconfigPath specifies where to write the kubeconfig file.
	// See: terraform/variables.tf cluster_kubeconfig_path
	KubeconfigPath string `mapstructure:"kubeconfig_path" yaml:"kubeconfig_path"`

	// TalosconfigPath specifies where to write the talosconfig file.
	// See: terraform/variables.tf cluster_talosconfig_path
	TalosconfigPath string `mapstructure:"talosconfig_path" yaml:"talosconfig_path"`

	// TalosctlVersionCheckEnabled verifies talosctl version compatibility.
	// See: terraform/variables.tf talosctl_version_check_enabled
	// Default: true
	TalosctlVersionCheckEnabled *bool `mapstructure:"talosctl_version_check_enabled" yaml:"talosctl_version_check_enabled"`

	// TalosctlRetryCount specifies retry attempts for talosctl operations.
	// See: terraform/variables.tf talosctl_retry_count
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

	// Autoscaler
	Autoscaler AutoscalerConfig `mapstructure:"autoscaler" yaml:"autoscaler"`

	// Load Balancer (Ingress)
	Ingress IngressConfig `mapstructure:"ingress" yaml:"ingress"`

	// Ingress Load Balancer Pools
	// See: terraform/variables.tf ingress_load_balancer_pools
	// Allows configuring multiple load balancer pools for different ingress needs.
	IngressLoadBalancerPools []IngressLoadBalancerPool `mapstructure:"ingress_load_balancer_pools" yaml:"ingress_load_balancer_pools"`

	// Talos Configuration
	Talos TalosConfig `mapstructure:"talos" yaml:"talos"`

	// Kubernetes Configuration
	Kubernetes KubernetesConfig `mapstructure:"kubernetes" yaml:"kubernetes"`

	// Addons Configuration
	Addons AddonsConfig `mapstructure:"addons" yaml:"addons"`

	// RDNS Configuration
	RDNS RDNSConfig `mapstructure:"rdns" yaml:"rdns"`
}

// NetworkConfig defines the network-related configuration.
type NetworkConfig struct {
	// ExistingID specifies an existing Hetzner network ID to use instead of creating one.
	// See: terraform/variables.tf hcloud_network_id
	ExistingID int `mapstructure:"id" yaml:"id"`

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
	ExistingID     int            `mapstructure:"id" yaml:"id"`
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
	Name        string            `mapstructure:"name" yaml:"name"`
	Location    string            `mapstructure:"location" yaml:"location"`
	ServerType  string            `mapstructure:"type" yaml:"type"`
	Count       int               `mapstructure:"count" yaml:"count"`
	Labels      map[string]string `mapstructure:"labels" yaml:"labels"`
	Annotations map[string]string `mapstructure:"annotations" yaml:"annotations"`
	Taints      []string          `mapstructure:"taints" yaml:"taints"`
	Image       string            `mapstructure:"image" yaml:"image"` // Optional override
	RDNS        string            `mapstructure:"rdns" yaml:"rdns"`
	RDNSIPv4    string            `mapstructure:"rdns_ipv4" yaml:"rdns_ipv4"`
	RDNSIPv6    string            `mapstructure:"rdns_ipv6" yaml:"rdns_ipv6"`

	// Backups enables Hetzner server backups.
	// See: terraform/variables.tf control_plane_nodepools.backups
	// Default: false
	Backups bool `mapstructure:"backups" yaml:"backups"`

	// KeepDisk keeps the disk when the server is deleted.
	// See: terraform/variables.tf control_plane_nodepools.keep_disk
	// Default: false
	KeepDisk bool `mapstructure:"keep_disk" yaml:"keep_disk"`

	// ConfigPatches allows applying raw Talos machine configuration patches.
	// See: terraform/variables.tf control_plane_config_patches
	ConfigPatches []string `mapstructure:"config_patches" yaml:"config_patches"`
}

// WorkerNodePool defines a node pool for workers.
type WorkerNodePool struct {
	Name           string            `mapstructure:"name" yaml:"name"`
	Location       string            `mapstructure:"location" yaml:"location"`
	ServerType     string            `mapstructure:"type" yaml:"type"`
	Count          int               `mapstructure:"count" yaml:"count"`
	Labels         map[string]string `mapstructure:"labels" yaml:"labels"`
	Annotations    map[string]string `mapstructure:"annotations" yaml:"annotations"`
	Taints         []string          `mapstructure:"taints" yaml:"taints"`
	PlacementGroup bool              `mapstructure:"placement_group" yaml:"placement_group"`
	Image          string            `mapstructure:"image" yaml:"image"` // Optional override
	RDNS           string            `mapstructure:"rdns" yaml:"rdns"`
	RDNSIPv4       string            `mapstructure:"rdns_ipv4" yaml:"rdns_ipv4"`
	RDNSIPv6       string            `mapstructure:"rdns_ipv6" yaml:"rdns_ipv6"`

	// Backups enables Hetzner server backups.
	// See: terraform/variables.tf worker_nodepools.backups
	// Default: false
	Backups bool `mapstructure:"backups" yaml:"backups"`

	// KeepDisk keeps the disk when the server is deleted.
	// See: terraform/variables.tf worker_nodepools.keep_disk
	// Default: false
	KeepDisk bool `mapstructure:"keep_disk" yaml:"keep_disk"`

	// ConfigPatches allows applying raw Talos machine configuration patches.
	// See: terraform/variables.tf worker_config_patches
	ConfigPatches []string `mapstructure:"config_patches" yaml:"config_patches"`
}

// AutoscalerConfig defines the autoscaler configuration.
type AutoscalerConfig struct {
	Enabled   bool                 `mapstructure:"enabled" yaml:"enabled"`
	NodePools []AutoscalerNodePool `mapstructure:"nodepools" yaml:"nodepools"`
}

// AutoscalerNodePool defines a node pool for the autoscaler.
type AutoscalerNodePool struct {
	Name        string            `mapstructure:"name" yaml:"name"`
	Location    string            `mapstructure:"location" yaml:"location"`
	Type        string            `mapstructure:"type" yaml:"type"`
	Min         int               `mapstructure:"min" yaml:"min"`
	Max         int               `mapstructure:"max" yaml:"max"`
	Labels      map[string]string `mapstructure:"labels" yaml:"labels"`
	Annotations map[string]string `mapstructure:"annotations" yaml:"annotations"`
	Taints      []string          `mapstructure:"taints" yaml:"taints"`
}

// IngressConfig defines the ingress (load balancer) configuration.
type IngressConfig struct {
	Enabled            bool   `mapstructure:"enabled" yaml:"enabled"`
	LoadBalancerType   string `mapstructure:"load_balancer_type" yaml:"load_balancer_type"`
	PublicNetwork      bool   `mapstructure:"public_network_enabled" yaml:"public_network_enabled"`
	Algorithm          string `mapstructure:"algorithm" yaml:"algorithm"`
	HealthCheckInt     int    `mapstructure:"health_check_interval" yaml:"health_check_interval"`
	HealthCheckRetry   int    `mapstructure:"health_check_retries" yaml:"health_check_retries"`
	HealthCheckTimeout int    `mapstructure:"health_check_timeout" yaml:"health_check_timeout"`
	RDNS               string `mapstructure:"rdns" yaml:"rdns"`
	RDNSIPv4           string `mapstructure:"rdns_ipv4" yaml:"rdns_ipv4"`
	RDNSIPv6           string `mapstructure:"rdns_ipv6" yaml:"rdns_ipv6"`
}

// IngressLoadBalancerPool defines a load balancer pool for ingress.
// See: terraform/variables.tf ingress_load_balancer_pools
type IngressLoadBalancerPool struct {
	// Name is a unique identifier for the load balancer pool.
	Name string `mapstructure:"name" yaml:"name"`

	// Location specifies the Hetzner location for this load balancer.
	Location string `mapstructure:"location" yaml:"location"`

	// Type specifies the load balancer type (e.g., lb11, lb21, lb31).
	Type string `mapstructure:"type" yaml:"type"`

	// Labels are custom labels to apply to the load balancer.
	Labels map[string]string `mapstructure:"labels" yaml:"labels"`

	// Count specifies the number of load balancers in this pool.
	// Default: 1
	Count int `mapstructure:"count" yaml:"count"`

	// TargetLabelSelector specifies node labels to target for this pool.
	TargetLabelSelector []string `mapstructure:"target_label_selector" yaml:"target_label_selector"`

	// LocalTraffic enables local traffic policy (externalTrafficPolicy=Local).
	// Default: false
	LocalTraffic bool `mapstructure:"local_traffic" yaml:"local_traffic"`

	// Algorithm specifies the load balancing algorithm.
	// Valid values: "round_robin", "least_connections"
	Algorithm string `mapstructure:"load_balancer_algorithm" yaml:"load_balancer_algorithm"`

	// PublicNetworkEnabled enables the public interface for this load balancer.
	PublicNetworkEnabled *bool `mapstructure:"public_network_enabled" yaml:"public_network_enabled"`

	// RDNS settings for the load balancer
	RDNS     string `mapstructure:"rdns" yaml:"rdns"`
	RDNSIPv4 string `mapstructure:"rdns_ipv4" yaml:"rdns_ipv4"`
	RDNSIPv6 string `mapstructure:"rdns_ipv6" yaml:"rdns_ipv6"`
}

// TalosConfig defines the Talos-specific configuration.
type TalosConfig struct {
	Version     string        `mapstructure:"version" yaml:"version"`
	SchematicID string        `mapstructure:"schematic_id" yaml:"schematic_id"`
	Extensions  []string      `mapstructure:"extensions" yaml:"extensions"`
	Upgrade     UpgradeConfig `mapstructure:"upgrade" yaml:"upgrade"`

	// ImageBuilder configures the server used for building Talos images.
	// See: terraform/variables.tf packer_amd64_builder, packer_arm64_builder
	ImageBuilder ImageBuilderConfig `mapstructure:"image_builder" yaml:"image_builder"`

	// Machine-level configuration options (matching Terraform talos_* variables)
	Machine TalosMachineConfig `mapstructure:"machine" yaml:"machine"`
}

// TalosMachineConfig defines Talos machine-level configuration.
// See: terraform/variables.tf talos_* variables
type TalosMachineConfig struct {
	// Disk Encryption (LUKS2 with nodeID key)
	// See: terraform/talos_config.tf local.talos_system_disk_encryption
	StateEncryption     *bool `mapstructure:"state_encryption" yaml:"state_encryption"`
	EphemeralEncryption *bool `mapstructure:"ephemeral_encryption" yaml:"ephemeral_encryption"`

	// Network Configuration
	// See: terraform/variables.tf talos_ipv6_enabled, talos_public_ipv4_enabled, etc.
	IPv6Enabled       *bool    `mapstructure:"ipv6_enabled" yaml:"ipv6_enabled"`
	PublicIPv4Enabled *bool    `mapstructure:"public_ipv4_enabled" yaml:"public_ipv4_enabled"`
	PublicIPv6Enabled *bool    `mapstructure:"public_ipv6_enabled" yaml:"public_ipv6_enabled"`
	Nameservers       []string `mapstructure:"nameservers" yaml:"nameservers"`
	TimeServers       []string `mapstructure:"time_servers" yaml:"time_servers"`
	ExtraRoutes       []string `mapstructure:"extra_routes" yaml:"extra_routes"`

	// DNS Configuration
	// See: terraform/variables.tf talos_extra_host_entries, talos_coredns_enabled
	ExtraHostEntries []TalosHostEntry `mapstructure:"extra_host_entries" yaml:"extra_host_entries"`
	CoreDNSEnabled   *bool            `mapstructure:"coredns_enabled" yaml:"coredns_enabled"`

	// Registry Configuration
	// See: terraform/variables.tf talos_registries
	Registries *TalosRegistryConfig `mapstructure:"registries" yaml:"registries"`

	// Kernel Configuration
	// See: terraform/variables.tf talos_extra_kernel_args, talos_kernel_modules, talos_sysctls_extra_args
	KernelArgs    []string            `mapstructure:"kernel_args" yaml:"kernel_args"`
	KernelModules []TalosKernelModule `mapstructure:"kernel_modules" yaml:"kernel_modules"`
	Sysctls       map[string]string   `mapstructure:"sysctls" yaml:"sysctls"`

	// Kubelet Configuration
	// See: terraform/variables.tf talos_kubelet_extra_mounts, kubernetes_kubelet_extra_args
	KubeletExtraMounts []TalosKubeletMount `mapstructure:"kubelet_extra_mounts" yaml:"kubelet_extra_mounts"`

	// Bootstrap Manifests
	// See: terraform/variables.tf talos_extra_inline_manifests, talos_extra_remote_manifests
	InlineManifests []TalosInlineManifest `mapstructure:"inline_manifests" yaml:"inline_manifests"`
	RemoteManifests []string              `mapstructure:"remote_manifests" yaml:"remote_manifests"`

	// Discovery Services
	// See: terraform/variables.tf talos_discovery_kubernetes_enabled, talos_discovery_service_enabled
	DiscoveryKubernetesEnabled *bool `mapstructure:"discovery_kubernetes_enabled" yaml:"discovery_kubernetes_enabled"`
	DiscoveryServiceEnabled    *bool `mapstructure:"discovery_service_enabled" yaml:"discovery_service_enabled"`

	// Logging
	// See: terraform/variables.tf talos_logging_destinations
	LoggingDestinations []TalosLoggingDestination `mapstructure:"logging_destinations" yaml:"logging_destinations"`

	// Config Apply Mode (auto, reboot, no_reboot, staged)
	// See: terraform/variables.tf talos_machine_configuration_apply_mode
	ConfigApplyMode string `mapstructure:"config_apply_mode" yaml:"config_apply_mode"`
}

// TalosHostEntry defines an extra host entry for /etc/hosts.
type TalosHostEntry struct {
	IP      string   `mapstructure:"ip" yaml:"ip"`
	Aliases []string `mapstructure:"aliases" yaml:"aliases"`
}

// TalosKernelModule defines a kernel module to load.
type TalosKernelModule struct {
	Name       string   `mapstructure:"name" yaml:"name"`
	Parameters []string `mapstructure:"parameters" yaml:"parameters"`
}

// TalosKubeletMount defines an extra mount for kubelet.
type TalosKubeletMount struct {
	Source      string   `mapstructure:"source" yaml:"source"`
	Destination string   `mapstructure:"destination" yaml:"destination"`
	Type        string   `mapstructure:"type" yaml:"type"`
	Options     []string `mapstructure:"options" yaml:"options"`
}

// TalosInlineManifest defines an inline Kubernetes manifest for bootstrap.
type TalosInlineManifest struct {
	Name     string `mapstructure:"name" yaml:"name"`
	Contents string `mapstructure:"contents" yaml:"contents"`
}

// TalosLoggingDestination defines a remote logging destination.
type TalosLoggingDestination struct {
	Endpoint  string            `mapstructure:"endpoint" yaml:"endpoint"`
	Format    string            `mapstructure:"format" yaml:"format"`
	ExtraTags map[string]string `mapstructure:"extra_tags" yaml:"extra_tags"`
}

// TalosRegistryConfig defines registry mirror configuration.
type TalosRegistryConfig struct {
	Mirrors map[string]TalosRegistryMirror `mapstructure:"mirrors" yaml:"mirrors"`
}

// TalosRegistryMirror defines a registry mirror.
type TalosRegistryMirror struct {
	Endpoints []string `mapstructure:"endpoints" yaml:"endpoints"`
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
// See: terraform/variables.tf packer_amd64_builder, packer_arm64_builder
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
	Version string     `mapstructure:"version" yaml:"version"`
	OIDC    OIDCConfig `mapstructure:"oidc" yaml:"oidc"`
	CNI     CNIConfig  `mapstructure:"cni" yaml:"cni"`

	// Cluster-level configuration
	// See: terraform/variables.tf cluster_domain, cluster_allow_scheduling_on_control_planes
	Domain              string `mapstructure:"domain" yaml:"domain"`
	AllowSchedulingOnCP *bool  `mapstructure:"allow_scheduling_on_control_planes" yaml:"allow_scheduling_on_control_planes"`

	// API Server Hostname
	// See: terraform/variables.tf kube_api_hostname
	// Specifies the hostname for external access to the Kubernetes API server.
	APIHostname string `mapstructure:"api_hostname" yaml:"api_hostname"`

	// API Server Load Balancer
	// See: terraform/variables.tf kube_api_load_balancer_enabled
	// Enables a load balancer for the Kubernetes API server for high availability.
	APILoadBalancerEnabled bool `mapstructure:"api_load_balancer_enabled" yaml:"api_load_balancer_enabled"`

	// API Server Load Balancer Public Network
	// See: terraform/variables.tf kube_api_load_balancer_public_network_enabled
	// Enables the public interface for the Kubernetes API load balancer.
	APILoadBalancerPublicNetwork *bool `mapstructure:"api_load_balancer_public_network" yaml:"api_load_balancer_public_network"`

	// API Server Admission Control
	// See: terraform/variables.tf kube_api_admission_control
	// List of admission control plugins to enable.
	AdmissionControl []AdmissionControlPlugin `mapstructure:"admission_control" yaml:"admission_control"`

	// API Server Configuration
	// See: terraform/variables.tf kube_api_admission_control, kube_api_extra_args
	APIServerExtraArgs map[string]string `mapstructure:"api_server_extra_args" yaml:"api_server_extra_args"`

	// Kubelet Configuration
	// See: terraform/variables.tf kubernetes_kubelet_extra_args, kubernetes_kubelet_extra_config
	KubeletExtraArgs   map[string]string `mapstructure:"kubelet_extra_args" yaml:"kubelet_extra_args"`
	KubeletExtraConfig map[string]any    `mapstructure:"kubelet_extra_config" yaml:"kubelet_extra_config"`
}

// OIDCConfig defines the OIDC authentication configuration.
type OIDCConfig struct {
	Enabled   bool   `mapstructure:"enabled" yaml:"enabled"`
	IssuerURL string `mapstructure:"issuer_url" yaml:"issuer_url"`
	ClientID  string `mapstructure:"client_id" yaml:"client_id"`

	// UsernameClaim specifies the JWT claim to use as the username.
	// See: terraform/variables.tf oidc_username_claim
	// Default: "sub"
	UsernameClaim string `mapstructure:"username_claim" yaml:"username_claim"`

	// GroupsClaim specifies the JWT claim to use as the user's groups.
	// See: terraform/variables.tf oidc_groups_claim
	// Default: "groups"
	GroupsClaim string `mapstructure:"groups_claim" yaml:"groups_claim"`
}

// CNIConfig defines the CNI-related configuration.
type CNIConfig struct {
	Encryption string `mapstructure:"encryption" yaml:"encryption"` // ipsec, wireguard
}

// AdmissionControlPlugin defines an admission control plugin configuration.
// See: terraform/variables.tf kube_api_admission_control
type AdmissionControlPlugin struct {
	// Name is the admission plugin name.
	Name string `mapstructure:"name" yaml:"name"`

	// Configuration is the plugin-specific configuration.
	Configuration map[string]any `mapstructure:"configuration" yaml:"configuration"`
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
