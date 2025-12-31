package config

// ClusterConfig represents the configuration for the Kubernetes cluster.
type ClusterConfig struct {
	// ClusterName is the name of the cluster.
	ClusterName      string `yaml:"cluster_name"`
	// HcloudToken is the API token for Hetzner Cloud.
	HcloudToken      string `yaml:"hcloud_token"`
	// TalosVersion is the version of Talos Linux to use.
	TalosVersion     string `yaml:"talos_version"`
	// TalosSchematicID is the ID of the custom Talos image schematic.
	TalosSchematicID string `yaml:"talos_schematic_id"`
    // KubernetesVersion is the version of Kubernetes to deploy.
    // Default: v1.30.0 (if empty)
    KubernetesVersion string `yaml:"kubernetes_version"`

	// NetworkIPv4CIDR is the main IPv4 CIDR block for the network.
    // Default: 10.0.0.0/16
	NetworkIPv4CIDR string `yaml:"network_ipv4_cidr"`

    // ControlPlane configures the control plane nodes.
	ControlPlane NodePool `yaml:"control_plane"`
	// Workers configures the worker node pools.
	Workers      []NodePool `yaml:"workers"`

    // Firewall configures additional firewall rules.
    Firewall FirewallConfig `yaml:"firewall"`

    // Addons configures optional cluster components.
    Addons AddonsConfig `yaml:"addons"`
}

// NodePool represents a group of nodes with shared configuration.
type NodePool struct {
    // Name is the name of the node pool.
    Name       string `yaml:"name"`
    // ServerType is the Hetzner server type (e.g. cpx11).
    ServerType string `yaml:"server_type"`
    // Location is the Hetzner datacenter location (e.g. nbg1).
    Location   string `yaml:"location"`
    // Count is the number of nodes in the pool.
    Count      int    `yaml:"count"`
    // Labels are key-value pairs applied to the nodes.
    Labels     map[string]string `yaml:"labels"`
    // Taints are taints applied to the nodes.
    Taints     []string `yaml:"taints"`
}

// FirewallConfig represents firewall configuration.
type FirewallConfig struct {
    // ApiAllowList is a list of CIDRs allowed to access the Kubernetes API.
    ApiAllowList []string `yaml:"api_allow_list"`
}

// AddonsConfig represents configuration for cluster addons.
type AddonsConfig struct {
    // CNIEnabled enables the CNI plugin (default: true).
    CNIEnabled bool `yaml:"cni_enabled"`
    // CCMEnabled enables the Cloud Controller Manager (default: true).
    CCMEnabled bool `yaml:"ccm_enabled"`
    // CSIEnabled enables the Container Storage Interface (default: true).
    CSIEnabled bool `yaml:"csi_enabled"`
}
