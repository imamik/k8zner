package config

import (
	"fmt"
	"net"
)

// Validate checks the configuration for common errors and returns a detailed error if validation fails.
func (c *Config) Validate() error {
	// Required fields
	if c.ClusterName == "" {
		return fmt.Errorf("cluster_name is required")
	}
	if c.HCloudToken == "" {
		return fmt.Errorf("hcloud_token is required")
	}
	if c.Location == "" {
		return fmt.Errorf("location is required")
	}

	// Network validation
	if err := c.validateNetwork(); err != nil {
		return fmt.Errorf("network validation failed: %w", err)
	}

	// Control plane validation
	if err := c.validateControlPlane(); err != nil {
		return fmt.Errorf("control plane validation failed: %w", err)
	}

	// Worker validation
	if err := c.validateWorkers(); err != nil {
		return fmt.Errorf("worker validation failed: %w", err)
	}

	return nil
}

// validateNetwork validates network configuration.
func (c *Config) validateNetwork() error {
	if c.Network.IPv4CIDR == "" {
		return fmt.Errorf("network.ipv4_cidr is required")
	}

	// Validate CIDR format
	if _, _, err := net.ParseCIDR(c.Network.IPv4CIDR); err != nil {
		return fmt.Errorf("invalid network.ipv4_cidr: %w", err)
	}

	// Validate optional CIDRs if provided
	if c.Network.NodeIPv4CIDR != "" {
		if _, _, err := net.ParseCIDR(c.Network.NodeIPv4CIDR); err != nil {
			return fmt.Errorf("invalid network.node_ipv4_cidr: %w", err)
		}
	}

	if c.Network.ServiceIPv4CIDR != "" {
		if _, _, err := net.ParseCIDR(c.Network.ServiceIPv4CIDR); err != nil {
			return fmt.Errorf("invalid network.service_ipv4_cidr: %w", err)
		}
	}

	if c.Network.PodIPv4CIDR != "" {
		if _, _, err := net.ParseCIDR(c.Network.PodIPv4CIDR); err != nil {
			return fmt.Errorf("invalid network.pod_ipv4_cidr: %w", err)
		}
	}

	return nil
}

// validateControlPlane validates control plane configuration.
func (c *Config) validateControlPlane() error {
	if len(c.ControlPlane.NodePools) == 0 {
		return fmt.Errorf("at least one control plane node pool is required")
	}

	for i, pool := range c.ControlPlane.NodePools {
		if pool.Name == "" {
			return fmt.Errorf("control plane node pool %d: name is required", i)
		}
		if pool.ServerType == "" {
			return fmt.Errorf("control plane node pool %s: server type is required", pool.Name)
		}
		if pool.Count < 1 {
			return fmt.Errorf("control plane node pool %s: count must be at least 1, got %d", pool.Name, pool.Count)
		}
		if pool.Count%2 == 0 {
			return fmt.Errorf("control plane node pool %s: count must be odd for HA (got %d)", pool.Name, pool.Count)
		}
	}

	return nil
}

// validateWorkers validates worker node pool configuration.
func (c *Config) validateWorkers() error {
	for i, pool := range c.Workers {
		if pool.Name == "" {
			return fmt.Errorf("worker node pool %d: name is required", i)
		}
		if pool.ServerType == "" {
			return fmt.Errorf("worker node pool %s: server type is required", pool.Name)
		}
		if pool.Count < 0 {
			return fmt.Errorf("worker node pool %s: count cannot be negative, got %d", pool.Name, pool.Count)
		}
	}

	return nil
}

// ApplyDefaults applies sensible defaults to the configuration.
func (c *Config) ApplyDefaults() error {
	// Default Talos version
	if c.Talos.Version == "" {
		c.Talos.Version = "v1.8.3"
	}

	// Default Kubernetes version
	if c.Kubernetes.Version == "" {
		c.Kubernetes.Version = "v1.31.0"
	}

	// Default network zone if not set
	if c.Network.Zone == "" {
		c.Network.Zone = "eu-central"
	}

	// Default subnet mask size for nodes
	if c.Network.NodeIPv4SubnetMask == 0 {
		c.Network.NodeIPv4SubnetMask = 25
	}

	// Apply location defaults to node pools
	for i := range c.ControlPlane.NodePools {
		if c.ControlPlane.NodePools[i].Location == "" {
			c.ControlPlane.NodePools[i].Location = c.Location
		}
	}

	for i := range c.Workers {
		if c.Workers[i].Location == "" {
			c.Workers[i].Location = c.Location
		}
	}

	return nil
}
