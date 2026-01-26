package wizard

import "github.com/imamik/k8zner/internal/config"

// BuildConfig creates a Config struct from the wizard result.
func BuildConfig(result *WizardResult) *config.Config {
	cfg := &config.Config{
		ClusterName: result.ClusterName,
		Location:    result.Location,
		Talos: config.TalosConfig{
			Version: result.TalosVersion,
		},
		Kubernetes: config.KubernetesConfig{
			Version: result.KubernetesVersion,
		},
	}

	// Only set SSH keys if provided (optional field)
	if len(result.SSHKeys) > 0 {
		cfg.SSHKeys = result.SSHKeys
	}

	cfg.ControlPlane = config.ControlPlaneConfig{
		NodePools: []config.ControlPlaneNodePool{
			{
				Name:       "control-plane",
				ServerType: result.ControlPlaneType,
				Count:      result.ControlPlaneCount,
			},
		},
	}

	if result.AddWorkers {
		cfg.Workers = []config.WorkerNodePool{
			{
				Name:       "workers",
				ServerType: result.WorkerType,
				Count:      result.WorkerCount,
			},
		}
	} else {
		// Enable scheduling on control plane when no workers are configured
		cfg.Kubernetes.AllowSchedulingOnCP = boolPtr(true)
	}

	cfg.Addons = buildAddonsConfig(result.EnabledAddons, result.CNIChoice, result.IngressController)

	if result.AdvancedOptions != nil {
		applyAdvancedOptions(cfg, result.AdvancedOptions)
	}

	return cfg
}

// buildAddonsConfig creates the AddonsConfig from enabled addon keys, CNI choice, and ingress controller choice.
func buildAddonsConfig(enabledAddons []string, cniChoice, ingressController string) config.AddonsConfig {
	addons := config.AddonsConfig{}

	// Handle CNI selection
	switch cniChoice {
	case CNICilium:
		addons.Cilium.Enabled = true
	case CNITalosNative:
		// Talos native CNI (Flannel) - Cilium stays disabled
		addons.Cilium.Enabled = false
	case CNINone:
		// User will install their own CNI
		addons.Cilium.Enabled = false
	}

	// Handle ingress controller selection
	switch ingressController {
	case IngressNginx:
		addons.IngressNginx.Enabled = true
	case IngressTraefik:
		addons.Traefik.Enabled = true
	case IngressNone:
		// User will install their own ingress controller
	}

	// Handle other addons
	for _, addon := range enabledAddons {
		switch addon {
		case "ccm":
			addons.CCM.Enabled = true
		case "csi":
			addons.CSI.Enabled = true
		case "metrics_server":
			addons.MetricsServer.Enabled = true
		case "cert_manager":
			addons.CertManager.Enabled = true
		case "longhorn":
			addons.Longhorn.Enabled = true
		case "argocd":
			addons.ArgoCD.Enabled = true
		}
	}

	return addons
}

// applyAdvancedOptions applies advanced options to the config.
func applyAdvancedOptions(cfg *config.Config, opts *AdvancedOptions) {
	// Network configuration
	if opts.NetworkCIDR != "" {
		cfg.Network.IPv4CIDR = opts.NetworkCIDR
	}
	if opts.PodCIDR != "" {
		cfg.Network.PodIPv4CIDR = opts.PodCIDR
	}
	if opts.ServiceCIDR != "" {
		cfg.Network.ServiceIPv4CIDR = opts.ServiceCIDR
	}

	// Security options
	cfg.Talos.Machine.StateEncryption = boolPtr(opts.DiskEncryption)
	cfg.Talos.Machine.EphemeralEncryption = boolPtr(opts.DiskEncryption)
	cfg.ClusterAccess = opts.ClusterAccess

	// Cilium options
	if cfg.Addons.Cilium.Enabled {
		if opts.CiliumEncryption {
			cfg.Addons.Cilium.EncryptionEnabled = true
			cfg.Addons.Cilium.EncryptionType = opts.CiliumEncryptionType
		}
		cfg.Addons.Cilium.HubbleEnabled = opts.HubbleEnabled
		cfg.Addons.Cilium.HubbleRelayEnabled = opts.HubbleEnabled
		cfg.Addons.Cilium.GatewayAPIEnabled = opts.GatewayAPIEnabled
	}
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}
