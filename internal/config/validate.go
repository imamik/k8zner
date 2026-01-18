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

	// Ingress load balancer pools validation
	if err := c.validateIngressLoadBalancerPools(); err != nil {
		return fmt.Errorf("ingress_load_balancer_pools validation failed: %w", err)
	}

	// Talos machine config validation
	if err := c.validateTalosMachineConfig(); err != nil {
		return fmt.Errorf("talos machine config validation failed: %w", err)
	}

	// CCM validation
	if err := c.validateCCM(); err != nil {
		return fmt.Errorf("ccm validation failed: %w", err)
	}

	// Cilium validation
	if err := c.validateCilium(); err != nil {
		return fmt.Errorf("cilium validation failed: %w", err)
	}

	// Ingress NGINX validation
	if err := c.validateIngressNginx(); err != nil {
		return fmt.Errorf("ingress_nginx validation failed: %w", err)
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

// validateIngressNginx validates Ingress NGINX addon configuration.
func (c *Config) validateIngressNginx() error {
	if !c.Addons.IngressNginx.Enabled {
		return nil // Skip validation if Ingress NGINX is disabled
	}

	nginx := &c.Addons.IngressNginx

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

// validateCilium validates Cilium addon configuration.
func (c *Config) validateCilium() error {
	if !c.Addons.Cilium.Enabled {
		return nil // Skip validation if Cilium is disabled
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

	// Note: netkit with IPsec is not recommended but allowed.
	// Users should be aware of this combination from the documentation.

	return nil
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
	if hc.Retries != 0 && (hc.Retries < 1 || hc.Retries > 10) {
		return fmt.Errorf("load_balancers.health_check.retries must be between 1 and 10, got %d", hc.Retries)
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
