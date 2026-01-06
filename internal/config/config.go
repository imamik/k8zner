// Package config defines the configuration structure and methods for the application.
package config

import (
	"fmt"
	"os"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

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

// CNIConfig defines the CNI-related configuration.
type CNIConfig struct {
	Encryption string `mapstructure:"encryption" yaml:"encryption"` // ipsec, wireguard
}

// AddonsConfig defines addon-specific configurations.
type AddonsConfig struct {
	CCM    CCMConfig    `mapstructure:"ccm" yaml:"ccm"`
	CSI    CSIConfig    `mapstructure:"csi" yaml:"csi"`
	Cilium CiliumConfig `mapstructure:"cilium" yaml:"cilium"`
}

// CCMConfig defines the Hetzner Cloud Controller Manager configuration.
type CCMConfig struct {
	Enabled                      bool           `mapstructure:"enabled" yaml:"enabled"`
	LoadBalancersEnabled         bool           `mapstructure:"load_balancers_enabled" yaml:"load_balancers_enabled"`
	NetworkRoutesEnabled         bool           `mapstructure:"network_routes_enabled" yaml:"network_routes_enabled"`
	LoadBalancerAlgorithmType    string         `mapstructure:"load_balancer_algorithm_type" yaml:"load_balancer_algorithm_type"`
	LoadBalancerType             string         `mapstructure:"load_balancer_type" yaml:"load_balancer_type"`
	LoadBalancerLocation         string         `mapstructure:"load_balancer_location" yaml:"load_balancer_location"`
	LoadBalancerDisablePrivate   bool           `mapstructure:"load_balancer_disable_private" yaml:"load_balancer_disable_private"`
	LoadBalancerDisablePublic    bool           `mapstructure:"load_balancer_disable_public" yaml:"load_balancer_disable_public"`
	LoadBalancerUsePrivateIP     bool           `mapstructure:"load_balancer_use_private_ip" yaml:"load_balancer_use_private_ip"`
	LoadBalancerHealthCheckInt   int            `mapstructure:"load_balancer_health_check_interval" yaml:"load_balancer_health_check_interval"`
	LoadBalancerHealthCheckRetry int            `mapstructure:"load_balancer_health_check_retries" yaml:"load_balancer_health_check_retries"`
	LoadBalancerHealthCheckTimeout int          `mapstructure:"load_balancer_health_check_timeout" yaml:"load_balancer_health_check_timeout"`
	HelmRepository               string         `mapstructure:"helm_repository" yaml:"helm_repository"`
	HelmChart                    string         `mapstructure:"helm_chart" yaml:"helm_chart"`
	HelmVersion                  string         `mapstructure:"helm_version" yaml:"helm_version"`
	HelmValues                   map[string]any `mapstructure:"helm_values" yaml:"helm_values,omitempty"`
}

// CSIConfig defines the Hetzner CSI Driver configuration.
type CSIConfig struct {
	Enabled              bool              `mapstructure:"enabled" yaml:"enabled"`
	EncryptionPassphrase string            `mapstructure:"encryption_passphrase" yaml:"encryption_passphrase,omitempty"`
	StorageClasses       []StorageClass    `mapstructure:"storage_classes" yaml:"storage_classes,omitempty"`
	HelmRepository       string            `mapstructure:"helm_repository" yaml:"helm_repository"`
	HelmChart            string            `mapstructure:"helm_chart" yaml:"helm_chart"`
	HelmVersion          string            `mapstructure:"helm_version" yaml:"helm_version"`
	HelmValues           map[string]any    `mapstructure:"helm_values" yaml:"helm_values,omitempty"`
}

// StorageClass defines a Kubernetes storage class configuration for CSI.
type StorageClass struct {
	Name                string            `mapstructure:"name" yaml:"name"`
	ReclaimPolicy       string            `mapstructure:"reclaim_policy" yaml:"reclaim_policy"`
	DefaultStorageClass bool              `mapstructure:"default_storage_class" yaml:"default_storage_class"`
	Encrypted           bool              `mapstructure:"encrypted" yaml:"encrypted"`
	ExtraLabels         map[string]string `mapstructure:"extra_labels" yaml:"extra_labels,omitempty"`
}

// CiliumConfig defines the Cilium CNI configuration.
type CiliumConfig struct {
	Enabled              bool           `mapstructure:"enabled" yaml:"enabled"`
	RoutingMode          string         `mapstructure:"routing_mode" yaml:"routing_mode"`
	KubeProxyReplacement bool           `mapstructure:"kube_proxy_replacement" yaml:"kube_proxy_replacement"`
	EncryptionEnabled    bool           `mapstructure:"encryption_enabled" yaml:"encryption_enabled"`
	EncryptionType       string         `mapstructure:"encryption_type" yaml:"encryption_type"`
	IPSecKeyID           int            `mapstructure:"ipsec_key_id" yaml:"ipsec_key_id,omitempty"`
	IPSecAlgorithm       string         `mapstructure:"ipsec_algorithm" yaml:"ipsec_algorithm,omitempty"`
	IPSecKeySize         int            `mapstructure:"ipsec_key_size" yaml:"ipsec_key_size,omitempty"`
	HubbleEnabled        bool           `mapstructure:"hubble_enabled" yaml:"hubble_enabled"`
	GatewayAPIEnabled    bool           `mapstructure:"gateway_api_enabled" yaml:"gateway_api_enabled"`
	BPFDatapathMode      string         `mapstructure:"bpf_datapath_mode" yaml:"bpf_datapath_mode"`
	PolicyCIDRMatchMode  []string       `mapstructure:"policy_cidr_match_mode" yaml:"policy_cidr_match_mode"`
	HelmRepository       string         `mapstructure:"helm_repository" yaml:"helm_repository"`
	HelmChart            string         `mapstructure:"helm_chart" yaml:"helm_chart"`
	HelmVersion          string         `mapstructure:"helm_version" yaml:"helm_version"`
	HelmValues           map[string]any `mapstructure:"helm_values" yaml:"helm_values,omitempty"`
}

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
		cfg.Network.Zone = "eu-central" // Default used in terraform usually
	}

	// Validate
	if cfg.ClusterName == "" {
		return nil, fmt.Errorf("cluster_name is required")
	}

	return &cfg, nil
}
