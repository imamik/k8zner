package config

import (
	"fmt"
	"net"
	"regexp"
)

// clusterNameRegex validates cluster name format: 1-32 lowercase alphanumeric with hyphens.
var clusterNameRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$`)

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

// ValidClusterAccessModes contains valid cluster access modes.
var ValidClusterAccessModes = map[string]bool{
	"public":  true,
	"private": true,
}

// ValidCCMLBAlgorithms contains valid load balancer algorithms for CCM.
var ValidCCMLBAlgorithms = map[string]bool{
	"round_robin":       true,
	"least_connections": true,
}

// ValidCCMLBTypes contains valid Hetzner load balancer types.
var ValidCCMLBTypes = map[string]bool{
	"lb11": true,
	"lb21": true,
	"lb31": true,
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

	// Cluster name format validation
	if err := c.validateClusterName(); err != nil {
		return err
	}

	// Cluster access mode validation
	if c.ClusterAccess != "" && !ValidClusterAccessModes[c.ClusterAccess] {
		return fmt.Errorf("invalid cluster_access %q: must be one of %v",
			c.ClusterAccess, getMapKeys(ValidClusterAccessModes))
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

	// Node pool uniqueness validation
	if err := c.validateNodePoolUniqueness(); err != nil {
		return err
	}

	// Node count constraints validation
	if err := c.validateNodeCounts(); err != nil {
		return err
	}

	// Combined name length validation
	if err := c.validateCombinedNameLengths(); err != nil {
		return err
	}

	// Autoscaler validation
	if err := c.validateAutoscaler(); err != nil {
		return fmt.Errorf("autoscaler validation failed: %w", err)
	}

	// Firewall rules validation
	if err := c.validateFirewallRules(); err != nil {
		return fmt.Errorf("firewall validation failed: %w", err)
	}

	// Ingress load balancer pools validation
	if err := c.validateIngressLoadBalancerPools(); err != nil {
		return fmt.Errorf("ingress_load_balancer_pools validation failed: %w", err)
	}

	// Talos machine config validation
	if err := c.validateTalosMachineConfig(); err != nil {
		return fmt.Errorf("talos machine config validation failed: %w", err)
	}

	// Kubelet mounts validation
	if err := c.validateKubeletMounts(); err != nil {
		return fmt.Errorf("kubelet_extra_mounts validation failed: %w", err)
	}

	// CCM validation
	if err := c.validateCCM(); err != nil {
		return fmt.Errorf("ccm validation failed: %w", err)
	}

	// CSI validation
	if err := c.validateCSI(); err != nil {
		return fmt.Errorf("csi validation failed: %w", err)
	}

	// Cilium validation
	if err := c.validateCilium(); err != nil {
		return fmt.Errorf("cilium validation failed: %w", err)
	}

	// OIDC validation
	if err := c.validateOIDC(); err != nil {
		return fmt.Errorf("oidc validation failed: %w", err)
	}

	// Ingress NGINX validation
	if err := c.validateIngressNginx(); err != nil {
		return fmt.Errorf("ingress_nginx validation failed: %w", err)
	}

	// Talos backup validation
	if err := c.validateTalosBackup(); err != nil {
		return fmt.Errorf("talos_backup validation failed: %w", err)
	}

	// Cloudflare validation
	if err := c.validateCloudflare(); err != nil {
		return fmt.Errorf("cloudflare validation failed: %w", err)
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

	// Apply CCM defaults
	c.applyCCMDefaults()

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

// applyCCMDefaults applies default values to CCM configuration.
// These defaults match Terraform's hcloud_ccm_* variable defaults.
func (c *Config) applyCCMDefaults() {
	ccm := &c.Addons.CCM
	lb := &ccm.LoadBalancers

	// Network routes enabled by default
	if ccm.NetworkRoutesEnabled == nil {
		enabled := true
		ccm.NetworkRoutesEnabled = &enabled
	}

	// Load balancer controller enabled by default
	if lb.Enabled == nil {
		enabled := true
		lb.Enabled = &enabled
	}

	// Default LB type: lb11
	if lb.Type == "" {
		lb.Type = "lb11"
	}

	// Default algorithm: least_connections
	if lb.Algorithm == "" {
		lb.Algorithm = "least_connections"
	}

	// Use private IP by default
	if lb.UsePrivateIP == nil {
		enabled := true
		lb.UsePrivateIP = &enabled
	}

	// Disable private ingress by default
	if lb.DisablePrivateIngress == nil {
		enabled := true
		lb.DisablePrivateIngress = &enabled
	}

	// Don't disable public network by default
	if lb.DisablePublicNetwork == nil {
		disabled := false
		lb.DisablePublicNetwork = &disabled
	}

	// Don't disable IPv6 by default
	if lb.DisableIPv6 == nil {
		disabled := false
		lb.DisableIPv6 = &disabled
	}

	// Proxy protocol disabled by default
	if lb.UsesProxyProtocol == nil {
		enabled := false
		lb.UsesProxyProtocol = &enabled
	}

	// Health check defaults
	hc := &lb.HealthCheck
	if hc.Interval == 0 {
		hc.Interval = 3
	}
	if hc.Timeout == 0 {
		hc.Timeout = 3
	}
	if hc.Retries == 0 {
		hc.Retries = 3
	}
}

// ValidIngressNginxKinds contains valid controller kinds for Ingress NGINX.
var ValidIngressNginxKinds = map[string]bool{
	"Deployment": true,
	"DaemonSet":  true,
}

// ValidIngressNginxExternalTrafficPolicies contains valid external traffic policies.
var ValidIngressNginxExternalTrafficPolicies = map[string]bool{
	"Cluster": true,
	"Local":   true,
}

// ValidCiliumBPFDatapathModes contains valid BPF datapath modes for Cilium.
var ValidCiliumBPFDatapathModes = map[string]bool{
	"veth":      true,
	"netkit":    true,
	"netkit-l2": true,
}

// ValidCiliumPolicyCIDRMatchModes contains valid policy CIDR match modes for Cilium.
var ValidCiliumPolicyCIDRMatchModes = map[string]bool{
	"":      true, // disabled
	"nodes": true, // allow targeting nodes by CIDR
}

// ValidCiliumGatewayAPIExternalTrafficPolicies contains valid external traffic policies.
var ValidCiliumGatewayAPIExternalTrafficPolicies = map[string]bool{
	"Cluster": true,
	"Local":   true,
}

// validateCCM validates CCM configuration.
func (c *Config) validateCCM() error {
	if !c.Addons.CCM.Enabled {
		return nil // Skip validation if CCM is disabled
	}

	lb := &c.Addons.CCM.LoadBalancers

	// Validate LB type if set
	if lb.Type != "" && !ValidCCMLBTypes[lb.Type] {
		return fmt.Errorf("invalid load_balancers.type %q: must be one of %v",
			lb.Type, getMapKeys(ValidCCMLBTypes))
	}

	// Validate algorithm if set
	if lb.Algorithm != "" && !ValidCCMLBAlgorithms[lb.Algorithm] {
		return fmt.Errorf("invalid load_balancers.algorithm %q: must be one of %v",
			lb.Algorithm, getMapKeys(ValidCCMLBAlgorithms))
	}

	// Validate location if set
	if lb.Location != "" && !ValidLocations[lb.Location] {
		return fmt.Errorf("invalid load_balancers.location %q: must be one of %v",
			lb.Location, getMapKeys(ValidLocations))
	}

	// Validate health check settings
	hc := &lb.HealthCheck
	if hc.Interval != 0 && (hc.Interval < 3 || hc.Interval > 60) {
		return fmt.Errorf("load_balancers.health_check.interval must be between 3 and 60 seconds, got %d", hc.Interval)
	}
	if hc.Timeout != 0 && (hc.Timeout < 1 || hc.Timeout > 60) {
		return fmt.Errorf("load_balancers.health_check.timeout must be between 1 and 60 seconds, got %d", hc.Timeout)
	}
	if hc.Retries < 0 || hc.Retries > 5 {
		return fmt.Errorf("load_balancers.health_check.retries must be between 0 and 5, got %d", hc.Retries)
	}

	return nil
}

// validateIngressLoadBalancerPools validates ingress load balancer pools configuration.
func (c *Config) validateIngressLoadBalancerPools() error {
	seen := make(map[string]bool)
	for i, pool := range c.IngressLoadBalancerPools {
		// Name is required and must be unique
		if pool.Name == "" {
			return fmt.Errorf("pool %d: name is required", i)
		}
		if seen[pool.Name] {
			return fmt.Errorf("pool %d: duplicate name %q", i, pool.Name)
		}
		seen[pool.Name] = true

		// Location is required
		if pool.Location == "" {
			return fmt.Errorf("pool %q: location is required", pool.Name)
		}
		if !ValidLocations[pool.Location] {
			return fmt.Errorf("pool %q: invalid location %q: must be one of %v",
				pool.Name, pool.Location, getMapKeys(ValidLocations))
		}

		// Validate load balancer type if specified
		if pool.Type != "" && !ValidCCMLBTypes[pool.Type] {
			return fmt.Errorf("pool %q: invalid type %q: must be one of %v",
				pool.Name, pool.Type, getMapKeys(ValidCCMLBTypes))
		}

		// Validate algorithm if specified
		if pool.Algorithm != "" && !ValidCCMLBAlgorithms[pool.Algorithm] {
			return fmt.Errorf("pool %q: invalid algorithm %q: must be one of %v",
				pool.Name, pool.Algorithm, getMapKeys(ValidCCMLBAlgorithms))
		}

		// Count must be positive if specified
		if pool.Count < 0 {
			return fmt.Errorf("pool %q: count cannot be negative, got %d", pool.Name, pool.Count)
		}
	}

	return nil
}

// validateTalosBackup validates Talos backup configuration.
func (c *Config) validateTalosBackup() error {
	backup := &c.Addons.TalosBackup

	// Validate S3 Hcloud URL format if provided
	if backup.S3HcloudURL != "" {
		if !hcloudS3URLRegex.MatchString(backup.S3HcloudURL) {
			return fmt.Errorf("invalid s3_hcloud_url %q: must match format 'bucket.region.your-objectstorage.com' or 'https://bucket.region.your-objectstorage.com'",
				backup.S3HcloudURL)
		}
	}

	return nil
}

// validateClusterName validates cluster name format matches Terraform requirements.
func (c *Config) validateClusterName() error {
	if !clusterNameRegex.MatchString(c.ClusterName) {
		return fmt.Errorf("invalid cluster_name %q: must be 1-32 lowercase alphanumeric characters or hyphens, starting and ending with alphanumeric", c.ClusterName)
	}
	return nil
}

// validateNodePoolUniqueness ensures all node pool names are unique within their category.
func (c *Config) validateNodePoolUniqueness() error {
	// Check control plane pool names
	cpNames := make(map[string]bool)
	for _, pool := range c.ControlPlane.NodePools {
		if cpNames[pool.Name] {
			return fmt.Errorf("duplicate control plane pool name: %q", pool.Name)
		}
		cpNames[pool.Name] = true
	}

	// Check worker pool names
	workerNames := make(map[string]bool)
	for _, pool := range c.Workers {
		if workerNames[pool.Name] {
			return fmt.Errorf("duplicate worker pool name: %q", pool.Name)
		}
		workerNames[pool.Name] = true
	}

	// Check autoscaler pool names
	asNames := make(map[string]bool)
	for _, pool := range c.Autoscaler.NodePools {
		if asNames[pool.Name] {
			return fmt.Errorf("duplicate autoscaler pool name: %q", pool.Name)
		}
		asNames[pool.Name] = true
	}

	return nil
}

// validateNodeCounts validates total node counts don't exceed limits.
func (c *Config) validateNodeCounts() error {
	// Sum control plane nodes
	cpTotal := 0
	for _, pool := range c.ControlPlane.NodePools {
		cpTotal += pool.Count
	}
	if cpTotal > 9 {
		return fmt.Errorf("total control plane nodes must be <= 9, got %d", cpTotal)
	}

	// Sum all nodes (CP + workers + autoscaler max)
	totalNodes := cpTotal
	for _, pool := range c.Workers {
		totalNodes += pool.Count
	}
	for _, pool := range c.Autoscaler.NodePools {
		totalNodes += pool.Max
	}
	if totalNodes > 100 {
		return fmt.Errorf("total nodes (control plane + workers + autoscaler max) must be <= 100, got %d", totalNodes)
	}

	return nil
}

// validateCombinedNameLengths ensures cluster_name + pool_name <= 56 chars.
func (c *Config) validateCombinedNameLengths() error {
	const maxLen = 56

	// Check control plane pools
	for _, pool := range c.ControlPlane.NodePools {
		if len(c.ClusterName)+len(pool.Name)+1 > maxLen {
			return fmt.Errorf("control plane pool %q: combined length of cluster name and pool name exceeds %d characters", pool.Name, maxLen)
		}
	}

	// Check worker pools
	for _, pool := range c.Workers {
		if len(c.ClusterName)+len(pool.Name)+1 > maxLen {
			return fmt.Errorf("worker pool %q: combined length of cluster name and pool name exceeds %d characters", pool.Name, maxLen)
		}
	}

	// Check autoscaler pools
	for _, pool := range c.Autoscaler.NodePools {
		if len(c.ClusterName)+len(pool.Name)+1 > maxLen {
			return fmt.Errorf("autoscaler pool %q: combined length of cluster name and pool name exceeds %d characters", pool.Name, maxLen)
		}
	}

	// Check ingress LB pools
	for _, pool := range c.IngressLoadBalancerPools {
		if len(c.ClusterName)+len(pool.Name)+1 > maxLen {
			return fmt.Errorf("ingress load balancer pool %q: combined length of cluster name and pool name exceeds %d characters", pool.Name, maxLen)
		}
	}

	return nil
}

// validateAutoscaler validates autoscaler configuration.
func (c *Config) validateAutoscaler() error {
	for _, pool := range c.Autoscaler.NodePools {
		if pool.Name == "" {
			return fmt.Errorf("autoscaler pool name is required")
		}
		if pool.Type == "" {
			return fmt.Errorf("autoscaler pool %q: server type is required", pool.Name)
		}
		if pool.Location == "" {
			return fmt.Errorf("autoscaler pool %q: location is required", pool.Name)
		}
		if !ValidLocations[pool.Location] {
			return fmt.Errorf("autoscaler pool %q: invalid location %q: must be one of %v",
				pool.Name, pool.Location, getMapKeys(ValidLocations))
		}
		if pool.Max < pool.Min {
			return fmt.Errorf("autoscaler pool %q: max (%d) must be >= min (%d)", pool.Name, pool.Max, pool.Min)
		}
		if pool.Min < 0 {
			return fmt.Errorf("autoscaler pool %q: min cannot be negative", pool.Name)
		}
	}
	return nil
}

// ValidFirewallDirections contains valid firewall rule directions.
var ValidFirewallDirections = map[string]bool{
	"in":  true,
	"out": true,
}

// ValidFirewallProtocols contains valid firewall rule protocols.
var ValidFirewallProtocols = map[string]bool{
	"tcp":  true,
	"udp":  true,
	"icmp": true,
	"gre":  true,
	"esp":  true,
}

// validateFirewallRules validates firewall extra rules configuration.
func (c *Config) validateFirewallRules() error {
	for i, rule := range c.Firewall.ExtraRules {
		// Validate direction
		if !ValidFirewallDirections[rule.Direction] {
			return fmt.Errorf("firewall rule %d: direction must be 'in' or 'out', got %q", i, rule.Direction)
		}

		// Validate protocol
		if !ValidFirewallProtocols[rule.Protocol] {
			return fmt.Errorf("firewall rule %d: protocol must be one of tcp, udp, icmp, gre, esp, got %q", i, rule.Protocol)
		}

		// Validate direction-specific IP requirements
		switch rule.Direction {
		case "in":
			if len(rule.SourceIPs) == 0 {
				return fmt.Errorf("firewall rule %d: 'in' direction requires source_ips", i)
			}
			if len(rule.DestinationIPs) > 0 {
				return fmt.Errorf("firewall rule %d: 'in' direction cannot have destination_ips", i)
			}
		case "out":
			if len(rule.DestinationIPs) == 0 {
				return fmt.Errorf("firewall rule %d: 'out' direction requires destination_ips", i)
			}
			if len(rule.SourceIPs) > 0 {
				return fmt.Errorf("firewall rule %d: 'out' direction cannot have source_ips", i)
			}
		}

		// Validate port requirements based on protocol
		switch rule.Protocol {
		case "tcp", "udp":
			if rule.Port == "" {
				return fmt.Errorf("firewall rule %d: %s protocol requires port", i, rule.Protocol)
			}
		case "icmp", "gre", "esp":
			if rule.Port != "" {
				return fmt.Errorf("firewall rule %d: %s protocol cannot have port", i, rule.Protocol)
			}
		}
	}
	return nil
}

// validateKubeletMounts validates kubelet extra mounts configuration.
func (c *Config) validateKubeletMounts() error {
	mounts := c.Talos.Machine.KubeletExtraMounts
	if len(mounts) == 0 {
		return nil
	}

	// Check for unique destinations
	destinations := make(map[string]bool)
	for i, mount := range mounts {
		dest := mount.Destination
		if dest == "" {
			dest = mount.Source // Default destination is source
		}

		if destinations[dest] {
			return fmt.Errorf("kubelet_extra_mount %d: duplicate destination %q", i, dest)
		}
		destinations[dest] = true

		// Check for Longhorn path conflict
		if c.Addons.Longhorn.Enabled && dest == "/var/lib/longhorn" {
			return fmt.Errorf("kubelet_extra_mount %d: /var/lib/longhorn conflicts with Longhorn addon", i)
		}
	}

	return nil
}

// validateCSI validates CSI configuration.
func (c *Config) validateCSI() error {
	csi := &c.Addons.CSI
	if !csi.Enabled {
		return nil
	}

	// Validate encryption passphrase if provided
	if csi.EncryptionPassphrase != "" {
		if len(csi.EncryptionPassphrase) < 8 || len(csi.EncryptionPassphrase) > 512 {
			return fmt.Errorf("csi encryption_passphrase must be 8-512 characters, got %d", len(csi.EncryptionPassphrase))
		}
		// Validate printable ASCII (32-126)
		for i, r := range csi.EncryptionPassphrase {
			if r < 32 || r > 126 {
				return fmt.Errorf("csi encryption_passphrase contains non-printable ASCII at position %d", i)
			}
		}
	}

	return nil
}

// ValidCiliumEncryptionTypes contains valid Cilium encryption types.
var ValidCiliumEncryptionTypes = map[string]bool{
	"":          true,
	"wireguard": true,
	"ipsec":     true,
}

// ValidIPSecKeySizes contains valid IPSec key sizes.
var ValidIPSecKeySizes = map[int]bool{
	0:   true, // Not set
	128: true,
	192: true,
	256: true,
}

// validateCilium validates Cilium addon configuration with dependency checks.
func (c *Config) validateCilium() error {
	if !c.Addons.Cilium.Enabled {
		return nil
	}

	cilium := &c.Addons.Cilium

	// Validate BPF datapath mode
	if cilium.BPFDatapathMode != "" && !ValidCiliumBPFDatapathModes[cilium.BPFDatapathMode] {
		return fmt.Errorf("invalid bpf_datapath_mode %q: must be one of %v",
			cilium.BPFDatapathMode, getMapKeys(ValidCiliumBPFDatapathModes))
	}

	// Validate policy CIDR match mode
	if !ValidCiliumPolicyCIDRMatchModes[cilium.PolicyCIDRMatchMode] {
		return fmt.Errorf("invalid policy_cidr_match_mode %q: must be one of %v",
			cilium.PolicyCIDRMatchMode, getMapKeys(ValidCiliumPolicyCIDRMatchModes))
	}

	// Validate Gateway API external traffic policy
	if cilium.GatewayAPIExternalTrafficPolicy != "" && !ValidCiliumGatewayAPIExternalTrafficPolicies[cilium.GatewayAPIExternalTrafficPolicy] {
		return fmt.Errorf("invalid gateway_api_external_traffic_policy %q: must be one of %v",
			cilium.GatewayAPIExternalTrafficPolicy, getMapKeys(ValidCiliumGatewayAPIExternalTrafficPolicies))
	}

	// Validate encryption type
	if !ValidCiliumEncryptionTypes[cilium.EncryptionType] {
		return fmt.Errorf("invalid encryption_type %q: must be one of wireguard, ipsec, or empty",
			cilium.EncryptionType)
	}

	// Validate egress gateway dependency (requires kube-proxy replacement)
	if cilium.EgressGatewayEnabled && !cilium.KubeProxyReplacementEnabled {
		return fmt.Errorf("egress_gateway_enabled requires kube_proxy_replacement_enabled=true")
	}

	// Validate Hubble dependency chain
	if cilium.HubbleRelayEnabled && !cilium.HubbleEnabled {
		return fmt.Errorf("hubble_relay_enabled requires hubble_enabled=true")
	}
	if cilium.HubbleUIEnabled && !cilium.HubbleRelayEnabled {
		return fmt.Errorf("hubble_ui_enabled requires hubble_relay_enabled=true")
	}

	// Validate IPSec settings if using IPSec encryption
	if cilium.EncryptionType == "ipsec" {
		if cilium.IPSecKeyID != 0 && (cilium.IPSecKeyID < 1 || cilium.IPSecKeyID > 15) {
			return fmt.Errorf("ipsec_key_id must be 1-15, got %d", cilium.IPSecKeyID)
		}
		if !ValidIPSecKeySizes[cilium.IPSecKeySize] {
			return fmt.Errorf("ipsec_key_size must be 128, 192, or 256, got %d", cilium.IPSecKeySize)
		}
	}

	return nil
}

// validateOIDC validates OIDC configuration.
func (c *Config) validateOIDC() error {
	oidc := &c.Kubernetes.OIDC
	if !oidc.Enabled {
		return nil
	}

	// Required fields when OIDC is enabled
	if oidc.IssuerURL == "" {
		return fmt.Errorf("oidc.issuer_url is required when OIDC is enabled")
	}
	if oidc.ClientID == "" {
		return fmt.Errorf("oidc.client_id is required when OIDC is enabled")
	}

	// Validate group mapping uniqueness
	groupNames := make(map[string]bool)
	for _, mapping := range c.Addons.OIDCRBAC.GroupMappings {
		if groupNames[mapping.Group] {
			return fmt.Errorf("duplicate OIDC group mapping: %q", mapping.Group)
		}
		groupNames[mapping.Group] = true
	}

	return nil
}

// validateIngressNginx validates Ingress NGINX addon configuration with dependencies.
func (c *Config) validateIngressNginx() error {
	if !c.Addons.IngressNginx.Enabled {
		return nil
	}

	nginx := &c.Addons.IngressNginx

	// Ingress NGINX requires cert-manager
	if !c.Addons.CertManager.Enabled {
		return fmt.Errorf("ingress_nginx requires cert_manager to be enabled")
	}

	// Validate kind
	if nginx.Kind != "" && !ValidIngressNginxKinds[nginx.Kind] {
		return fmt.Errorf("invalid kind %q: must be one of %v",
			nginx.Kind, getMapKeys(ValidIngressNginxKinds))
	}

	// Validate external traffic policy
	if nginx.ExternalTrafficPolicy != "" && !ValidIngressNginxExternalTrafficPolicies[nginx.ExternalTrafficPolicy] {
		return fmt.Errorf("invalid external_traffic_policy %q: must be one of %v",
			nginx.ExternalTrafficPolicy, getMapKeys(ValidIngressNginxExternalTrafficPolicies))
	}

	// Validate replicas must be nil when kind is DaemonSet
	if nginx.Kind == "DaemonSet" && nginx.Replicas != nil {
		return fmt.Errorf("replicas must not be set when kind is 'DaemonSet'")
	}

	// Validate replicas is positive if set
	if nginx.Replicas != nil && *nginx.Replicas < 1 {
		return fmt.Errorf("replicas must be at least 1, got %d", *nginx.Replicas)
	}

	return nil
}

// ValidExternalDNSPolicies contains valid external-dns sync policies.
var ValidExternalDNSPolicies = map[string]bool{
	"":            true, // Empty means default (sync)
	"sync":        true, // Deletes records when resources are removed
	"upsert-only": true, // Never deletes records
}

// ValidExternalDNSSources contains valid external-dns source types.
var ValidExternalDNSSources = map[string]bool{
	"ingress":              true,
	"service":              true,
	"gateway-httproute":    true,
	"gateway-grpcroute":    true,
	"gateway-tcproute":     true,
	"gateway-udproute":     true,
	"gateway-tlsroute":     true,
	"istio-gateway":        true,
	"istio-virtualservice": true,
	"contour-httpproxy":    true,
	"crd":                  true,
}

// validateCloudflare validates Cloudflare DNS integration configuration.
func (c *Config) validateCloudflare() error {
	cf := &c.Addons.Cloudflare
	extDNS := &c.Addons.ExternalDNS
	certCF := &c.Addons.CertManager.Cloudflare

	// If Cloudflare is not enabled, check that dependent features are also disabled
	if !cf.Enabled {
		if extDNS.Enabled {
			return fmt.Errorf("external_dns requires cloudflare to be enabled")
		}
		if certCF.Enabled {
			return fmt.Errorf("cert_manager.cloudflare requires cloudflare to be enabled")
		}
		return nil // Nothing else to validate
	}

	// Cloudflare is enabled - validate required fields
	if cf.APIToken == "" {
		return fmt.Errorf("cloudflare.api_token is required when cloudflare is enabled (or set CF_API_TOKEN env var)")
	}

	// Validate external-dns configuration if enabled
	if extDNS.Enabled {
		if err := c.validateExternalDNS(); err != nil {
			return fmt.Errorf("external_dns validation failed: %w", err)
		}
	}

	// Validate cert-manager Cloudflare configuration if enabled
	if certCF.Enabled {
		if err := c.validateCertManagerCloudflare(); err != nil {
			return fmt.Errorf("cert_manager.cloudflare validation failed: %w", err)
		}
	}

	return nil
}

// validateExternalDNS validates external-dns addon configuration.
func (c *Config) validateExternalDNS() error {
	extDNS := &c.Addons.ExternalDNS

	// Validate policy if set
	if !ValidExternalDNSPolicies[extDNS.Policy] {
		return fmt.Errorf("invalid policy %q: must be one of %v",
			extDNS.Policy, getMapKeys(ValidExternalDNSPolicies))
	}

	// Validate sources if set
	for _, source := range extDNS.Sources {
		if !ValidExternalDNSSources[source] {
			return fmt.Errorf("invalid source %q: must be one of %v",
				source, getMapKeys(ValidExternalDNSSources))
		}
	}

	return nil
}

// validateCertManagerCloudflare validates cert-manager Cloudflare DNS01 configuration.
func (c *Config) validateCertManagerCloudflare() error {
	certCF := &c.Addons.CertManager.Cloudflare

	// cert-manager must be enabled for Cloudflare DNS01 to work
	if !c.Addons.CertManager.Enabled {
		return fmt.Errorf("cert_manager must be enabled to use cloudflare DNS01 solver")
	}

	// Email is required for Let's Encrypt account
	if certCF.Email == "" {
		return fmt.Errorf("email is required for Let's Encrypt account registration")
	}

	// Basic email format validation (contains @)
	if len(certCF.Email) < 3 || !contains(certCF.Email, "@") {
		return fmt.Errorf("invalid email format %q", certCF.Email)
	}

	return nil
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
