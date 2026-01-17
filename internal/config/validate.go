package config

import (
	"fmt"
	"net"
)

// ValidLocations contains all valid Hetzner Cloud datacenter locations.
// https://docs.hetzner.com/cloud/general/locations/
var ValidLocations = map[string]bool{
	"nbg1": true, // Nuremberg, Germany
	"fsn1": true, // Falkenstein, Germany
	"hel1": true, // Helsinki, Finland
	"ash":  true, // Ashburn, USA
	"hil":  true, // Hillsboro, USA
	"sin":  true, // Singapore
}

// ValidNetworkZones contains all valid Hetzner Cloud network zones.
// https://docs.hetzner.com/cloud/networks/overview/
var ValidNetworkZones = map[string]bool{
	"eu-central":   true, // Europe
	"us-east":      true, // US East
	"us-west":      true, // US West
	"ap-southeast": true, // Asia Pacific
}

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

	// Location/Zone validation
	if err := c.validateLocations(); err != nil {
		return fmt.Errorf("location validation failed: %w", err)
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

	// Talos machine config validation
	if err := c.validateTalosMachineConfig(); err != nil {
		return fmt.Errorf("talos machine config validation failed: %w", err)
	}

	return nil
}

// validateLocations validates that all locations and network zones are valid Hetzner Cloud values.
func (c *Config) validateLocations() error {
	// Validate cluster-wide location
	if !ValidLocations[c.Location] {
		return fmt.Errorf("invalid location %q: must be one of %v", c.Location, getMapKeys(ValidLocations))
	}

	// Validate network zone if set
	if c.Network.Zone != "" && !ValidNetworkZones[c.Network.Zone] {
		return fmt.Errorf("invalid network zone %q: must be one of %v", c.Network.Zone, getMapKeys(ValidNetworkZones))
	}

	// Validate control plane node pool locations
	for _, pool := range c.ControlPlane.NodePools {
		if pool.Location != "" && !ValidLocations[pool.Location] {
			return fmt.Errorf("control plane pool %q has invalid location %q: must be one of %v",
				pool.Name, pool.Location, getMapKeys(ValidLocations))
		}
	}

	// Validate worker node pool locations
	for _, pool := range c.Workers {
		if pool.Location != "" && !ValidLocations[pool.Location] {
			return fmt.Errorf("worker pool %q has invalid location %q: must be one of %v",
				pool.Name, pool.Location, getMapKeys(ValidLocations))
		}
	}

	return nil
}

// getMapKeys returns the keys of a map as a slice for error messages.
func getMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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

// ValidConfigApplyModes contains valid Talos config apply modes.
var ValidConfigApplyModes = map[string]bool{
	"auto":      true,
	"reboot":    true,
	"no_reboot": true,
	"staged":    true,
}

// validateTalosMachineConfig validates Talos machine configuration.
func (c *Config) validateTalosMachineConfig() error {
	m := &c.Talos.Machine

	// Validate config apply mode
	if m.ConfigApplyMode != "" && !ValidConfigApplyModes[m.ConfigApplyMode] {
		return fmt.Errorf("invalid config_apply_mode %q: must be one of %v",
			m.ConfigApplyMode, getMapKeys(ValidConfigApplyModes))
	}

	// Validate extra routes are valid CIDRs
	for _, route := range m.ExtraRoutes {
		if _, _, err := net.ParseCIDR(route); err != nil {
			return fmt.Errorf("invalid extra_route CIDR %q: %w", route, err)
		}
	}

	// Validate nameservers are valid IPs
	for _, ns := range m.Nameservers {
		if net.ParseIP(ns) == nil {
			return fmt.Errorf("invalid nameserver IP %q", ns)
		}
	}

	// Validate host entries
	for i, entry := range m.ExtraHostEntries {
		if net.ParseIP(entry.IP) == nil {
			return fmt.Errorf("extra_host_entry %d: invalid IP %q", i, entry.IP)
		}
		if len(entry.Aliases) == 0 {
			return fmt.Errorf("extra_host_entry %d (IP %s): must have at least one alias", i, entry.IP)
		}
	}

	// Validate logging destinations
	for i, dest := range m.LoggingDestinations {
		if dest.Endpoint == "" {
			return fmt.Errorf("logging_destination %d: endpoint is required", i)
		}
		// Validate format if specified
		if dest.Format != "" && dest.Format != "json_lines" {
			return fmt.Errorf("logging_destination %d: invalid format %q (must be 'json_lines' or empty)", i, dest.Format)
		}
	}

	// Validate kernel modules
	for i, mod := range m.KernelModules {
		if mod.Name == "" {
			return fmt.Errorf("kernel_module %d: name is required", i)
		}
	}

	// Validate kubelet extra mounts
	for i, mount := range m.KubeletExtraMounts {
		if mount.Source == "" {
			return fmt.Errorf("kubelet_extra_mount %d: source is required", i)
		}
	}

	// Validate inline manifests
	for i, manifest := range m.InlineManifests {
		if manifest.Name == "" {
			return fmt.Errorf("inline_manifest %d: name is required", i)
		}
		if manifest.Contents == "" {
			return fmt.Errorf("inline_manifest %d (%s): contents is required", i, manifest.Name)
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

	// Apply Talos machine config defaults (matching Terraform)
	c.applyTalosMachineDefaults()

	// Apply Kubernetes config defaults
	c.applyKubernetesDefaults()

	return nil
}

// applyTalosMachineDefaults applies defaults for Talos machine configuration.
// See: terraform/variables.tf for default values
func (c *Config) applyTalosMachineDefaults() {
	m := &c.Talos.Machine

	// Disk encryption defaults (both true in Terraform)
	if m.StateEncryption == nil {
		m.StateEncryption = boolPtr(true)
	}
	if m.EphemeralEncryption == nil {
		m.EphemeralEncryption = boolPtr(true)
	}

	// Network defaults
	if m.IPv6Enabled == nil {
		m.IPv6Enabled = boolPtr(true)
	}
	if m.PublicIPv4Enabled == nil {
		m.PublicIPv4Enabled = boolPtr(true)
	}
	if m.PublicIPv6Enabled == nil {
		m.PublicIPv6Enabled = boolPtr(true)
	}

	// Hetzner DNS servers (from terraform/variables.tf talos_nameservers)
	if len(m.Nameservers) == 0 {
		m.Nameservers = []string{
			"185.12.64.1", "185.12.64.2", // Hetzner IPv4 DNS
		}
		if *m.IPv6Enabled {
			m.Nameservers = append(m.Nameservers,
				"2a01:4ff:ff00::add:1", "2a01:4ff:ff00::add:2", // Hetzner IPv6 DNS
			)
		}
	}

	// Hetzner NTP servers (from terraform/variables.tf talos_time_servers)
	if len(m.TimeServers) == 0 {
		m.TimeServers = []string{
			"ntp1.hetzner.de",
			"ntp2.hetzner.com",
			"ntp3.hetzner.net",
		}
	}

	// CoreDNS default
	if m.CoreDNSEnabled == nil {
		m.CoreDNSEnabled = boolPtr(true)
	}

	// Discovery defaults (from terraform/variables.tf)
	if m.DiscoveryKubernetesEnabled == nil {
		m.DiscoveryKubernetesEnabled = boolPtr(false)
	}
	if m.DiscoveryServiceEnabled == nil {
		m.DiscoveryServiceEnabled = boolPtr(true)
	}

	// Config apply mode default
	if m.ConfigApplyMode == "" {
		m.ConfigApplyMode = "auto"
	}
}

// applyKubernetesDefaults applies defaults for Kubernetes configuration.
func (c *Config) applyKubernetesDefaults() {
	// Cluster domain default (from terraform/variables.tf cluster_domain)
	if c.Kubernetes.Domain == "" {
		c.Kubernetes.Domain = "cluster.local"
	}

	// Allow scheduling on control planes default:
	// If nil, auto-determine based on worker count (same as Terraform)
	// If no workers, allow scheduling on control planes
	if c.Kubernetes.AllowSchedulingOnCP == nil {
		workerCount := 0
		for _, pool := range c.Workers {
			workerCount += pool.Count
		}
		// Also consider autoscaler max capacity
		for _, pool := range c.Autoscaler.NodePools {
			workerCount += pool.Max
		}
		c.Kubernetes.AllowSchedulingOnCP = boolPtr(workerCount == 0)
	}
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}
