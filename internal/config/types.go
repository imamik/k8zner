// Package config defines the configuration structure and methods for the application.
package config

// Config holds the application configuration.
type Config struct {
	ClusterName string   `mapstructure:"cluster_name" yaml:"cluster_name"`
	HCloudToken string   `mapstructure:"hcloud_token" yaml:"hcloud_token"`
	Location    string   `mapstructure:"location" yaml:"location"` // e.g. nbg1, fsn1, hel1
	SSHKeys     []string `mapstructure:"ssh_keys" yaml:"ssh_keys"` // List of SSH key names/IDs

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
	UseCurrentIPv4 bool           `mapstructure:"use_current_ipv4" yaml:"use_current_ipv4"`
	UseCurrentIPv6 bool           `mapstructure:"use_current_ipv6" yaml:"use_current_ipv6"`
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
}

// AutoscalerConfig defines the autoscaler configuration.
type AutoscalerConfig struct {
	Enabled   bool                 `mapstructure:"enabled" yaml:"enabled"`
	NodePools []AutoscalerNodePool `mapstructure:"nodepools" yaml:"nodepools"`
}

// AutoscalerNodePool defines a node pool for the autoscaler.
type AutoscalerNodePool struct {
	Name     string            `mapstructure:"name" yaml:"name"`
	Location string            `mapstructure:"location" yaml:"location"`
	Type     string            `mapstructure:"type" yaml:"type"`
	Min      int               `mapstructure:"min" yaml:"min"`
	Max      int               `mapstructure:"max" yaml:"max"`
	Labels   map[string]string `mapstructure:"labels" yaml:"labels"`
	Taints   []string          `mapstructure:"taints" yaml:"taints"`
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
}

// TalosConfig defines the Talos-specific configuration.
type TalosConfig struct {
	Version    string        `mapstructure:"version" yaml:"version"`
	Extensions []string      `mapstructure:"extensions" yaml:"extensions"`
	Upgrade    UpgradeConfig `mapstructure:"upgrade" yaml:"upgrade"`
}

// UpgradeConfig defines the upgrade-related configuration.
type UpgradeConfig struct {
	Debug      bool   `mapstructure:"debug" yaml:"debug"`
	Force      bool   `mapstructure:"force" yaml:"force"`
	Insecure   bool   `mapstructure:"insecure" yaml:"insecure"`
	RebootMode string `mapstructure:"reboot_mode" yaml:"reboot_mode"`
}

// KubernetesConfig defines the Kubernetes-specific configuration.
type KubernetesConfig struct {
	Version string     `mapstructure:"version" yaml:"version"`
	OIDC    OIDCConfig `mapstructure:"oidc" yaml:"oidc"`
	CNI     CNIConfig  `mapstructure:"cni" yaml:"cni"`
}

// OIDCConfig defines the OIDC authentication configuration.
type OIDCConfig struct {
	Enabled   bool   `mapstructure:"enabled" yaml:"enabled"`
	IssuerURL string `mapstructure:"issuer_url" yaml:"issuer_url"`
	ClientID  string `mapstructure:"client_id" yaml:"client_id"`
}

// CNIConfig defines the CNI-related configuration (Cilium).
type CNIConfig struct {
	// Enabled controls whether Cilium CNI is installed.
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// HelmVersion is the Cilium chart version.
	HelmVersion string `mapstructure:"helm_version" yaml:"helm_version"`

	// KubeProxyReplacement enables Cilium's eBPF kube-proxy replacement.
	KubeProxyReplacement bool `mapstructure:"kube_proxy_replacement" yaml:"kube_proxy_replacement"`

	// RoutingMode: "native" or "tunnel".
	RoutingMode string `mapstructure:"routing_mode" yaml:"routing_mode"`

	// BPFDatapathMode: "veth", "netkit", or "netkit-l2".
	BPFDatapathMode string `mapstructure:"bpf_datapath_mode" yaml:"bpf_datapath_mode"`

	// Encryption settings for transparent network encryption.
	Encryption CiliumEncryptionConfig `mapstructure:"encryption" yaml:"encryption"`

	// Hubble observability settings.
	Hubble CiliumHubbleConfig `mapstructure:"hubble" yaml:"hubble"`

	// GatewayAPI settings.
	GatewayAPI CiliumGatewayAPIConfig `mapstructure:"gateway_api" yaml:"gateway_api"`

	// ExtraHelmValues for advanced customization (merged with defaults).
	ExtraHelmValues map[string]interface{} `mapstructure:"extra_helm_values" yaml:"extra_helm_values"`
}

// CiliumEncryptionConfig defines encryption settings for Cilium.
type CiliumEncryptionConfig struct {
	// Enabled turns on transparent network encryption.
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Type: "wireguard" (default) or "ipsec".
	Type string `mapstructure:"type" yaml:"type"`

	// IPSec settings (only used when Type="ipsec").
	IPSec CiliumIPSecConfig `mapstructure:"ipsec" yaml:"ipsec"`
}

// CiliumIPSecConfig defines IPSec-specific settings.
type CiliumIPSecConfig struct {
	// Algorithm: default "rfc4106(gcm(aes))".
	Algorithm string `mapstructure:"algorithm" yaml:"algorithm"`

	// KeySize in bits: 128, 192, or 256.
	KeySize int `mapstructure:"key_size" yaml:"key_size"`

	// KeyID for key rotation (1-15).
	KeyID int `mapstructure:"key_id" yaml:"key_id"`

	// Key is the pre-generated IPSec key (optional, generated if empty).
	Key string `mapstructure:"key" yaml:"key"`
}

// CiliumHubbleConfig defines Hubble observability settings.
type CiliumHubbleConfig struct {
	Enabled      bool `mapstructure:"enabled" yaml:"enabled"`
	RelayEnabled bool `mapstructure:"relay_enabled" yaml:"relay_enabled"`
	UIEnabled    bool `mapstructure:"ui_enabled" yaml:"ui_enabled"`
}

// CiliumGatewayAPIConfig defines Gateway API settings.
type CiliumGatewayAPIConfig struct {
	Enabled               bool   `mapstructure:"enabled" yaml:"enabled"`
	ExternalTrafficPolicy string `mapstructure:"external_traffic_policy" yaml:"external_traffic_policy"`
}

// AddonsConfig defines the addon-related configuration.
type AddonsConfig struct {
	CCM CCMConfig `mapstructure:"ccm" yaml:"ccm"`
	CSI CSIConfig `mapstructure:"csi" yaml:"csi"`
}

// CCMConfig defines the Hetzner Cloud Controller Manager configuration.
type CCMConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
}

// CSIConfig defines the Hetzner Cloud CSI driver configuration.
type CSIConfig struct {
	Enabled              bool           `mapstructure:"enabled" yaml:"enabled"`
	DefaultStorageClass  bool           `mapstructure:"default_storage_class" yaml:"default_storage_class"`
	EncryptionPassphrase string         `mapstructure:"encryption_passphrase" yaml:"encryption_passphrase"`
	StorageClasses       []StorageClass `mapstructure:"storage_classes" yaml:"storage_classes"`
}

// StorageClass defines a Kubernetes StorageClass for CSI.
type StorageClass struct {
	Name          string `mapstructure:"name" yaml:"name"`
	ReclaimPolicy string `mapstructure:"reclaim_policy" yaml:"reclaim_policy"`
	IsDefault     bool   `mapstructure:"is_default" yaml:"is_default"`
}
