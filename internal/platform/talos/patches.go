package talos

import (
	"fmt"

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
	CoreDNSEnabled             bool
	DiscoveryKubernetesEnabled bool
	DiscoveryServiceEnabled    bool

	// From config.KubernetesConfig
	ClusterDomain       string
	AllowSchedulingOnCP bool

	// Network context (from provisioning state)
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

	return &MachineConfigOptions{
		SchematicID:                cfg.Talos.SchematicID,
		StateEncryption:            derefBool(m.StateEncryption, true),
		EphemeralEncryption:        derefBool(m.EphemeralEncryption, true),
		IPv6Enabled:                derefBool(m.IPv6Enabled, true),
		PublicIPv4Enabled:          derefBool(m.PublicIPv4Enabled, true),
		PublicIPv6Enabled:          derefBool(m.PublicIPv6Enabled, true),
		CoreDNSEnabled:             derefBool(m.CoreDNSEnabled, true),
		DiscoveryKubernetesEnabled: derefBool(m.DiscoveryKubernetesEnabled, false),
		DiscoveryServiceEnabled:    derefBool(m.DiscoveryServiceEnabled, true),
		ClusterDomain:              cfg.Kubernetes.Domain,
		AllowSchedulingOnCP:        derefBool(cfg.Kubernetes.AllowSchedulingOnCP, false),
		NodeIPv4CIDR:               cfg.Network.NodeIPv4CIDR,
		PodIPv4CIDR:                cfg.Network.PodIPv4CIDR,
		ServiceIPv4CIDR:            cfg.Network.ServiceIPv4CIDR,
		EtcdSubnet:                 cfg.Network.NodeIPv4CIDR,
		KubeProxyReplacement:       cfg.Addons.Cilium.KubeProxyReplacementEnabled,
	}
}

// derefBool returns the value of a *bool, or the default if nil.
func derefBool(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// buildControlPlanePatch builds the full config patch for a control plane node.
func buildControlPlanePatch(hostname string, serverID int64, opts *MachineConfigOptions, installerImage string, certSANs []string) map[string]any {
	return map[string]any{
		"machine": buildMachinePatch(hostname, serverID, opts, installerImage, certSANs, true),
		"cluster": buildClusterPatch(opts, true),
	}
}

// buildWorkerPatch builds the full config patch for a worker node.
func buildWorkerPatch(hostname string, serverID int64, opts *MachineConfigOptions, installerImage string, certSANs []string) map[string]any {
	return map[string]any{
		"machine": buildMachinePatch(hostname, serverID, opts, installerImage, certSANs, false),
		"cluster": buildClusterPatch(opts, false),
	}
}

// buildMachinePatch builds the machine section of the config patch.
func buildMachinePatch(hostname string, serverID int64, opts *MachineConfigOptions, installerImage string, certSANs []string, isControlPlane bool) map[string]any {
	machine := map[string]any{}

	// Install section
	machine["install"] = map[string]any{
		"image": installerImage,
	}

	// Certificate SANs
	if len(certSANs) > 0 {
		machine["certSANs"] = certSANs
	}

	// Node labels - include nodeid for CCM integration
	// The nodeid label allows the Hetzner CCM to properly identify nodes by their server ID.
	if serverID > 0 {
		machine["nodeLabels"] = map[string]any{
			"nodeid": fmt.Sprintf("%d", serverID),
		}
	}

	// Network configuration
	machine["network"] = buildNetworkPatch(hostname, opts, isControlPlane)

	// Kubelet configuration
	machine["kubelet"] = buildKubeletPatch(opts, isControlPlane, serverID)

	// Sysctls
	machine["sysctls"] = buildSysctlsPatch(opts)

	// Disk encryption
	if enc := buildDiskEncryptionPatch(opts); len(enc) > 0 {
		machine["systemDiskEncryption"] = enc
	}

	// Features
	machine["features"] = buildFeaturesPatch(isControlPlane)

	return machine
}

// buildNetworkPatch builds the network section.
// The _ parameter is reserved for future node-type-specific network configuration.
func buildNetworkPatch(hostname string, opts *MachineConfigOptions, _ bool) map[string]any {
	network := map[string]any{
		"hostname": hostname,
	}

	// Interfaces - interface configuration
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

	interfaces = append(interfaces, eth1)
	network["interfaces"] = interfaces

	return network
}

// buildKubeletPatch builds the kubelet section.
func buildKubeletPatch(opts *MachineConfigOptions, isControlPlane bool, serverID int64) map[string]any {
	// Base extra args
	// Note: rotate-server-certificates is NOT enabled because it requires a CSR approver.
	// Without a CSR approver, the kubelet has no serving certificate, causing "tls: internal error".
	// Talos manages kubelet certificates internally, so this flag is not needed.
	extraArgs := map[string]any{
		"cloud-provider": "external",
	}

	// Set provider-id for Hetzner CCM integration
	// This tells the kubelet its cloud provider identity, allowing the Hetzner CCM to properly
	// recognize and manage the node. Format: hcloud://<server-id>
	// Without this, metal images would use talos://metal/<ip> which the CCM can't recognize.
	if serverID > 0 {
		extraArgs["provider-id"] = fmt.Sprintf("hcloud://%d", serverID)
	}

	// Base extra config
	extraConfig := map[string]any{
		"shutdownGracePeriod":             "90s",
		"shutdownGracePeriodCriticalPods": "15s",
	}

	// System/kube reserved (different for CP vs worker)
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

	kubelet := map[string]any{
		"extraArgs":   extraArgs,
		"extraConfig": extraConfig,
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

	return sysctls
}

// buildDiskEncryptionPatch builds disk encryption config.
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
		cluster["apiServer"] = map[string]any{
			"extraArgs": map[string]any{
				"enable-aggregator-routing": true,
			},
		}

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

		// Admin kubeconfig lifetime (10 years)
		cluster["adminKubeconfig"] = map[string]any{
			"certLifetime": "87600h",
		}

		// External cloud provider
		cluster["externalCloudProvider"] = map[string]any{"enabled": true}
	}

	return cluster
}

// buildDiscoveryPatch builds the discovery section.
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
