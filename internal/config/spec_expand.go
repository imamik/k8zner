package config

import (
	"os"
)

// boolPtr returns a pointer to a boolean value.
func boolPtr(b bool) *bool { return &b }

// ExpandSpec converts a simplified Spec to the full internal Config
// that the provisioning layer expects.
//
// This is where all the opinionated defaults are applied:
// - IPv6-only nodes
// - Hardcoded addon stack
// - Pinned versions
// - Secure network configuration
func ExpandSpec(cfg *Spec) (*Config, error) {
	vm := DefaultVersionMatrix()

	// Create the full internal config
	internal := &Config{
		// Basic cluster info
		ClusterName: cfg.Name,
		Location:    string(cfg.Region),
		HCloudToken: os.Getenv("HCLOUD_TOKEN"),

		// Access configuration
		ClusterAccess:      "public", // LB is public, nodes are IPv6-only
		GracefulDestroy:    true,
		HealthcheckEnabled: boolPtr(true),
		DeleteProtection:   false,

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

func expandNetwork(cfg *Spec) NetworkConfig {
	return NetworkConfig{
		IPv4CIDR:           NetworkCIDR,
		NodeIPv4CIDR:       NodeCIDR,
		ServiceIPv4CIDR:    ServiceCIDR,
		PodIPv4CIDR:        PodCIDR,
		Zone:               NetworkZone(cfg.Region),
		NodeIPv4SubnetMask: 25, // /25 subnets for each role (126 IPs per subnet)
	}
}

func expandFirewall(cfg *Spec) FirewallConfig {
	return FirewallConfig{
		// Auto-detect current IP for API access
		UseCurrentIPv4: boolPtr(true),
		UseCurrentIPv6: boolPtr(true),
		// No ExtraRules needed: Traefik uses LoadBalancer service,
		// so the Hetzner LB handles ingress traffic (not node ports 80/443).
	}
}

func expandControlPlane(cfg *Spec) ControlPlaneConfig {
	cpCount := cfg.ControlPlaneCount()

	return ControlPlaneConfig{
		NodePools: []ControlPlaneNodePool{
			{
				Name:       "control-plane",
				Location:   string(cfg.Region),
				ServerType: string(cfg.ControlPlaneSize()), // Use configured or default size
				Count:      cpCount,
				Labels: map[string]string{
					"node.kubernetes.io/role": "control-plane",
				},
			},
		},
	}
}

func expandWorkers(cfg *Spec) []WorkerNodePool {
	return []WorkerNodePool{
		{
			Name:           "workers",
			Location:       string(cfg.Region),
			ServerType:     string(cfg.Workers.Size.Normalize()), // Convert old cx22 to cx23 etc.
			Count:          cfg.Workers.Count,
			PlacementGroup: true, // Spread workers across different physical hosts
			Labels: map[string]string{
				"node.kubernetes.io/role": "worker",
			},
		},
	}
}

func expandIngress(cfg *Spec) IngressConfig {
	// Ingress LB is not pre-provisioned; Traefik's LoadBalancer Service
	// creates a Hetzner LB automatically via CCM annotations.
	return IngressConfig{
		Enabled: false,
	}
}

func expandTalos(cfg *Spec, vm VersionMatrix) TalosConfig {
	return TalosConfig{
		Version: vm.Talos,
		Machine: TalosMachineConfig{
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

func expandKubernetes(cfg *Spec, vm VersionMatrix) KubernetesConfig {
	return KubernetesConfig{
		Version: vm.Kubernetes,
		Domain:  "cluster.local",

		// Enable API load balancer for HA mode
		APILoadBalancerEnabled:       cfg.Mode == ModeHA,
		APILoadBalancerPublicNetwork: boolPtr(true),

		// Allow scheduling on control plane only in dev mode
		AllowSchedulingOnCP: boolPtr(cfg.Mode == ModeDev),
	}
}

func expandAddons(cfg *Spec, vm VersionMatrix) AddonsConfig {
	hasDomain := cfg.HasDomain()

	return AddonsConfig{
		// Hetzner Cloud Controller Manager - always enabled
		CCM: DefaultCCM(),

		// Hetzner CSI Driver - always enabled
		CSI: DefaultCSI(),

		// Cilium CNI - always enabled with kube-proxy replacement
		Cilium: DefaultCilium(),

		// Traefik ingress - always enabled (replaces ingress-nginx)
		Traefik: DefaultTraefik(true),

		// Ingress-nginx - disabled (we use Traefik)
		IngressNginx: IngressNginxConfig{
			Enabled: false,
		},

		// cert-manager - always enabled
		CertManager: CertManagerConfig{
			Enabled: true,
			Cloudflare: CertManagerCloudflareConfig{
				Enabled:    hasDomain,
				Email:      cfg.GetCertEmail(),
				Production: true, // Use production Let's Encrypt
			},
		},

		// Metrics server - always enabled
		MetricsServer: MetricsServerConfig{
			Enabled: true,
		},

		// ArgoCD - always enabled, with ingress when domain is set
		ArgoCD: expandArgoCD(cfg),

		// Gateway API CRDs - always enabled
		GatewayAPICRDs: DefaultGatewayAPICRDs(),

		// Prometheus Operator CRDs - always enabled
		PrometheusOperatorCRDs: DefaultPrometheusOperatorCRDs(),

		// Kube Prometheus Stack - enabled only when monitoring is set
		KubePrometheusStack: expandKubePrometheusStack(cfg),

		// Talos CCM - always enabled
		TalosCCM: TalosCCMConfig{
			Enabled: true,
			Version: vm.TalosCCM,
		},

		// Cloudflare - enabled only when domain is set
		// API token is read from CF_API_TOKEN environment variable
		Cloudflare: CloudflareConfig{
			Enabled:  hasDomain,
			Domain:   cfg.Domain,
			APIToken: os.Getenv("CF_API_TOKEN"),
		},

		// External DNS - enabled only when domain is set
		ExternalDNS: ExternalDNSConfig{
			Enabled:    hasDomain,
			TXTOwnerID: cfg.Name,
			Policy:     "sync",
			Sources:    []string{"ingress"},
		},

		// Talos Backup - enabled only when backup is set
		TalosBackup: expandTalosBackup(cfg),
	}
}

func expandTalosBackup(cfg *Spec) TalosBackupConfig {
	if !cfg.HasBackup() {
		return TalosBackupConfig{Enabled: false}
	}

	return TalosBackupConfig{
		Enabled:            true,
		Schedule:           "0 * * * *", // Hourly
		S3Bucket:           cfg.BackupBucketName(),
		S3Region:           string(cfg.Region),
		S3Endpoint:         cfg.S3Endpoint(),
		S3AccessKey:        os.Getenv("HETZNER_S3_ACCESS_KEY"),
		S3SecretKey:        os.Getenv("HETZNER_S3_SECRET_KEY"),
		S3Prefix:           "etcd-backups",
		EnableCompression:  true,
		EncryptionDisabled: true, // Spec config: encryption disabled by default (private bucket provides security)
	}
}

func expandArgoCD(cfg *Spec) ArgoCDConfig {
	argoCfg := ArgoCDConfig{
		Enabled: true,
		HA:      cfg.Mode == ModeHA,
	}

	// Enable ingress with TLS when domain is configured
	if cfg.HasDomain() {
		argoCfg.IngressEnabled = true
		argoCfg.IngressHost = cfg.ArgoHost()
		argoCfg.IngressClassName = "traefik"
		argoCfg.IngressTLS = true
	}

	return argoCfg
}

func expandKubePrometheusStack(cfg *Spec) KubePrometheusStackConfig {
	if !cfg.HasMonitoring() {
		return KubePrometheusStackConfig{Enabled: false}
	}

	promCfg := KubePrometheusStackConfig{
		Enabled: true,
		Grafana: KubePrometheusGrafanaConfig{},
		Prometheus: KubePrometheusPrometheusConfig{
			Persistence: KubePrometheusPersistenceConfig{
				Enabled: true,
				Size:    "50Gi",
			},
		},
		Alertmanager: KubePrometheusAlertmanagerConfig{},
	}

	// Enable Grafana ingress with TLS when domain is configured
	if cfg.HasDomain() {
		promCfg.Grafana.IngressEnabled = true
		promCfg.Grafana.IngressHost = cfg.GrafanaHost()
		promCfg.Grafana.IngressClassName = "traefik"
		promCfg.Grafana.IngressTLS = true
	}

	return promCfg
}
