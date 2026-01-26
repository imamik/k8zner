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

	// PrerequisitesCheckEnabled enables preflight check for required client tools.
	// See: terraform/variables.tf client_prerequisites_check_enabled
	// Default: true
	PrerequisitesCheckEnabled *bool `mapstructure:"prerequisites_check_enabled" yaml:"prerequisites_check_enabled"`

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
	NodePools             []ControlPlaneNodePool `mapstructure:"nodepools" yaml:"nodepools"`
	PublicVIPIPv4Enabled  bool                   `mapstructure:"public_vip_ipv4_enabled" yaml:"public_vip_ipv4_enabled"`
	PublicVIPIPv4ID       int                    `mapstructure:"public_vip_ipv4_id" yaml:"public_vip_ipv4_id"`
	PrivateVIPIPv4Enabled bool                   `mapstructure:"private_vip_ipv4_enabled" yaml:"private_vip_ipv4_enabled"`
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

// HelmChartConfig defines custom Helm chart configuration for addons.
// This allows overriding the default repository, chart, version, and values.
type HelmChartConfig struct {
	// Repository specifies a custom Helm repository URL.
	Repository string `mapstructure:"repository" yaml:"repository"`

	// Chart specifies a custom chart name.
	Chart string `mapstructure:"chart" yaml:"chart"`

	// Version specifies a custom chart version.
	Version string `mapstructure:"version" yaml:"version"`

	// Values specifies custom Helm values to merge with defaults.
	Values map[string]any `mapstructure:"values" yaml:"values"`
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

// AddonsConfig defines the addon-related configuration.
type AddonsConfig struct {
	Cilium                 CiliumConfig                 `mapstructure:"cilium" yaml:"cilium"`
	CCM                    CCMConfig                    `mapstructure:"ccm" yaml:"ccm"`
	CSI                    CSIConfig                    `mapstructure:"csi" yaml:"csi"`
	MetricsServer          MetricsServerConfig          `mapstructure:"metrics_server" yaml:"metrics_server"`
	CertManager            CertManagerConfig            `mapstructure:"cert_manager" yaml:"cert_manager"`
	IngressNginx           IngressNginxConfig           `mapstructure:"ingress_nginx" yaml:"ingress_nginx"`
	Traefik                TraefikConfig                `mapstructure:"traefik" yaml:"traefik"`
	Longhorn               LonghornConfig               `mapstructure:"longhorn" yaml:"longhorn"`
	ClusterAutoscaler      ClusterAutoscalerConfig      `mapstructure:"cluster_autoscaler" yaml:"cluster_autoscaler"`
	RBAC                   RBACConfig                   `mapstructure:"rbac" yaml:"rbac"`
	OIDCRBAC               OIDCRBACConfig               `mapstructure:"oidc_rbac" yaml:"oidc_rbac"`
	TalosBackup            TalosBackupConfig            `mapstructure:"talos_backup" yaml:"talos_backup"`
	GatewayAPICRDs         GatewayAPICRDsConfig         `mapstructure:"gateway_api_crds" yaml:"gateway_api_crds"`
	PrometheusOperatorCRDs PrometheusOperatorCRDsConfig `mapstructure:"prometheus_operator_crds" yaml:"prometheus_operator_crds"`
	TalosCCM               TalosCCMConfig               `mapstructure:"talos_ccm" yaml:"talos_ccm"`
}

// CCMConfig defines the Hetzner Cloud Controller Manager configuration.
// See: terraform/variables.tf hcloud_ccm_* variables
type CCMConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Helm allows customizing the Helm chart repository, version, and values.
	Helm HelmChartConfig `mapstructure:"helm" yaml:"helm"`

	// LoadBalancers configures the CCM Load Balancer controller.
	// These settings control the default behavior for CCM-managed load balancers.
	LoadBalancers CCMLoadBalancerConfig `mapstructure:"load_balancers" yaml:"load_balancers"`

	// NetworkRoutesEnabled enables or disables the CCM Route Controller.
	// When enabled, CCM manages routes for pod networking.
	// Default: true (matching Terraform)
	NetworkRoutesEnabled *bool `mapstructure:"network_routes_enabled" yaml:"network_routes_enabled"`
}

// CCMLoadBalancerConfig defines the CCM Load Balancer controller configuration.
// See: terraform/variables.tf hcloud_ccm_load_balancers_* variables
type CCMLoadBalancerConfig struct {
	// Enabled enables or disables the CCM Service Controller (load balancer management).
	// Default: true
	Enabled *bool `mapstructure:"enabled" yaml:"enabled"`

	// Location sets the default Hetzner location for CCM-managed Load Balancers.
	// If not set, uses the cluster's default location.
	Location string `mapstructure:"location" yaml:"location"`

	// Type sets the default Load Balancer type (e.g., lb11, lb21, lb31).
	// Default: "lb11"
	Type string `mapstructure:"type" yaml:"type"`

	// Algorithm sets the default load balancing algorithm.
	// Valid values: "round_robin", "least_connections"
	// Default: "least_connections"
	Algorithm string `mapstructure:"algorithm" yaml:"algorithm"`

	// UsePrivateIP configures Load Balancer server targets to use the private IP by default.
	// Default: true
	UsePrivateIP *bool `mapstructure:"use_private_ip" yaml:"use_private_ip"`

	// DisablePrivateIngress disables the use of the private network for ingress by default.
	// Default: true
	DisablePrivateIngress *bool `mapstructure:"disable_private_ingress" yaml:"disable_private_ingress"`

	// DisablePublicNetwork disables the public interface of CCM-managed Load Balancers by default.
	// Default: false
	DisablePublicNetwork *bool `mapstructure:"disable_public_network" yaml:"disable_public_network"`

	// DisableIPv6 disables the use of IPv6 for Load Balancers by default.
	// Default: false
	DisableIPv6 *bool `mapstructure:"disable_ipv6" yaml:"disable_ipv6"`

	// UsesProxyProtocol enables the PROXY protocol for CCM-managed Load Balancers by default.
	// Default: false
	UsesProxyProtocol *bool `mapstructure:"uses_proxy_protocol" yaml:"uses_proxy_protocol"`

	// HealthCheck configures default health check settings for load balancers.
	HealthCheck CCMHealthCheckConfig `mapstructure:"health_check" yaml:"health_check"`
}

// CCMHealthCheckConfig defines health check settings for CCM-managed load balancers.
type CCMHealthCheckConfig struct {
	// Interval is the time interval in seconds between health checks.
	// Valid range: 3-60 seconds
	// Default: 3
	Interval int `mapstructure:"interval" yaml:"interval"`

	// Timeout is the time in seconds after which a health check is considered failed.
	// Valid range: 1-60 seconds
	// Default: 3
	Timeout int `mapstructure:"timeout" yaml:"timeout"`

	// Retries is the number of unsuccessful retries before a target is considered unhealthy.
	// Valid range: 1-10
	// Default: 3
	Retries int `mapstructure:"retries" yaml:"retries"`
}

// CSIConfig defines the Hetzner Cloud CSI driver configuration.
type CSIConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Helm allows customizing the Helm chart repository, version, and values.
	Helm HelmChartConfig `mapstructure:"helm" yaml:"helm"`

	DefaultStorageClass  bool           `mapstructure:"default_storage_class" yaml:"default_storage_class"`
	EncryptionPassphrase string         `mapstructure:"encryption_passphrase" yaml:"encryption_passphrase"`
	StorageClasses       []StorageClass `mapstructure:"storage_classes" yaml:"storage_classes"`

	// VolumeExtraLabels specifies additional labels to apply to Hetzner volumes.
	// See: terraform/variables.tf hcloud_csi_volume_extra_labels
	VolumeExtraLabels map[string]string `mapstructure:"volume_extra_labels" yaml:"volume_extra_labels"`
}

// MetricsServerConfig defines the Kubernetes Metrics Server configuration.
type MetricsServerConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Helm allows customizing the Helm chart repository, version, and values.
	Helm HelmChartConfig `mapstructure:"helm" yaml:"helm"`

	// ScheduleOnControlPlane determines whether to schedule the Metrics Server on control plane nodes.
	// If nil, defaults to true when there are no worker nodes.
	// See: terraform/variables.tf metrics_server_schedule_on_control_plane
	ScheduleOnControlPlane *bool `mapstructure:"schedule_on_control_plane" yaml:"schedule_on_control_plane"`

	// Replicas specifies the number of replicas for the Metrics Server.
	// If nil, auto-calculated: 2 for clusters with >1 schedulable nodes, 1 otherwise.
	// See: terraform/variables.tf metrics_server_replicas
	Replicas *int `mapstructure:"replicas" yaml:"replicas"`
}

// CertManagerConfig defines the cert-manager configuration.
type CertManagerConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Helm allows customizing the Helm chart repository, version, and values.
	Helm HelmChartConfig `mapstructure:"helm" yaml:"helm"`
}

// IngressNginxConfig defines the ingress-nginx configuration.
// See: terraform/variables.tf ingress_nginx_* variables
type IngressNginxConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Helm allows customizing the Helm chart repository, version, and values.
	Helm HelmChartConfig `mapstructure:"helm" yaml:"helm"`

	// Kind specifies the Kubernetes controller type: "Deployment" or "DaemonSet".
	// Default: "Deployment"
	Kind string `mapstructure:"kind" yaml:"kind"`

	// Replicas specifies the number of controller replicas.
	// If nil, auto-calculated: 2 for <3 workers, 3 for >=3 workers.
	// Must be nil when Kind is "DaemonSet".
	Replicas *int `mapstructure:"replicas" yaml:"replicas"`

	// TopologyAwareRouting enables topology-aware traffic routing.
	// Sets service.kubernetes.io/topology-mode annotation.
	// Default: false
	TopologyAwareRouting bool `mapstructure:"topology_aware_routing" yaml:"topology_aware_routing"`

	// ExternalTrafficPolicy controls how external traffic is routed.
	// Valid values: "Cluster" (cluster-wide) or "Local" (node-local).
	// Default: "Cluster"
	ExternalTrafficPolicy string `mapstructure:"external_traffic_policy" yaml:"external_traffic_policy"`

	// Config provides global nginx configuration via ConfigMap.
	// Reference: https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/configmap/
	Config map[string]string `mapstructure:"config" yaml:"config"`
}

// TraefikConfig defines the Traefik Proxy ingress controller configuration.
// Traefik is an alternative to ingress-nginx with built-in support for
// modern protocols and automatic service discovery.
type TraefikConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Helm allows customizing the Helm chart repository, version, and values.
	Helm HelmChartConfig `mapstructure:"helm" yaml:"helm"`

	// Kind specifies the Kubernetes controller type: "Deployment" or "DaemonSet".
	// Default: "Deployment"
	Kind string `mapstructure:"kind" yaml:"kind"`

	// Replicas specifies the number of controller replicas.
	// If nil, auto-calculated: 2 for <3 workers, 3 for >=3 workers.
	// Must be nil when Kind is "DaemonSet".
	Replicas *int `mapstructure:"replicas" yaml:"replicas"`

	// ExternalTrafficPolicy controls how external traffic is routed.
	// Valid values: "Cluster" (cluster-wide) or "Local" (node-local).
	// Default: "Local"
	ExternalTrafficPolicy string `mapstructure:"external_traffic_policy" yaml:"external_traffic_policy"`

	// IngressClass specifies the IngressClass name for Traefik.
	// Default: "traefik"
	IngressClass string `mapstructure:"ingress_class" yaml:"ingress_class"`

	// Dashboard configures the Traefik dashboard.
	Dashboard TraefikDashboardConfig `mapstructure:"dashboard" yaml:"dashboard"`
}

// TraefikDashboardConfig defines the Traefik dashboard configuration.
type TraefikDashboardConfig struct {
	// Enabled enables the Traefik dashboard.
	// Default: false
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// IngressRoute creates an IngressRoute to expose the dashboard.
	// Only applicable when Enabled is true.
	// Default: false
	IngressRoute bool `mapstructure:"ingress_route" yaml:"ingress_route"`
}

// LonghornConfig defines the Longhorn storage configuration.
type LonghornConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Helm allows customizing the Helm chart repository, version, and values.
	Helm HelmChartConfig `mapstructure:"helm" yaml:"helm"`

	DefaultStorageClass bool `mapstructure:"default_storage_class" yaml:"default_storage_class"`
}

// StorageClass defines a Kubernetes StorageClass for CSI.
type StorageClass struct {
	Name                string            `mapstructure:"name" yaml:"name"`
	Encrypted           bool              `mapstructure:"encrypted" yaml:"encrypted"`
	ReclaimPolicy       string            `mapstructure:"reclaim_policy" yaml:"reclaim_policy"`
	DefaultStorageClass bool              `mapstructure:"default_storage_class" yaml:"default_storage_class"`
	ExtraParameters     map[string]string `mapstructure:"extra_parameters" yaml:"extra_parameters"`
}

// RBACConfig defines RBAC roles and cluster roles.
type RBACConfig struct {
	Enabled      bool                `mapstructure:"enabled" yaml:"enabled"`
	Roles        []RoleConfig        `mapstructure:"roles" yaml:"roles"`
	ClusterRoles []ClusterRoleConfig `mapstructure:"cluster_roles" yaml:"cluster_roles"`
}

// RoleConfig defines a namespaced Role.
type RoleConfig struct {
	Name      string           `mapstructure:"name" yaml:"name"`
	Namespace string           `mapstructure:"namespace" yaml:"namespace"`
	Rules     []RBACRuleConfig `mapstructure:"rules" yaml:"rules"`
}

// ClusterRoleConfig defines a ClusterRole.
type ClusterRoleConfig struct {
	Name  string           `mapstructure:"name" yaml:"name"`
	Rules []RBACRuleConfig `mapstructure:"rules" yaml:"rules"`
}

// RBACRuleConfig defines a policy rule for RBAC.
type RBACRuleConfig struct {
	APIGroups []string `mapstructure:"api_groups" yaml:"api_groups"`
	Resources []string `mapstructure:"resources" yaml:"resources"`
	Verbs     []string `mapstructure:"verbs" yaml:"verbs"`
}

// OIDCRBACConfig defines OIDC group mappings to Kubernetes roles.
type OIDCRBACConfig struct {
	Enabled       bool                   `mapstructure:"enabled" yaml:"enabled"`
	GroupsPrefix  string                 `mapstructure:"groups_prefix" yaml:"groups_prefix"`
	GroupMappings []OIDCRBACGroupMapping `mapstructure:"group_mappings" yaml:"group_mappings"`
}

// OIDCRBACGroupMapping maps an OIDC group to Kubernetes roles and cluster roles.
type OIDCRBACGroupMapping struct {
	Group        string         `mapstructure:"group" yaml:"group"`
	ClusterRoles []string       `mapstructure:"cluster_roles" yaml:"cluster_roles"`
	Roles        []OIDCRBACRole `mapstructure:"roles" yaml:"roles"`
}

// OIDCRBACRole defines a namespaced role for OIDC mapping.
type OIDCRBACRole struct {
	Name      string `mapstructure:"name" yaml:"name"`
	Namespace string `mapstructure:"namespace" yaml:"namespace"`
}

// CiliumConfig defines the Cilium CNI configuration.
type CiliumConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Helm allows customizing the Helm chart repository, version, and values.
	Helm HelmChartConfig `mapstructure:"helm" yaml:"helm"`

	// Encryption
	EncryptionEnabled bool   `mapstructure:"encryption_enabled" yaml:"encryption_enabled"`
	EncryptionType    string `mapstructure:"encryption_type" yaml:"encryption_type"` // wireguard, ipsec

	// IPSec specific
	IPSecAlgorithm string `mapstructure:"ipsec_algorithm" yaml:"ipsec_algorithm"`
	IPSecKeySize   int    `mapstructure:"ipsec_key_size" yaml:"ipsec_key_size"`
	IPSecKeyID     int    `mapstructure:"ipsec_key_id" yaml:"ipsec_key_id"`

	// Routing
	RoutingMode string `mapstructure:"routing_mode" yaml:"routing_mode"` // native, tunnel

	// BPF Configuration
	// BPFDatapathMode sets the mode for Pod devices. Valid values: veth, netkit, netkit-l2.
	// Warning: Netkit is still in beta and should not be used with IPsec encryption.
	// Default: "veth"
	BPFDatapathMode string `mapstructure:"bpf_datapath_mode" yaml:"bpf_datapath_mode"`

	// PolicyCIDRMatchMode allows cluster nodes to be selected by CIDR network policies.
	// Set to "nodes" to enable targeting the kube-api server with k8s NetworkPolicy.
	// Default: "" (disabled)
	PolicyCIDRMatchMode string `mapstructure:"policy_cidr_match_mode" yaml:"policy_cidr_match_mode"`

	// SocketLBHostNamespaceOnly limits Cilium's socket-level load-balancing to the host namespace only.
	// Default: false
	SocketLBHostNamespaceOnly bool `mapstructure:"socket_lb_host_namespace_only" yaml:"socket_lb_host_namespace_only"`

	// KubeProxy Replacement
	KubeProxyReplacementEnabled bool `mapstructure:"kube_proxy_replacement_enabled" yaml:"kube_proxy_replacement_enabled"`

	// Gateway API
	GatewayAPIEnabled bool `mapstructure:"gateway_api_enabled" yaml:"gateway_api_enabled"`

	// GatewayAPIProxyProtocolEnabled enables PROXY Protocol on Cilium Gateway API for external LB traffic.
	// Default: true
	GatewayAPIProxyProtocolEnabled *bool `mapstructure:"gateway_api_proxy_protocol_enabled" yaml:"gateway_api_proxy_protocol_enabled"`

	// GatewayAPIExternalTrafficPolicy controls traffic routing for Gateway API services.
	// Valid values: "Cluster" (cluster-wide) or "Local" (node-local endpoints).
	// Default: "Cluster"
	GatewayAPIExternalTrafficPolicy string `mapstructure:"gateway_api_external_traffic_policy" yaml:"gateway_api_external_traffic_policy"`

	// Egress Gateway
	EgressGatewayEnabled bool `mapstructure:"egress_gateway_enabled" yaml:"egress_gateway_enabled"`

	// Hubble Observability
	HubbleEnabled      bool `mapstructure:"hubble_enabled" yaml:"hubble_enabled"`
	HubbleRelayEnabled bool `mapstructure:"hubble_relay_enabled" yaml:"hubble_relay_enabled"`
	HubbleUIEnabled    bool `mapstructure:"hubble_ui_enabled" yaml:"hubble_ui_enabled"`

	// Prometheus Integration
	// ServiceMonitorEnabled enables Prometheus ServiceMonitor resources.
	// Requires Prometheus Operator CRDs to be installed.
	// Default: false
	ServiceMonitorEnabled bool `mapstructure:"service_monitor_enabled" yaml:"service_monitor_enabled"`
}

// ClusterAutoscalerConfig defines the Cluster Autoscaler addon configuration.
type ClusterAutoscalerConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Helm allows customizing the Helm chart repository, version, and values.
	Helm HelmChartConfig `mapstructure:"helm" yaml:"helm"`

	// DiscoveryEnabled enables cluster-api based node discovery.
	// See: terraform/variables.tf cluster_autoscaler_discovery_enabled
	// Default: false
	DiscoveryEnabled bool `mapstructure:"discovery_enabled" yaml:"discovery_enabled"`

	// ImageTag specifies the cluster-autoscaler image tag.
	// See: terraform/variables.tf cluster_autoscaler_image_tag
	// Default: uses latest compatible version
	ImageTag string `mapstructure:"image_tag" yaml:"image_tag"`
}

// TalosBackupConfig defines the Talos etcd backup configuration.
type TalosBackupConfig struct {
	Enabled            bool   `mapstructure:"enabled" yaml:"enabled"`
	Version            string `mapstructure:"version" yaml:"version"`
	Schedule           string `mapstructure:"schedule" yaml:"schedule"`
	S3Bucket           string `mapstructure:"s3_bucket" yaml:"s3_bucket"`
	S3Region           string `mapstructure:"s3_region" yaml:"s3_region"`
	S3Endpoint         string `mapstructure:"s3_endpoint" yaml:"s3_endpoint"`
	S3Prefix           string `mapstructure:"s3_prefix" yaml:"s3_prefix"`
	S3AccessKey        string `mapstructure:"s3_access_key" yaml:"s3_access_key"`
	S3SecretKey        string `mapstructure:"s3_secret_key" yaml:"s3_secret_key"`
	S3PathStyle        bool   `mapstructure:"s3_path_style" yaml:"s3_path_style"`
	AGEX25519PublicKey string `mapstructure:"age_x25519_public_key" yaml:"age_x25519_public_key"`
	EnableCompression  bool   `mapstructure:"enable_compression" yaml:"enable_compression"`

	// S3HcloudURL is a convenience field for Hetzner Object Storage.
	// Format: bucket.region.your-objectstorage.com or https://bucket.region.your-objectstorage.com
	// When set, automatically extracts S3Bucket, S3Region, and S3Endpoint.
	// See: terraform/variables.tf talos_backup_s3_hcloud_url
	S3HcloudURL string `mapstructure:"s3_hcloud_url" yaml:"s3_hcloud_url"`
}

// RDNSConfig defines cluster-wide reverse DNS defaults.
type RDNSConfig struct {
	// Cluster-wide defaults (fallback for all resources)
	ClusterRDNS     string `mapstructure:"cluster" yaml:"cluster"`
	ClusterRDNSIPv4 string `mapstructure:"cluster_ipv4" yaml:"cluster_ipv4"`
	ClusterRDNSIPv6 string `mapstructure:"cluster_ipv6" yaml:"cluster_ipv6"`
	// Ingress load balancer RDNS (generic, without IP version suffix)
	IngressRDNS     string `mapstructure:"ingress" yaml:"ingress"`
	IngressRDNSIPv4 string `mapstructure:"ingress_ipv4" yaml:"ingress_ipv4"`
	IngressRDNSIPv6 string `mapstructure:"ingress_ipv6" yaml:"ingress_ipv6"`
}

// GatewayAPICRDsConfig defines the Gateway API CRDs configuration.
// See: terraform/variables.tf gateway_api_crds_* variables
type GatewayAPICRDsConfig struct {
	// Enabled enables the Gateway API CRDs deployment.
	// Default: true (matching Terraform)
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Version specifies the Gateway API CRDs version.
	// Default: "v1.4.1"
	Version string `mapstructure:"version" yaml:"version"`

	// ReleaseChannel specifies the release channel (standard or experimental).
	// Default: "standard"
	ReleaseChannel string `mapstructure:"release_channel" yaml:"release_channel"`
}

// PrometheusOperatorCRDsConfig defines the Prometheus Operator CRDs configuration.
// See: terraform/variables.tf prometheus_operator_crds_* variables
type PrometheusOperatorCRDsConfig struct {
	// Enabled enables the Prometheus Operator CRDs deployment.
	// Default: true (matching Terraform)
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Version specifies the Prometheus Operator CRDs version.
	// Default: "v0.87.1"
	Version string `mapstructure:"version" yaml:"version"`
}

// TalosCCMConfig defines the Talos Cloud Controller Manager configuration.
// See: terraform/variables.tf talos_ccm_* variables
// This is separate from the Hetzner CCM - it's the Siderolabs Talos CCM.
type TalosCCMConfig struct {
	// Enabled enables the Talos CCM deployment.
	// Default: true (matching Terraform)
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Version specifies the Talos CCM version.
	// Default: "v1.11.0"
	Version string `mapstructure:"version" yaml:"version"`
}
