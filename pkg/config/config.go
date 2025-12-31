package config

type ClusterConfig struct {
	ClusterName      string `yaml:"cluster_name"`
	HcloudToken      string `yaml:"hcloud_token"`
	TalosVersion     string `yaml:"talos_version"`
	TalosSchematicID string `yaml:"talos_schematic_id"`

	ControlPlane NodePool `yaml:"control_plane"`
	Workers      []NodePool `yaml:"workers"`

    // ... other config fields mapping to variables.tf
}

type NodePool struct {
    Name       string `yaml:"name"`
    ServerType string `yaml:"server_type"`
    Location   string `yaml:"location"`
    Count      int    `yaml:"count"`
}
