package addons

import (
	"context"
	"fmt"
	"os"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/crypto/ipsec"
)

const (
	ciliumReleaseName    = "cilium"
	ciliumNamespace      = "kube-system"
	ciliumRepoURL        = "https://helm.cilium.io"
	ciliumChartName      = "cilium"
	ciliumDefaultVersion = "1.18.5"

	// KubePrism endpoint for Talos
	kubePrismHost = "127.0.0.1"
	kubePrismPort = 7445
)

// applyCilium installs or upgrades Cilium CNI.
func applyCilium(ctx context.Context, cfg *config.Config, kubeconfig []byte, controlPlaneCount int) error {
	// Create IPSec secret first if needed
	if cfg.Kubernetes.CNI.Encryption.Enabled && cfg.Kubernetes.CNI.Encryption.Type == "ipsec" {
		if err := createIPSecSecret(ctx, cfg, kubeconfig); err != nil {
			return fmt.Errorf("failed to create IPSec secret: %w", err)
		}
	}

	// Create Helm client
	helmClient, err := helm.NewClient(kubeconfig, ciliumNamespace)
	if err != nil {
		return fmt.Errorf("failed to create helm client: %w", err)
	}

	// Build Helm values
	values := buildCiliumValues(cfg, controlPlaneCount)

	// Get chart version
	version := cfg.Kubernetes.CNI.HelmVersion
	if version == "" {
		version = ciliumDefaultVersion
	}

	// Install or upgrade
	_, err = helmClient.InstallOrUpgrade(ctx, ciliumReleaseName, ciliumRepoURL, ciliumChartName, version, values)
	if err != nil {
		return fmt.Errorf("failed to install/upgrade cilium: %w", err)
	}

	return nil
}

func buildCiliumValues(cfg *config.Config, controlPlaneCount int) map[string]any {
	cniCfg := cfg.Kubernetes.CNI
	encCfg := cniCfg.Encryption

	// Calculate native routing CIDR
	nativeRoutingCIDR := cfg.Network.NativeRoutingIPv4CIDR
	if nativeRoutingCIDR == "" {
		nativeRoutingCIDR = cfg.Network.IPv4CIDR
	}

	// Determine if host legacy routing is needed (required for IPSec)
	hostLegacyRouting := encCfg.Enabled && encCfg.Type == "ipsec"

	// Calculate operator replicas based on control plane count
	operatorReplicas := 1
	if controlPlaneCount > 1 {
		operatorReplicas = 2
	}

	// Determine routing mode
	routingMode := cniCfg.RoutingMode
	if routingMode == "" {
		routingMode = "native"
	}

	// Determine BPF datapath mode
	bpfDatapathMode := cniCfg.BPFDatapathMode
	if bpfDatapathMode == "" {
		bpfDatapathMode = "veth"
	}

	// Build core values
	values := map[string]any{
		"ipam": map[string]any{
			"mode": "kubernetes",
		},
		"routingMode":           routingMode,
		"ipv4NativeRoutingCIDR": nativeRoutingCIDR,
		"bpf": map[string]any{
			"masquerade":        cniCfg.KubeProxyReplacement,
			"datapathMode":      bpfDatapathMode,
			"hostLegacyRouting": hostLegacyRouting,
		},
		"encryption": map[string]any{
			"enabled": encCfg.Enabled,
			"type":    encCfg.Type,
		},
		"k8s": map[string]any{
			"requireIPv4PodCIDR": true,
		},
		"k8sServiceHost":       kubePrismHost,
		"k8sServicePort":       kubePrismPort,
		"kubeProxyReplacement": cniCfg.KubeProxyReplacement,
		"kubeProxyReplacementHealthzBindAddr": func() string {
			if cniCfg.KubeProxyReplacement {
				return "0.0.0.0:10256"
			}
			return ""
		}(),
		"installNoConntrackIptablesRules": cniCfg.KubeProxyReplacement && routingMode == "native",
		"socketLB": map[string]any{
			"hostNamespaceOnly": false,
		},
		// Talos-specific cgroup configuration
		"cgroup": map[string]any{
			"autoMount": map[string]any{"enabled": false},
			"hostRoot":  "/sys/fs/cgroup",
		},
		// Security context with required capabilities
		"securityContext": map[string]any{
			"capabilities": map[string]any{
				"ciliumAgent": []string{
					"CHOWN", "KILL", "NET_ADMIN", "NET_RAW", "IPC_LOCK",
					"SYS_ADMIN", "SYS_RESOURCE", "DAC_OVERRIDE", "FOWNER",
					"SETGID", "SETUID",
				},
				"cleanCiliumState": []string{
					"NET_ADMIN", "SYS_ADMIN", "SYS_RESOURCE",
				},
			},
		},
		"dnsProxy": map[string]any{
			"enableTransparentMode": true,
		},
		"loadBalancer": map[string]any{
			"acceleration": "native",
		},
		// Hubble observability
		"hubble": map[string]any{
			"enabled": cniCfg.Hubble.Enabled,
			"relay":   map[string]any{"enabled": cniCfg.Hubble.RelayEnabled},
			"ui":      map[string]any{"enabled": cniCfg.Hubble.UIEnabled},
			"peerService": map[string]any{
				"clusterDomain": "cluster.local",
			},
		},
		// Gateway API
		"gatewayAPI": map[string]any{
			"enabled":           cniCfg.GatewayAPI.Enabled,
			"enableAppProtocol": true,
			"enableAlpn":        true,
			"externalTrafficPolicy": func() string {
				if cniCfg.GatewayAPI.ExternalTrafficPolicy != "" {
					return cniCfg.GatewayAPI.ExternalTrafficPolicy
				}
				return "Cluster"
			}(),
			"gatewayClass": map[string]any{
				"create": fmt.Sprintf("%t", cniCfg.GatewayAPI.Enabled),
			},
		},
		// Prometheus metrics
		"prometheus": map[string]any{
			"enabled": true,
			"serviceMonitor": map[string]any{
				"enabled":  false,
				"interval": "15s",
			},
		},
		// Operator configuration
		"operator": map[string]any{
			"nodeSelector": map[string]string{
				"node-role.kubernetes.io/control-plane": "",
			},
			"replicas": operatorReplicas,
			"podDisruptionBudget": map[string]any{
				"enabled":        true,
				"minAvailable":   nil,
				"maxUnavailable": 1,
			},
			"topologySpreadConstraints": []map[string]any{
				{
					"topologyKey":       "kubernetes.io/hostname",
					"maxSkew":           1,
					"whenUnsatisfiable": "DoNotSchedule",
					"labelSelector": map[string]any{
						"matchLabels": map[string]string{
							"app.kubernetes.io/name": "cilium-operator",
						},
					},
					"matchLabelKeys": []string{"pod-template-hash"},
				},
			},
			"prometheus": map[string]any{
				"enabled": true,
				"serviceMonitor": map[string]any{
					"enabled":  false,
					"interval": "15s",
				},
			},
		},
	}

	// Merge extra helm values if provided
	if cniCfg.ExtraHelmValues != nil {
		values = mergeMaps(values, cniCfg.ExtraHelmValues)
	}

	return values
}

func createIPSecSecret(ctx context.Context, cfg *config.Config, kubeconfig []byte) error {
	ipsecCfg := cfg.Kubernetes.CNI.Encryption.IPSec

	// Get or generate key
	keyHex := ipsecCfg.Key
	if keyHex == "" {
		keySize := ipsecCfg.KeySize
		if keySize == 0 {
			keySize = ipsec.DefaultKeySize
		}
		var err error
		keyHex, err = ipsec.GenerateKey(keySize)
		if err != nil {
			return fmt.Errorf("failed to generate IPSec key: %w", err)
		}
	}

	// Get algorithm
	algorithm := ipsecCfg.Algorithm
	if algorithm == "" {
		algorithm = ipsec.DefaultAlgorithm
	}

	// Get key ID
	keyID := ipsecCfg.KeyID
	if keyID == 0 {
		keyID = ipsec.DefaultKeyID
	}

	// Get key size
	keySize := ipsecCfg.KeySize
	if keySize == 0 {
		keySize = ipsec.DefaultKeySize
	}

	// Create secret manifest
	keyCfg := ipsec.KeyConfig{
		KeyID:     keyID,
		Algorithm: algorithm,
		KeySize:   keySize,
		KeyHex:    keyHex,
	}

	manifest, err := ipsec.CreateSecretManifest(keyCfg)
	if err != nil {
		return fmt.Errorf("failed to create IPSec secret manifest: %w", err)
	}

	// Write kubeconfig to temp file for kubectl
	tmpKubeconfig, err := writeTempKubeconfig(kubeconfig)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmpKubeconfig) }()

	// Apply the secret
	return applyWithKubectl(ctx, tmpKubeconfig, "cilium-ipsec-keys", manifest)
}

// mergeMaps recursively merges override into base
func mergeMaps(base, override map[string]any) map[string]any {
	result := make(map[string]any)

	// Copy base values
	for k, v := range base {
		result[k] = v
	}

	// Merge override values
	for k, v := range override {
		if baseMap, ok := base[k].(map[string]any); ok {
			if overrideMap, ok := v.(map[string]any); ok {
				result[k] = mergeMaps(baseMap, overrideMap)
				continue
			}
		}
		result[k] = v
	}

	return result
}
