# Talos Machine Configuration Enhancement Plan

## Overview

This plan details the implementation of enhanced Talos machine configuration options to achieve feature parity with the Terraform implementation. The goal is to support all production-critical configuration options while maintaining the existing code patterns.

## Analysis of Terraform Implementation

### Architecture Pattern

Terraform builds configuration patches as YAML maps with two main sections:

```yaml
machine:
  install: { image, extraKernelArgs }
  nodeLabels: {}
  nodeAnnotations: {}
  nodeTaints: {}
  certSANs: []
  network: { hostname, interfaces, nameservers, extraHostEntries }
  kubelet: { extraArgs, extraConfig, extraMounts, nodeIP }
  kernel: { modules }
  sysctls: {}
  registries: {}
  systemDiskEncryption: {}
  features: { kubernetesTalosAPIAccess, hostDNS }
  time: { servers }
  logging: { destinations }
cluster:
  allowSchedulingOnControlPlanes: bool
  network: { dnsDomain, podSubnets, serviceSubnets, cni }
  coreDNS: { disabled }
  proxy: { disabled }
  apiServer: { admissionControl, certSANs, extraArgs }
  controllerManager: { extraArgs }
  discovery: { enabled, registries }
  etcd: { advertisedSubnets, extraArgs }
  scheduler: { extraArgs }
  adminKubeconfig: { certLifetime }
  inlineManifests: []
  externalCloudProvider: { enabled, manifests }
```

### Key Terraform Configuration Sources

| Feature | Terraform Variable | Default |
|---------|-------------------|---------|
| Disk Encryption (State) | `talos_state_partition_encryption_enabled` | `true` |
| Disk Encryption (Ephemeral) | `talos_ephemeral_partition_encryption_enabled` | `true` |
| Nameservers | `talos_nameservers` | Hetzner DNS |
| Time Servers | `talos_time_servers` | Hetzner NTP |
| Extra Host Entries | `talos_extra_host_entries` | `[]` |
| Extra Routes | `talos_extra_routes` | `[]` |
| Kernel Args | `talos_extra_kernel_args` | `[]` |
| Kernel Modules | `talos_kernel_modules` | `null` |
| Sysctls | `talos_sysctls_extra_args` | `{}` |
| Registries | `talos_registries` | `null` |
| Kubelet Extra Mounts | `talos_kubelet_extra_mounts` | `[]` |
| CoreDNS | `talos_coredns_enabled` | `true` |
| Discovery K8s | `talos_discovery_kubernetes_enabled` | `false` |
| Discovery Service | `talos_discovery_service_enabled` | `true` |
| Logging Destinations | `talos_logging_destinations` | `[]` |
| IPv6 | `talos_ipv6_enabled` | `true` |
| Public IPv4 | `talos_public_ipv4_enabled` | `true` |
| Public IPv6 | `talos_public_ipv6_enabled` | `true` |
| Inline Manifests | `talos_extra_inline_manifests` | `null` |
| Remote Manifests | `talos_extra_remote_manifests` | `null` |
| Config Apply Mode | `talos_machine_configuration_apply_mode` | `"auto"` |
| Cluster Domain | `cluster_domain` | `"cluster.local"` |
| Allow Scheduling on CP | `cluster_allow_scheduling_on_control_planes` | auto |

---

## Implementation Plan

### Phase 1: Configuration Types

**File: `internal/config/types.go`**

Add new types to `TalosConfig`:

```go
type TalosConfig struct {
    Version     string        `yaml:"version"`
    SchematicID string        `yaml:"schematic_id"`
    Extensions  []string      `yaml:"extensions"`
    Upgrade     UpgradeConfig `yaml:"upgrade"`

    // NEW: Machine-level configuration
    Machine TalosMachineConfig `yaml:"machine"`
}

type TalosMachineConfig struct {
    // Disk Encryption (default: true for both)
    StateEncryption     *bool `yaml:"state_encryption"`
    EphemeralEncryption *bool `yaml:"ephemeral_encryption"`

    // Network Configuration
    IPv6Enabled       *bool    `yaml:"ipv6_enabled"`
    PublicIPv4Enabled *bool    `yaml:"public_ipv4_enabled"`
    PublicIPv6Enabled *bool    `yaml:"public_ipv6_enabled"`
    Nameservers       []string `yaml:"nameservers"`
    TimeServers       []string `yaml:"time_servers"`
    ExtraRoutes       []string `yaml:"extra_routes"`
    ExtraHostEntries  []TalosHostEntry `yaml:"extra_host_entries"`

    // DNS
    CoreDNSEnabled *bool `yaml:"coredns_enabled"`

    // Registry Configuration
    Registries *TalosRegistryConfig `yaml:"registries"`

    // Kernel Configuration
    KernelArgs    []string                `yaml:"kernel_args"`
    KernelModules []TalosKernelModule     `yaml:"kernel_modules"`
    Sysctls       map[string]string       `yaml:"sysctls"`

    // Kubelet Configuration
    KubeletExtraMounts []TalosKubeletMount `yaml:"kubelet_extra_mounts"`
    KubeletExtraArgs   map[string]string   `yaml:"kubelet_extra_args"`
    KubeletExtraConfig map[string]any      `yaml:"kubelet_extra_config"`

    // Bootstrap Manifests
    InlineManifests []TalosInlineManifest `yaml:"inline_manifests"`
    RemoteManifests []string              `yaml:"remote_manifests"`

    // Discovery Services
    DiscoveryKubernetesEnabled *bool `yaml:"discovery_kubernetes_enabled"`
    DiscoveryServiceEnabled    *bool `yaml:"discovery_service_enabled"`

    // Logging
    LoggingDestinations []TalosLoggingDestination `yaml:"logging_destinations"`

    // Apply Mode (auto, reboot, no_reboot, staged)
    ConfigApplyMode string `yaml:"config_apply_mode"`
}

type TalosHostEntry struct {
    IP      string   `yaml:"ip"`
    Aliases []string `yaml:"aliases"`
}

type TalosKernelModule struct {
    Name       string   `yaml:"name"`
    Parameters []string `yaml:"parameters"`
}

type TalosKubeletMount struct {
    Source      string   `yaml:"source"`
    Destination string   `yaml:"destination"`
    Type        string   `yaml:"type"`
    Options     []string `yaml:"options"`
}

type TalosInlineManifest struct {
    Name     string `yaml:"name"`
    Contents string `yaml:"contents"`
}

type TalosLoggingDestination struct {
    Endpoint  string            `yaml:"endpoint"`
    Format    string            `yaml:"format"`
    ExtraTags map[string]string `yaml:"extra_tags"`
}

type TalosRegistryConfig struct {
    Mirrors map[string]TalosRegistryMirror `yaml:"mirrors"`
}

type TalosRegistryMirror struct {
    Endpoints []string `yaml:"endpoints"`
}
```

**Add to `KubernetesConfig`:**

```go
type KubernetesConfig struct {
    Version string     `yaml:"version"`
    OIDC    OIDCConfig `yaml:"oidc"`
    CNI     CNIConfig  `yaml:"cni"`

    // NEW: Cluster-level configuration
    Domain                  string            `yaml:"domain"`
    AllowSchedulingOnCP     *bool             `yaml:"allow_scheduling_on_control_planes"`
    APIServerExtraArgs      map[string]string `yaml:"api_server_extra_args"`
    KubeletExtraArgs        map[string]string `yaml:"kubelet_extra_args"`
    AdmissionControl        []any             `yaml:"admission_control"`
}
```

### Phase 2: Defaults

**File: `internal/config/defaults.go`**

```go
func applyTalosMachineDefaults(cfg *Config) {
    // Disk encryption defaults (matching Terraform)
    if cfg.Talos.Machine.StateEncryption == nil {
        cfg.Talos.Machine.StateEncryption = ptr(true)
    }
    if cfg.Talos.Machine.EphemeralEncryption == nil {
        cfg.Talos.Machine.EphemeralEncryption = ptr(true)
    }

    // Network defaults
    if cfg.Talos.Machine.IPv6Enabled == nil {
        cfg.Talos.Machine.IPv6Enabled = ptr(true)
    }
    if cfg.Talos.Machine.PublicIPv4Enabled == nil {
        cfg.Talos.Machine.PublicIPv4Enabled = ptr(true)
    }
    if cfg.Talos.Machine.PublicIPv6Enabled == nil {
        cfg.Talos.Machine.PublicIPv6Enabled = ptr(true)
    }

    // Hetzner DNS servers (matching Terraform)
    if len(cfg.Talos.Machine.Nameservers) == 0 {
        cfg.Talos.Machine.Nameservers = []string{
            "185.12.64.1", "185.12.64.2",  // IPv4
        }
        if *cfg.Talos.Machine.IPv6Enabled {
            cfg.Talos.Machine.Nameservers = append(
                cfg.Talos.Machine.Nameservers,
                "2a01:4ff:ff00::add:1", "2a01:4ff:ff00::add:2", // IPv6
            )
        }
    }

    // Hetzner NTP servers (matching Terraform)
    if len(cfg.Talos.Machine.TimeServers) == 0 {
        cfg.Talos.Machine.TimeServers = []string{
            "ntp1.hetzner.de",
            "ntp2.hetzner.com",
            "ntp3.hetzner.net",
        }
    }

    // CoreDNS default
    if cfg.Talos.Machine.CoreDNSEnabled == nil {
        cfg.Talos.Machine.CoreDNSEnabled = ptr(true)
    }

    // Discovery defaults
    if cfg.Talos.Machine.DiscoveryKubernetesEnabled == nil {
        cfg.Talos.Machine.DiscoveryKubernetesEnabled = ptr(false)
    }
    if cfg.Talos.Machine.DiscoveryServiceEnabled == nil {
        cfg.Talos.Machine.DiscoveryServiceEnabled = ptr(true)
    }

    // Config apply mode default
    if cfg.Talos.Machine.ConfigApplyMode == "" {
        cfg.Talos.Machine.ConfigApplyMode = "auto"
    }

    // Cluster domain default
    if cfg.Kubernetes.Domain == "" {
        cfg.Kubernetes.Domain = "cluster.local"
    }
}

func ptr[T any](v T) *T {
    return &v
}
```

### Phase 3: Validation

**File: `internal/config/validate.go`**

```go
func validateTalosMachineConfig(cfg *Config) error {
    // Validate config apply mode
    validModes := []string{"auto", "reboot", "no_reboot", "staged"}
    if cfg.Talos.Machine.ConfigApplyMode != "" {
        if !contains(validModes, cfg.Talos.Machine.ConfigApplyMode) {
            return fmt.Errorf("invalid config_apply_mode: %s (valid: %v)",
                cfg.Talos.Machine.ConfigApplyMode, validModes)
        }
    }

    // Validate extra routes are valid CIDRs
    for _, route := range cfg.Talos.Machine.ExtraRoutes {
        if _, _, err := net.ParseCIDR(route); err != nil {
            return fmt.Errorf("invalid extra_route CIDR: %s", route)
        }
    }

    // Validate nameservers are valid IPs
    for _, ns := range cfg.Talos.Machine.Nameservers {
        if net.ParseIP(ns) == nil {
            return fmt.Errorf("invalid nameserver IP: %s", ns)
        }
    }

    // Validate host entries
    for _, entry := range cfg.Talos.Machine.ExtraHostEntries {
        if net.ParseIP(entry.IP) == nil {
            return fmt.Errorf("invalid host entry IP: %s", entry.IP)
        }
        if len(entry.Aliases) == 0 {
            return fmt.Errorf("host entry %s must have at least one alias", entry.IP)
        }
    }

    // Validate logging destinations
    for _, dest := range cfg.Talos.Machine.LoggingDestinations {
        if dest.Endpoint == "" {
            return fmt.Errorf("logging destination must have an endpoint")
        }
        validFormats := []string{"json_lines", ""}
        if !contains(validFormats, dest.Format) {
            return fmt.Errorf("invalid logging format: %s", dest.Format)
        }
    }

    return nil
}
```

### Phase 4: Talos Generator Enhancement

**File: `internal/platform/talos/config.go`**

The key insight from Terraform: build config patches as structured maps, then YAML-encode and merge.

```go
// TalosMachineConfigOptions holds configuration options for Talos machine config generation.
// These are passed from the config package to the generator.
type TalosMachineConfigOptions struct {
    // From config.TalosMachineConfig
    StateEncryption            bool
    EphemeralEncryption        bool
    IPv6Enabled                bool
    PublicIPv4Enabled          bool
    PublicIPv6Enabled          bool
    Nameservers                []string
    TimeServers                []string
    ExtraRoutes                []string
    ExtraHostEntries           []HostEntry
    CoreDNSEnabled             bool
    Registries                 map[string]any
    KernelArgs                 []string
    KernelModules              []KernelModule
    Sysctls                    map[string]string
    KubeletExtraMounts         []KubeletMount
    KubeletExtraArgs           map[string]string
    KubeletExtraConfig         map[string]any
    DiscoveryKubernetesEnabled bool
    DiscoveryServiceEnabled    bool
    LoggingDestinations        []LoggingDestination
    InlineManifests            []InlineManifest
    RemoteManifests            []string

    // From config.KubernetesConfig
    ClusterDomain          string
    AllowSchedulingOnCP    bool
    APIServerExtraArgs     map[string]string
    AdmissionControl       []any

    // Network context
    NetworkGateway    string // For extra routes
    NodeIPv4CIDR      string // For kubelet nodeIP
    PodIPv4CIDR       string
    ServiceIPv4CIDR   string
    EtcdSubnet        string // For etcd advertisedSubnets

    // Cilium context
    KubeProxyReplacement bool
}

type HostEntry struct {
    IP      string
    Aliases []string
}

type KernelModule struct {
    Name       string
    Parameters []string
}

type KubeletMount struct {
    Source      string
    Destination string
    Type        string
    Options     []string
}

type LoggingDestination struct {
    Endpoint  string
    Format    string
    ExtraTags map[string]string
}

type InlineManifest struct {
    Name     string
    Contents string
}
```

**Core Patch Builder Functions:**

```go
// buildControlPlanePatch builds the full config patch for a control plane node.
func buildControlPlanePatch(hostname string, opts *TalosMachineConfigOptions, installerImage string) map[string]any {
    return map[string]any{
        "machine": buildMachinePatch(hostname, opts, installerImage, true),
        "cluster": buildClusterPatch(opts, true),
    }
}

// buildWorkerPatch builds the full config patch for a worker node.
func buildWorkerPatch(hostname string, opts *TalosMachineConfigOptions, installerImage string) map[string]any {
    return map[string]any{
        "machine": buildMachinePatch(hostname, opts, installerImage, false),
        "cluster": buildClusterPatch(opts, false),
    }
}

// buildMachinePatch builds the machine section of the config patch.
func buildMachinePatch(hostname string, opts *TalosMachineConfigOptions, installerImage string, isControlPlane bool) map[string]any {
    machine := map[string]any{
        "install": map[string]any{
            "image": installerImage,
        },
    }

    // Kernel args
    if len(opts.KernelArgs) > 0 {
        machine["install"].(map[string]any)["extraKernelArgs"] = opts.KernelArgs
    }

    // Network configuration
    machine["network"] = buildNetworkPatch(hostname, opts, isControlPlane)

    // Kubelet configuration
    machine["kubelet"] = buildKubeletPatch(opts, isControlPlane)

    // Kernel modules
    if len(opts.KernelModules) > 0 {
        modules := make([]map[string]any, len(opts.KernelModules))
        for i, m := range opts.KernelModules {
            modules[i] = map[string]any{"name": m.Name}
            if len(m.Parameters) > 0 {
                modules[i]["parameters"] = m.Parameters
            }
        }
        machine["kernel"] = map[string]any{"modules": modules}
    }

    // Sysctls (with defaults matching Terraform)
    machine["sysctls"] = buildSysctlsPatch(opts)

    // Registries
    if opts.Registries != nil {
        machine["registries"] = opts.Registries
    }

    // Disk encryption
    machine["systemDiskEncryption"] = buildDiskEncryptionPatch(opts)

    // Features
    machine["features"] = buildFeaturesPatch(isControlPlane)

    // Time servers
    if len(opts.TimeServers) > 0 {
        machine["time"] = map[string]any{"servers": opts.TimeServers}
    }

    // Logging
    if len(opts.LoggingDestinations) > 0 {
        dests := make([]map[string]any, len(opts.LoggingDestinations))
        for i, d := range opts.LoggingDestinations {
            dests[i] = map[string]any{
                "endpoint": d.Endpoint,
                "format":   d.Format,
            }
            if len(d.ExtraTags) > 0 {
                dests[i]["extraTags"] = d.ExtraTags
            }
        }
        machine["logging"] = map[string]any{"destinations": dests}
    }

    return machine
}

// buildNetworkPatch builds the network section.
func buildNetworkPatch(hostname string, opts *TalosMachineConfigOptions, isControlPlane bool) map[string]any {
    network := map[string]any{
        "hostname": hostname,
    }

    // Interfaces - following Terraform pattern
    publicEnabled := opts.PublicIPv4Enabled || opts.PublicIPv6Enabled
    interfaces := []map[string]any{}

    if publicEnabled {
        eth0 := map[string]any{
            "interface": "eth0",
            "dhcp":      true,
            "dhcpOptions": map[string]any{
                "ipv4": opts.PublicIPv4Enabled,
                "ipv6": false, // IPv6 via DHCP is handled differently
            },
        }
        interfaces = append(interfaces, eth0)
    }

    // Private interface (eth1 if public enabled, eth0 otherwise)
    privateIface := "eth0"
    if publicEnabled {
        privateIface = "eth1"
    }

    eth1 := map[string]any{
        "interface": privateIface,
        "dhcp":      true,
    }

    // Extra routes
    if len(opts.ExtraRoutes) > 0 && opts.NetworkGateway != "" {
        routes := make([]map[string]any, len(opts.ExtraRoutes))
        for i, cidr := range opts.ExtraRoutes {
            routes[i] = map[string]any{
                "network": cidr,
                "gateway": opts.NetworkGateway,
                "metric":  512,
            }
        }
        eth1["routes"] = routes
    }

    interfaces = append(interfaces, eth1)
    network["interfaces"] = interfaces

    // Nameservers
    if len(opts.Nameservers) > 0 {
        network["nameservers"] = opts.Nameservers
    }

    // Extra host entries
    if len(opts.ExtraHostEntries) > 0 {
        entries := make([]map[string]any, len(opts.ExtraHostEntries))
        for i, e := range opts.ExtraHostEntries {
            entries[i] = map[string]any{
                "ip":      e.IP,
                "aliases": e.Aliases,
            }
        }
        network["extraHostEntries"] = entries
    }

    return network
}

// buildKubeletPatch builds the kubelet section.
func buildKubeletPatch(opts *TalosMachineConfigOptions, isControlPlane bool) map[string]any {
    kubelet := map[string]any{
        "extraArgs": map[string]any{
            "cloud-provider":             "external",
            "rotate-server-certificates": true,
        },
        "extraConfig": map[string]any{
            "shutdownGracePeriod":             "90s",
            "shutdownGracePeriodCriticalPods": "15s",
        },
    }

    // Merge user extra args
    if len(opts.KubeletExtraArgs) > 0 {
        args := kubelet["extraArgs"].(map[string]any)
        for k, v := range opts.KubeletExtraArgs {
            args[k] = v
        }
    }

    // Merge user extra config
    if len(opts.KubeletExtraConfig) > 0 {
        config := kubelet["extraConfig"].(map[string]any)
        for k, v := range opts.KubeletExtraConfig {
            config[k] = v
        }
    }

    // System reserved (different for CP vs worker, matching Terraform)
    config := kubelet["extraConfig"].(map[string]any)
    if isControlPlane {
        config["systemReserved"] = map[string]any{
            "cpu":               "250m",
            "memory":            "300Mi",
            "ephemeral-storage": "1Gi",
        }
        config["kubeReserved"] = map[string]any{
            "cpu":               "250m",
            "memory":            "350Mi",
            "ephemeral-storage": "1Gi",
        }
    } else {
        config["systemReserved"] = map[string]any{
            "cpu":               "100m",
            "memory":            "300Mi",
            "ephemeral-storage": "1Gi",
        }
        config["kubeReserved"] = map[string]any{
            "cpu":               "100m",
            "memory":            "350Mi",
            "ephemeral-storage": "1Gi",
        }
    }

    // Extra mounts
    if len(opts.KubeletExtraMounts) > 0 {
        mounts := make([]map[string]any, len(opts.KubeletExtraMounts))
        for i, m := range opts.KubeletExtraMounts {
            dest := m.Destination
            if dest == "" {
                dest = m.Source
            }
            mounts[i] = map[string]any{
                "source":      m.Source,
                "destination": dest,
                "type":        m.Type,
                "options":     m.Options,
            }
        }
        kubelet["extraMounts"] = mounts
    }

    // Node IP valid subnets
    if opts.NodeIPv4CIDR != "" {
        kubelet["nodeIP"] = map[string]any{
            "validSubnets": []string{opts.NodeIPv4CIDR},
        }
    }

    return kubelet
}

// buildSysctlsPatch builds sysctls with defaults.
func buildSysctlsPatch(opts *TalosMachineConfigOptions) map[string]string {
    sysctls := map[string]string{
        "net.core.somaxconn":          "65535",
        "net.core.netdev_max_backlog": "4096",
    }

    // IPv6 toggle
    ipv6Disabled := "1"
    if opts.IPv6Enabled {
        ipv6Disabled = "0"
    }
    sysctls["net.ipv6.conf.default.disable_ipv6"] = ipv6Disabled
    sysctls["net.ipv6.conf.all.disable_ipv6"] = ipv6Disabled

    // Merge user sysctls
    for k, v := range opts.Sysctls {
        sysctls[k] = v
    }

    return sysctls
}

// buildDiskEncryptionPatch builds disk encryption config.
func buildDiskEncryptionPatch(opts *TalosMachineConfigOptions) map[string]any {
    enc := map[string]any{}

    if opts.StateEncryption {
        enc["state"] = map[string]any{
            "provider": "luks2",
            "options":  []string{"no_read_workqueue", "no_write_workqueue"},
            "keys": []map[string]any{
                {
                    "nodeID": map[string]any{},
                    "slot":   0,
                },
            },
        }
    }

    if opts.EphemeralEncryption {
        enc["ephemeral"] = map[string]any{
            "provider": "luks2",
            "options":  []string{"no_read_workqueue", "no_write_workqueue"},
            "keys": []map[string]any{
                {
                    "nodeID": map[string]any{},
                    "slot":   0,
                },
            },
        }
    }

    return enc
}

// buildFeaturesPatch builds features section.
func buildFeaturesPatch(isControlPlane bool) map[string]any {
    features := map[string]any{
        "hostDNS": map[string]any{
            "enabled":              true,
            "forwardKubeDNSToHost": false,
            "resolveMemberNames":   true,
        },
    }

    // Control plane gets Talos API access for etcd backups
    if isControlPlane {
        features["kubernetesTalosAPIAccess"] = map[string]any{
            "enabled": true,
            "allowedRoles": []string{
                "os:reader",
                "os:etcd:backup",
            },
            "allowedKubernetesNamespaces": []string{"kube-system"},
        }
    }

    return features
}

// buildClusterPatch builds the cluster section of the config patch.
func buildClusterPatch(opts *TalosMachineConfigOptions, isControlPlane bool) map[string]any {
    cluster := map[string]any{
        "network": map[string]any{
            "dnsDomain": opts.ClusterDomain,
            "cni":       map[string]any{"name": "none"}, // CNI managed by Cilium
        },
        "proxy": map[string]any{
            "disabled": opts.KubeProxyReplacement,
        },
        "discovery": buildDiscoveryPatch(opts),
    }

    // Pod and service subnets
    if opts.PodIPv4CIDR != "" {
        cluster["network"].(map[string]any)["podSubnets"] = []string{opts.PodIPv4CIDR}
    }
    if opts.ServiceIPv4CIDR != "" {
        cluster["network"].(map[string]any)["serviceSubnets"] = []string{opts.ServiceIPv4CIDR}
    }

    // CoreDNS
    cluster["coreDNS"] = map[string]any{
        "disabled": !opts.CoreDNSEnabled,
    }

    // Control plane specific settings
    if isControlPlane {
        cluster["allowSchedulingOnControlPlanes"] = opts.AllowSchedulingOnCP

        // API server
        apiServer := map[string]any{
            "extraArgs": map[string]any{
                "enable-aggregator-routing": true,
            },
        }

        // Merge user API server args
        if len(opts.APIServerExtraArgs) > 0 {
            args := apiServer["extraArgs"].(map[string]any)
            for k, v := range opts.APIServerExtraArgs {
                args[k] = v
            }
        }

        // Admission control
        if len(opts.AdmissionControl) > 0 {
            apiServer["admissionControl"] = opts.AdmissionControl
        }

        cluster["apiServer"] = apiServer

        // Controller manager
        cluster["controllerManager"] = map[string]any{
            "extraArgs": map[string]any{
                "cloud-provider": "external",
                "bind-address":   "0.0.0.0",
            },
        }

        // Scheduler
        cluster["scheduler"] = map[string]any{
            "extraArgs": map[string]any{
                "bind-address": "0.0.0.0",
            },
        }

        // etcd
        if opts.EtcdSubnet != "" {
            cluster["etcd"] = map[string]any{
                "advertisedSubnets": []string{opts.EtcdSubnet},
                "extraArgs": map[string]any{
                    "listen-metrics-urls": "http://0.0.0.0:2381",
                },
            }
        }

        // Admin kubeconfig lifetime
        cluster["adminKubeconfig"] = map[string]any{
            "certLifetime": "87600h", // 10 years
        }

        // Inline manifests
        if len(opts.InlineManifests) > 0 {
            manifests := make([]map[string]any, len(opts.InlineManifests))
            for i, m := range opts.InlineManifests {
                manifests[i] = map[string]any{
                    "name":     m.Name,
                    "contents": m.Contents,
                }
            }
            cluster["inlineManifests"] = manifests
        }

        // External cloud provider with remote manifests
        ecp := map[string]any{"enabled": true}
        if len(opts.RemoteManifests) > 0 {
            ecp["manifests"] = opts.RemoteManifests
        }
        cluster["externalCloudProvider"] = ecp
    }

    return cluster
}

// buildDiscoveryPatch builds the discovery section.
func buildDiscoveryPatch(opts *TalosMachineConfigOptions) map[string]any {
    enabled := opts.DiscoveryKubernetesEnabled || opts.DiscoveryServiceEnabled

    return map[string]any{
        "enabled": enabled,
        "registries": map[string]any{
            "kubernetes": map[string]any{"disabled": !opts.DiscoveryKubernetesEnabled},
            "service":    map[string]any{"disabled": !opts.DiscoveryServiceEnabled},
        },
    }
}
```

### Phase 5: Integration with Generator

Update the Generator to accept options and apply patches:

```go
// GenerateControlPlaneConfig generates the configuration for a control plane node.
func (g *Generator) GenerateControlPlaneConfig(san []string, hostname string, opts *TalosMachineConfigOptions) ([]byte, error) {
    genOpts := []generate.Option{
        generate.WithAdditionalSubjectAltNames(san),
    }

    baseConfig, err := g.generateConfig(machine.TypeControlPlane, hostname, genOpts...)
    if err != nil {
        return nil, err
    }

    // Build and apply patch
    patch := buildControlPlanePatch(hostname, opts, g.getInstallerImage())
    return applyConfigPatch(baseConfig, patch)
}

// GenerateWorkerConfig generates the configuration for a worker node.
func (g *Generator) GenerateWorkerConfig(hostname string, opts *TalosMachineConfigOptions) ([]byte, error) {
    baseConfig, err := g.generateConfig(machine.TypeWorker, hostname)
    if err != nil {
        return nil, err
    }

    // Build and apply patch
    patch := buildWorkerPatch(hostname, opts, g.getInstallerImage())
    return applyConfigPatch(baseConfig, patch)
}

// applyConfigPatch merges a patch into the base config.
func applyConfigPatch(baseConfig []byte, patch map[string]any) ([]byte, error) {
    var base map[string]any
    if err := yaml.Unmarshal(baseConfig, &base); err != nil {
        return nil, fmt.Errorf("failed to unmarshal base config: %w", err)
    }

    // Deep merge patch into base
    merged := deepMerge(base, patch)

    return yaml.Marshal(merged)
}

// deepMerge recursively merges src into dst.
func deepMerge(dst, src map[string]any) map[string]any {
    result := make(map[string]any)

    // Copy dst
    for k, v := range dst {
        result[k] = v
    }

    // Merge src
    for k, v := range src {
        if srcMap, ok := v.(map[string]any); ok {
            if dstMap, ok := result[k].(map[string]any); ok {
                result[k] = deepMerge(dstMap, srcMap)
                continue
            }
        }
        result[k] = v
    }

    return result
}
```

### Phase 6: Testing Strategy

1. **Unit Tests**: Test each patch builder function independently
2. **Integration Tests**: Test full config generation with various option combinations
3. **Real Cluster Test**: Deploy a cluster with various options enabled

---

## File Summary

| File | Action | Description |
|------|--------|-------------|
| `internal/config/types.go` | Modify | Add TalosMachineConfig types |
| `internal/config/defaults.go` | Modify | Add defaults for new options |
| `internal/config/validate.go` | Modify | Add validation rules |
| `internal/platform/talos/config.go` | Modify | Add patch builder functions |
| `internal/platform/talos/config_test.go` | Modify | Add comprehensive tests |
| `internal/provisioning/context.go` | Modify | Pass config options to generator |
| `internal/provisioning/cluster/bootstrap.go` | Modify | Use new generator API |

---

## Incremental Implementation Order

To minimize risk and allow testing at each step:

### Increment 1: Disk Encryption (Highest Value)
- Add `StateEncryption`, `EphemeralEncryption` to config
- Implement `buildDiskEncryptionPatch`
- Test with real cluster

### Increment 2: Network Basics
- Add `Nameservers`, `TimeServers`
- Implement `buildNetworkPatch` basics
- Test nameserver resolution

### Increment 3: Host DNS & Entries
- Add `ExtraHostEntries`, `CoreDNSEnabled`
- Implement host DNS features
- Test with custom host entries

### Increment 4: Kubelet Configuration
- Add `KubeletExtraArgs`, `KubeletExtraConfig`, `KubeletExtraMounts`
- Implement `buildKubeletPatch`
- Test with Longhorn mounts

### Increment 5: Kernel & Sysctls
- Add `KernelArgs`, `KernelModules`, `Sysctls`
- Implement kernel/sysctl patches
- Test IPv6 disable, custom sysctls

### Increment 6: Discovery & Logging
- Add `DiscoveryKubernetesEnabled`, `DiscoveryServiceEnabled`, `LoggingDestinations`
- Implement discovery/logging patches
- Test cluster discovery

### Increment 7: Bootstrap Manifests
- Add `InlineManifests`, `RemoteManifests`
- Implement manifest injection
- Test with custom manifests

### Increment 8: Cluster Configuration
- Add `ClusterDomain`, `AllowSchedulingOnCP`, `APIServerExtraArgs`
- Implement cluster-level patches
- Test full configuration

---

## Success Criteria

1. All new config options parse correctly from YAML
2. Generated Talos configs include all specified options
3. Defaults match Terraform behavior exactly
4. Disk encryption works on new clusters
5. All tests pass
6. Existing functionality not regressed
7. Real cluster deployment succeeds with enhanced options
