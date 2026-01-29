package v2

import (
	"github.com/imamik/k8zner/internal/config"
)

// Expand converts a simplified v2 Config to the full internal config.Config
// that the provisioning layer expects.
//
// This is where all the opinionated defaults are applied:
// - IPv6-only nodes
// - Hardcoded addon stack
// - Pinned versions
// - Secure network configuration
func Expand(cfg *Config) (*config.Config, error) {
	vm := DefaultVersionMatrix()

	// Boolean helpers
	boolPtr := func(b bool) *bool { return &b }

	// Create the full internal config
	internal := &config.Config{
		// Basic cluster info
		ClusterName: cfg.Name,
		Location:    string(cfg.Region),

		// Access configuration
		ClusterAccess:             "public", // LB is public, nodes are IPv6-only
		GracefulDestroy:           true,
		HealthcheckEnabled:        boolPtr(true),
		PrerequisitesCheckEnabled: boolPtr(true),
		DeleteProtection:          false,

		// Config output paths
		KubeconfigPath:  "./secrets/kubeconfig",
		TalosconfigPath: "./secrets/talosconfig",

		// Talosctl settings
		TalosctlVersionCheckEnabled: boolPtr(true),
		TalosctlRetryCount:          5,

		// Network configuration
		Network: expandNetwork(cfg),

		// Firewall configuration
		Firewall: expandFirewall(cfg),

		// Control plane
		ControlPlane: expandControlPlane(cfg),

		// Workers
		Workers: expandWorkers(cfg),

		// Ingress load balancer
		Ingress: expandIngress(cfg),

		// Talos configuration
		Talos: expandTalos(cfg, vm),

		// Kubernetes configuration
		Kubernetes: expandKubernetes(cfg, vm),

		// Addons
		Addons: expandAddons(cfg, vm),
	}

	return internal, nil
}

func expandNetwork(cfg *Config) config.NetworkConfig {
	return config.NetworkConfig{
		IPv4CIDR:        NetworkCIDR,
		NodeIPv4CIDR:    NodeCIDR,
		ServiceIPv4CIDR: ServiceCIDR,
		PodIPv4CIDR:     PodCIDR,
		Zone:            NetworkZone(cfg.Region),
	}
}

func expandFirewall(cfg *Config) config.FirewallConfig {
	boolPtr := func(b bool) *bool { return &b }

	return config.FirewallConfig{
		// Auto-detect current IP for API access
		UseCurrentIPv4: boolPtr(true),
		UseCurrentIPv6: boolPtr(true),
	}
}

func expandControlPlane(cfg *Config) config.ControlPlaneConfig {
	cpCount := cfg.ControlPlaneCount()

	return config.ControlPlaneConfig{
		NodePools: []config.ControlPlaneNodePool{
			{
				Name:       "control-plane",
				Location:   string(cfg.Region),
				ServerType: ControlPlaneServerType,
				Count:      cpCount,
				Labels: map[string]string{
					"node.kubernetes.io/role": "control-plane",
				},
			},
		},
		// Enable public VIP for HA clusters
		PublicVIPIPv4Enabled: cfg.Mode == ModeHA,
	}
}

func expandWorkers(cfg *Config) []config.WorkerNodePool {
	return []config.WorkerNodePool{
		{
			Name:       "workers",
			Location:   string(cfg.Region),
			ServerType: string(cfg.Workers.Size),
			Count:      cfg.Workers.Count,
			Labels: map[string]string{
				"node.kubernetes.io/role": "worker",
			},
		},
	}
}

func expandIngress(cfg *Config) config.IngressConfig {
	// Dev mode: No separate ingress LB - Traefik uses hostNetwork on workers
	// HA mode: Dedicated ingress LB for high availability
	return config.IngressConfig{
		Enabled:          cfg.Mode == ModeHA,
		LoadBalancerType: LoadBalancerType,
		PublicNetwork:    true,
		Algorithm:        "round_robin",
	}
}

func expandTalos(cfg *Config, vm VersionMatrix) config.TalosConfig {
	boolPtr := func(b bool) *bool { return &b }

	return config.TalosConfig{
		Version: vm.Talos,
		Machine: config.TalosMachineConfig{
			// IPv6-only configuration
			IPv6Enabled:       boolPtr(true),
			PublicIPv4Enabled: boolPtr(false), // No public IPv4!
			PublicIPv6Enabled: boolPtr(true),

			// Disk encryption
			StateEncryption:     boolPtr(true),
			EphemeralEncryption: boolPtr(true),

			// CoreDNS
			CoreDNSEnabled: boolPtr(true),

			// Discovery
			DiscoveryKubernetesEnabled: boolPtr(true),
			DiscoveryServiceEnabled:    boolPtr(true),

			// Config apply mode
			ConfigApplyMode: "auto",
		},
	}
}

func expandKubernetes(cfg *Config, vm VersionMatrix) config.KubernetesConfig {
	boolPtr := func(b bool) *bool { return &b }

	return config.KubernetesConfig{
		Version: vm.Kubernetes,
		Domain:  "cluster.local",

		// Enable API load balancer for HA mode
		APILoadBalancerEnabled:       cfg.Mode == ModeHA,
		APILoadBalancerPublicNetwork: boolPtr(true),

		// Allow scheduling on control plane only in dev mode
		AllowSchedulingOnCP: boolPtr(cfg.Mode == ModeDev),
	}
}

// traefikKind returns the deployment kind for Traefik.
// Always uses DaemonSet with hostNetwork since we use infrastructure-level
// load balancers that target worker nodes directly.
func traefikKind(_ bool) string {
	// DaemonSet ensures Traefik runs on each worker, binding to host ports 80/443.
	// The infrastructure LB (or direct node access) routes traffic to these ports.
	return "DaemonSet"
}

func expandAddons(cfg *Config, vm VersionMatrix) config.AddonsConfig {
	hasDomain := cfg.HasDomain()
	boolPtr := func(b bool) *bool { return &b }

	// Dev mode uses hostNetwork for Traefik to avoid creating a separate ingress LB
	// HA mode uses LoadBalancer service with a dedicated ingress LB
	isDevMode := cfg.Mode == ModeDev

	return config.AddonsConfig{
		// Hetzner Cloud Controller Manager - always enabled
		CCM: config.CCMConfig{
			Enabled: true,
		},

		// Hetzner CSI Driver - always enabled
		CSI: config.CSIConfig{
			Enabled:             true,
			DefaultStorageClass: true,
		},

		// Cilium CNI - always enabled with kube-proxy replacement
		Cilium: config.CiliumConfig{
			Enabled:                     true,
			KubeProxyReplacementEnabled: true,
			RoutingMode:                 "native",
			HubbleEnabled:               true,
			HubbleRelayEnabled:          true,
			HubbleUIEnabled:             true,
		},

		// Traefik ingress - always enabled (replaces ingress-nginx)
		// Always uses DaemonSet with hostNetwork to bind to host ports 80/443.
		// Dev mode: Traffic goes directly to worker node IPs (or through API LB if extended)
		// HA mode: Dedicated ingress LB routes to Traefik on workers via private network
		Traefik: config.TraefikConfig{
			Enabled:               true,
			Kind:                  traefikKind(isDevMode),
			HostNetwork:          boolPtr(true), // Always use hostNetwork
			ExternalTrafficPolicy: "Local",
			IngressClass:          "traefik",
		},

		// Ingress-nginx - disabled (we use Traefik)
		IngressNginx: config.IngressNginxConfig{
			Enabled: false,
		},

		// cert-manager - always enabled
		CertManager: config.CertManagerConfig{
			Enabled: true,
			Cloudflare: config.CertManagerCloudflareConfig{
				Enabled:    hasDomain,
				Production: true, // Use production Let's Encrypt
			},
		},

		// Metrics server - always enabled
		MetricsServer: config.MetricsServerConfig{
			Enabled: true,
		},

		// ArgoCD - always enabled
		ArgoCD: config.ArgoCDConfig{
			Enabled: true,
			HA:      cfg.Mode == ModeHA,
		},

		// Gateway API CRDs - always enabled
		GatewayAPICRDs: config.GatewayAPICRDsConfig{
			Enabled: true,
		},

		// Prometheus Operator CRDs - always enabled
		PrometheusOperatorCRDs: config.PrometheusOperatorCRDsConfig{
			Enabled: true,
		},

		// Talos CCM - always enabled
		TalosCCM: config.TalosCCMConfig{
			Enabled: true,
		},

		// Cloudflare - enabled only when domain is set
		Cloudflare: config.CloudflareConfig{
			Enabled: hasDomain,
			Domain:  cfg.Domain,
		},

		// External DNS - enabled only when domain is set
		ExternalDNS: config.ExternalDNSConfig{
			Enabled:    hasDomain,
			TXTOwnerID: cfg.Name,
			Policy:     "sync",
			Sources:    []string{"ingress"},
		},
	}
}
