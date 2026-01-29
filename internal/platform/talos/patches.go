package talos

import (
	"github.com/imamik/k8zner/internal/config"
)

// MachineConfigOptions holds all options needed to build Talos machine config patches.
// These are derived from config.Config and runtime context.
type MachineConfigOptions struct {
	// From config.TalosConfig
	SchematicID string // For custom installer image from factory.talos.dev

	// From config.TalosMachineConfig
	StateEncryption            bool
	EphemeralEncryption        bool
	IPv6Enabled                bool
	PublicIPv4Enabled          bool
	PublicIPv6Enabled          bool
	Nameservers                []string
	TimeServers                []string
	ExtraRoutes                []string
	ExtraHostEntries           []config.TalosHostEntry
	CoreDNSEnabled             bool
	Registries                 *config.TalosRegistryConfig
	KernelArgs                 []string
	KernelModules              []config.TalosKernelModule
	Sysctls                    map[string]string
	KubeletExtraMounts         []config.TalosKubeletMount
	DiscoveryKubernetesEnabled bool
	DiscoveryServiceEnabled    bool
	LoggingDestinations        []config.TalosLoggingDestination
	InlineManifests            []config.TalosInlineManifest
	RemoteManifests            []string

	// From config.KubernetesConfig
	ClusterDomain       string
	AllowSchedulingOnCP bool
	APIServerExtraArgs  map[string]string
	KubeletExtraArgs    map[string]string
	KubeletExtraConfig  map[string]any

	// Network context (from provisioning state)
	NetworkGateway  string // First usable IP in network (for extra routes)
	NodeIPv4CIDR    string // For kubelet nodeIP.validSubnets
	PodIPv4CIDR     string // For cluster.network.podSubnets
	ServiceIPv4CIDR string // For cluster.network.serviceSubnets
	EtcdSubnet      string // For cluster.etcd.advertisedSubnets

	// Cilium context
	KubeProxyReplacement bool
}

// NewMachineConfigOptions creates MachineConfigOptions from a Config.
// Network context fields must be set separately after infrastructure provisioning.
func NewMachineConfigOptions(cfg *config.Config) *MachineConfigOptions {
	m := &cfg.Talos.Machine
	k := &cfg.Kubernetes

	opts := &MachineConfigOptions{
		// Talos Config
		SchematicID: cfg.Talos.SchematicID,

		// Talos Machine Config
		StateEncryption:            derefBool(m.StateEncryption, true),
		EphemeralEncryption:        derefBool(m.EphemeralEncryption, true),
		IPv6Enabled:                derefBool(m.IPv6Enabled, true),
		PublicIPv4Enabled:          derefBool(m.PublicIPv4Enabled, true),
		PublicIPv6Enabled:          derefBool(m.PublicIPv6Enabled, true),
		Nameservers:                m.Nameservers,
		TimeServers:                m.TimeServers,
		ExtraRoutes:                m.ExtraRoutes,
		ExtraHostEntries:           m.ExtraHostEntries,
		CoreDNSEnabled:             derefBool(m.CoreDNSEnabled, true),
		Registries:                 m.Registries,
		KernelArgs:                 m.KernelArgs,
		KernelModules:              m.KernelModules,
		Sysctls:                    m.Sysctls,
		KubeletExtraMounts:         m.KubeletExtraMounts,
		DiscoveryKubernetesEnabled: derefBool(m.DiscoveryKubernetesEnabled, false),
		DiscoveryServiceEnabled:    derefBool(m.DiscoveryServiceEnabled, true),
		LoggingDestinations:        m.LoggingDestinations,
		InlineManifests:            m.InlineManifests,
		RemoteManifests:            m.RemoteManifests,

		// Kubernetes Config
		ClusterDomain:       k.Domain,
		AllowSchedulingOnCP: derefBool(k.AllowSchedulingOnCP, false),
		APIServerExtraArgs:  k.APIServerExtraArgs,
		KubeletExtraArgs:    k.KubeletExtraArgs,
		KubeletExtraConfig:  k.KubeletExtraConfig,

		// Network context (from config)
		// NodeIPv4CIDR is critical for kubelet to use the correct node IP
		// and etcd to advertise on the correct subnet
		NodeIPv4CIDR:    cfg.Network.NodeIPv4CIDR,
		PodIPv4CIDR:     cfg.Network.PodIPv4CIDR,
		ServiceIPv4CIDR: cfg.Network.ServiceIPv4CIDR,
		EtcdSubnet:      cfg.Network.NodeIPv4CIDR, // etcd should advertise on the node subnet

		// Cilium context
		KubeProxyReplacement: cfg.Addons.Cilium.KubeProxyReplacementEnabled,
	}

	return opts
}

// derefBool returns the value of a *bool, or the default if nil.
func derefBool(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// buildControlPlanePatch builds the full config patch for a control plane node.
// See: terraform/talos_config.tf local.control_plane_talos_config_patch
func buildControlPlanePatch(hostname string, opts *MachineConfigOptions, installerImage string, certSANs []string) map[string]any {
	return map[string]any{
		"machine": buildMachinePatch(hostname, opts, installerImage, certSANs, true),
		"cluster": buildClusterPatch(opts, true),
	}
}

// buildWorkerPatch builds the full config patch for a worker node.
// See: terraform/talos_config.tf local.worker_talos_config_patch
func buildWorkerPatch(hostname string, opts *MachineConfigOptions, installerImage string, certSANs []string) map[string]any {
	return map[string]any{
		"machine": buildMachinePatch(hostname, opts, installerImage, certSANs, false),
		"cluster": buildClusterPatch(opts, false),
	}
}

// buildMachinePatch builds the machine section of the config patch.
// See: terraform/talos_config.tf control_plane_talos_config_patch.machine
func buildMachinePatch(hostname string, opts *MachineConfigOptions, installerImage string, certSANs []string, isControlPlane bool) map[string]any {
	machine := map[string]any{}

	// Install section
	install := map[string]any{
		"image": installerImage,
	}
	if len(opts.KernelArgs) > 0 {
		install["extraKernelArgs"] = opts.KernelArgs
	}
	machine["install"] = install

	// Certificate SANs
	if len(certSANs) > 0 {
		machine["certSANs"] = certSANs
	}

	// Network configuration
	machine["network"] = buildNetworkPatch(hostname, opts, isControlPlane)

	// Kubelet configuration
	machine["kubelet"] = buildKubeletPatch(opts, isControlPlane)

	// Kernel modules
	if len(opts.KernelModules) > 0 {
		modules := make([]map[string]any, len(opts.KernelModules))
		for i, m := range opts.KernelModules {
			mod := map[string]any{"name": m.Name}
			if len(m.Parameters) > 0 {
				mod["parameters"] = m.Parameters
			}
			modules[i] = mod
		}
		machine["kernel"] = map[string]any{"modules": modules}
	}

	// Sysctls (with defaults matching Terraform)
	machine["sysctls"] = buildSysctlsPatch(opts)

	// Registries
	if opts.Registries != nil && len(opts.Registries.Mirrors) > 0 {
		mirrors := make(map[string]any)
		for name, mirror := range opts.Registries.Mirrors {
			mirrors[name] = map[string]any{
				"endpoints": mirror.Endpoints,
			}
		}
		machine["registries"] = map[string]any{"mirrors": mirrors}
	}

	// Disk encryption
	if enc := buildDiskEncryptionPatch(opts); len(enc) > 0 {
		machine["systemDiskEncryption"] = enc
	}

	// Features
	machine["features"] = buildFeaturesPatch(isControlPlane)

	// Time servers
	if len(opts.TimeServers) > 0 {
		machine["time"] = map[string]any{"servers": opts.TimeServers}
	}

	// Logging destinations
	if len(opts.LoggingDestinations) > 0 {
		dests := make([]map[string]any, len(opts.LoggingDestinations))
		for i, d := range opts.LoggingDestinations {
			dest := map[string]any{"endpoint": d.Endpoint}
			if d.Format != "" {
				dest["format"] = d.Format
			}
			if len(d.ExtraTags) > 0 {
				dest["extraTags"] = d.ExtraTags
			}
			dests[i] = dest
		}
		machine["logging"] = map[string]any{"destinations": dests}
	}

	return machine
}

// buildNetworkPatch builds the network section.
// See: terraform/talos_config.tf control_plane_talos_config_patch.machine.network
// The _ parameter is reserved for future node-type-specific network configuration.
func buildNetworkPatch(hostname string, opts *MachineConfigOptions, _ bool) map[string]any {
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
				"ipv6": false, // IPv6 via DHCP handled differently on Hetzner
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

	// Extra routes through network gateway
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
// See: terraform/talos_config.tf control_plane_talos_config_patch.machine.kubelet
func buildKubeletPatch(opts *MachineConfigOptions, isControlPlane bool) map[string]any {
	// Base extra args (matching Terraform)
	// Note: rotate-server-certificates is NOT enabled because it requires a CSR approver.
	// Without a CSR approver, the kubelet has no serving certificate, causing "tls: internal error".
	// Talos manages kubelet certificates internally, so this flag is not needed.
	extraArgs := map[string]any{
		"cloud-provider": "external",
	}

	// Merge user extra args
	for k, v := range opts.KubeletExtraArgs {
		extraArgs[k] = v
	}

	// Base extra config (matching Terraform)
	extraConfig := map[string]any{
		"shutdownGracePeriod":             "90s",
		"shutdownGracePeriodCriticalPods": "15s",
	}

	// System/kube reserved (different for CP vs worker, matching Terraform)
	if isControlPlane {
		extraConfig["systemReserved"] = map[string]any{
			"cpu":               "250m",
			"memory":            "300Mi",
			"ephemeral-storage": "1Gi",
		}
		extraConfig["kubeReserved"] = map[string]any{
			"cpu":               "250m",
			"memory":            "350Mi",
			"ephemeral-storage": "1Gi",
		}
	} else {
		extraConfig["systemReserved"] = map[string]any{
			"cpu":               "100m",
			"memory":            "300Mi",
			"ephemeral-storage": "1Gi",
		}
		extraConfig["kubeReserved"] = map[string]any{
			"cpu":               "100m",
			"memory":            "350Mi",
			"ephemeral-storage": "1Gi",
		}
	}

	// Merge user extra config
	for k, v := range opts.KubeletExtraConfig {
		extraConfig[k] = v
	}

	kubelet := map[string]any{
		"extraArgs":   extraArgs,
		"extraConfig": extraConfig,
	}

	// Extra mounts
	if len(opts.KubeletExtraMounts) > 0 {
		mounts := make([]map[string]any, len(opts.KubeletExtraMounts))
		for i, m := range opts.KubeletExtraMounts {
			dest := m.Destination
			if dest == "" {
				dest = m.Source
			}
			typ := m.Type
			if typ == "" {
				typ = "bind"
			}
			options := m.Options
			if len(options) == 0 {
				options = []string{"bind", "rshared", "rw"}
			}
			mounts[i] = map[string]any{
				"source":      m.Source,
				"destination": dest,
				"type":        typ,
				"options":     options,
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

// buildSysctlsPatch builds sysctls with defaults matching Terraform.
// See: terraform/talos_config.tf control_plane_talos_config_patch.machine.sysctls
func buildSysctlsPatch(opts *MachineConfigOptions) map[string]string {
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
// See: terraform/talos_config.tf local.talos_system_disk_encryption
func buildDiskEncryptionPatch(opts *MachineConfigOptions) map[string]any {
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
// See: terraform/talos_config.tf control_plane_talos_config_patch.machine.features
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
// See: terraform/talos_config.tf control_plane_talos_config_patch.cluster
func buildClusterPatch(opts *MachineConfigOptions, isControlPlane bool) map[string]any {
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

		// Admin kubeconfig lifetime (10 years, matching Terraform)
		cluster["adminKubeconfig"] = map[string]any{
			"certLifetime": "87600h",
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
// See: terraform/talos_config.tf local.talos_discovery
func buildDiscoveryPatch(opts *MachineConfigOptions) map[string]any {
	enabled := opts.DiscoveryKubernetesEnabled || opts.DiscoveryServiceEnabled

	return map[string]any{
		"enabled": enabled,
		"registries": map[string]any{
			"kubernetes": map[string]any{"disabled": !opts.DiscoveryKubernetesEnabled},
			"service":    map[string]any{"disabled": !opts.DiscoveryServiceEnabled},
		},
	}
}
