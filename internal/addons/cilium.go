package addons

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyCilium installs the Cilium CNI network plugin.
// See: terraform/cilium.tf
func applyCilium(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	if !cfg.Addons.Cilium.Enabled {
		return nil
	}

	// Pre-create cilium-secrets namespace to avoid race condition
	// The Cilium Helm chart creates this namespace and resources that reference it,
	// but kubectl apply can fail if the resources are created before the namespace is fully ready.
	//nolint:gosec // G101 false positive - "secrets" is a namespace name, not credentials
	ciliumSecretsNS := `apiVersion: v1
kind: Namespace
metadata:
  name: cilium-secrets
`
	if err := applyManifests(ctx, client, "cilium-secrets-namespace", []byte(ciliumSecretsNS)); err != nil {
		return fmt.Errorf("failed to create cilium-secrets namespace: %w", err)
	}

	// Generate and apply IPSec secret if IPSec encryption is enabled
	if cfg.Addons.Cilium.EncryptionEnabled && cfg.Addons.Cilium.EncryptionType == "ipsec" {
		secretManifest, err := buildCiliumIPSecSecret(cfg)
		if err != nil {
			return fmt.Errorf("failed to generate IPSec secret: %w", err)
		}

		if err := applyManifests(ctx, client, "cilium-ipsec-keys", []byte(secretManifest)); err != nil {
			return fmt.Errorf("failed to apply Cilium IPSec secret: %w", err)
		}
	}

	// Build Cilium helm values
	values := buildCiliumValues(cfg)

	// Get chart spec with any config overrides
	spec := helm.GetChartSpec("cilium", cfg.Addons.Cilium.Helm)

	// Render helm chart
	manifestBytes, err := helm.RenderFromSpec(ctx, spec, "kube-system", values)
	if err != nil {
		return fmt.Errorf("failed to render Cilium chart: %w", err)
	}

	// Apply manifests
	if err := applyManifests(ctx, client, "cilium", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply Cilium manifests: %w", err)
	}

	return nil
}

// buildCiliumValues creates helm values matching terraform configuration.
// See: terraform/cilium.tf lines 45-207
func buildCiliumValues(cfg *config.Config) helm.Values {
	controlPlaneCount := getControlPlaneCount(cfg)
	ciliumCfg := cfg.Addons.Cilium

	// Native routing CIDR (use network CIDR or explicit native routing CIDR)
	nativeRoutingCIDR := cfg.Network.IPv4CIDR
	if cfg.Network.NativeRoutingIPv4CIDR != "" {
		nativeRoutingCIDR = cfg.Network.NativeRoutingIPv4CIDR
	}

	// BPF datapath mode - default to "veth"
	bpfDatapathMode := ciliumCfg.BPFDatapathMode
	if bpfDatapathMode == "" {
		bpfDatapathMode = "veth"
	}

	// Policy CIDR match mode - can be empty or "nodes"
	var policyCIDRMatchMode any
	if ciliumCfg.PolicyCIDRMatchMode == "nodes" {
		policyCIDRMatchMode = []string{"nodes"}
	} else {
		policyCIDRMatchMode = ""
	}

	values := helm.Values{
		"ipam": helm.Values{
			"mode": "kubernetes",
		},
		"routingMode":           ciliumCfg.RoutingMode,
		"ipv4NativeRoutingCIDR": nativeRoutingCIDR,
		"policyCIDRMatchMode":   policyCIDRMatchMode,
		"bpf": helm.Values{
			"masquerade":        ciliumCfg.KubeProxyReplacementEnabled,
			"datapathMode":      bpfDatapathMode,
			"hostLegacyRouting": ciliumCfg.EncryptionEnabled && ciliumCfg.EncryptionType == "ipsec",
		},
		"encryption": helm.Values{
			"enabled": ciliumCfg.EncryptionEnabled,
			"type":    ciliumCfg.EncryptionType,
		},
		"k8s": helm.Values{
			"requireIPv4PodCIDR": true,
		},
		"k8sServiceHost":                  "127.0.0.1",
		"k8sServicePort":                  7445,
		"kubeProxyReplacement":            ciliumCfg.KubeProxyReplacementEnabled,
		"installNoConntrackIptablesRules": ciliumCfg.KubeProxyReplacementEnabled && ciliumCfg.RoutingMode == "native",
		"socketLB": helm.Values{
			"hostNamespaceOnly": ciliumCfg.SocketLBHostNamespaceOnly,
		},
		"cgroup": helm.Values{
			"autoMount": helm.Values{"enabled": false},
			"hostRoot":  "/sys/fs/cgroup",
		},
		"securityContext": helm.Values{
			"capabilities": helm.Values{
				"ciliumAgent":      []string{"CHOWN", "KILL", "NET_ADMIN", "NET_RAW", "IPC_LOCK", "SYS_ADMIN", "SYS_RESOURCE", "DAC_OVERRIDE", "FOWNER", "SETGID", "SETUID"},
				"cleanCiliumState": []string{"NET_ADMIN", "SYS_ADMIN", "SYS_RESOURCE"},
			},
		},
		"dnsProxy": helm.Values{
			"enableTransparentMode": true,
		},
		"egressGateway": helm.Values{
			"enabled": ciliumCfg.EgressGatewayEnabled,
		},
		"loadBalancer": helm.Values{
			"acceleration": "native",
		},
	}

	// KubeProxy replacement specific settings
	if ciliumCfg.KubeProxyReplacementEnabled {
		values["kubeProxyReplacementHealthzBindAddr"] = "0.0.0.0:10256"
	}

	// Gateway API configuration
	if ciliumCfg.GatewayAPIEnabled {
		values["gatewayAPI"] = buildCiliumGatewayAPIConfig(ciliumCfg)
	}

	// Hubble configuration
	if ciliumCfg.HubbleEnabled {
		values["hubble"] = buildCiliumHubbleConfig(cfg)
	}

	// Prometheus configuration
	values["prometheus"] = buildCiliumPrometheusConfig(ciliumCfg)

	// Operator configuration
	values["operator"] = buildCiliumOperatorConfig(ciliumCfg, controlPlaneCount)

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, ciliumCfg.Helm.Values)
}

// buildCiliumGatewayAPIConfig creates Gateway API configuration.
// See: terraform/cilium.tf lines 101-110
func buildCiliumGatewayAPIConfig(ciliumCfg config.CiliumConfig) helm.Values {
	// Gateway API proxy protocol - default to true
	proxyProtocolEnabled := true
	if ciliumCfg.GatewayAPIProxyProtocolEnabled != nil {
		proxyProtocolEnabled = *ciliumCfg.GatewayAPIProxyProtocolEnabled
	}

	// External traffic policy - default to "Cluster"
	externalTrafficPolicy := ciliumCfg.GatewayAPIExternalTrafficPolicy
	if externalTrafficPolicy == "" {
		externalTrafficPolicy = "Cluster"
	}

	return helm.Values{
		"enabled":               true,
		"enableProxyProtocol":   proxyProtocolEnabled,
		"enableAppProtocol":     true,
		"enableAlpn":            true,
		"externalTrafficPolicy": externalTrafficPolicy,
		"gatewayClass": helm.Values{
			"create": "true",
		},
	}
}

// buildCiliumPrometheusConfig creates Prometheus integration configuration.
// See: terraform/cilium.tf lines 119-126
func buildCiliumPrometheusConfig(ciliumCfg config.CiliumConfig) helm.Values {
	prometheusConfig := helm.Values{
		"enabled": true,
	}

	prometheusConfig["serviceMonitor"] = helm.Values{
		"enabled":        ciliumCfg.ServiceMonitorEnabled,
		"trustCRDsExist": ciliumCfg.ServiceMonitorEnabled,
		"interval":       "15s",
	}

	return prometheusConfig
}

// buildCiliumOperatorConfig creates operator configuration.
// See: terraform/cilium.tf lines 139-177
func buildCiliumOperatorConfig(_ config.CiliumConfig, controlPlaneCount int) helm.Values {
	replicas := 1
	if controlPlaneCount > 1 {
		replicas = 2
	}

	operatorConfig := helm.Values{
		"replicas": replicas,
		"nodeSelector": helm.Values{
			"node-role.kubernetes.io/control-plane": "",
		},
		"tolerations": []helm.Values{
			{
				"key":      "node-role.kubernetes.io/control-plane",
				"effect":   "NoSchedule",
				"operator": "Exists",
			},
			{
				"key":      "node-role.kubernetes.io/master",
				"effect":   "NoSchedule",
				"operator": "Exists",
			},
			{
				"key":      "node.kubernetes.io/not-ready",
				"operator": "Exists",
			},
			{
				"key":      "node.cloudprovider.kubernetes.io/uninitialized",
				"operator": "Exists",
			},
		},
	}

	// Add PDB for HA setups
	if controlPlaneCount > 1 {
		operatorConfig["podDisruptionBudget"] = helm.Values{
			"enabled":        true,
			"maxUnavailable": 1,
		}

		operatorConfig["topologySpreadConstraints"] = []helm.Values{
			{
				"topologyKey":       "kubernetes.io/hostname",
				"maxSkew":           1,
				"whenUnsatisfiable": "DoNotSchedule",
				"labelSelector": helm.Values{
					"matchLabels": helm.Values{
						"app.kubernetes.io/part-of": "cilium",
						"app.kubernetes.io/name":    "cilium-operator",
					},
				},
				"matchLabelKeys": []string{"pod-template-hash"},
			},
			{
				"topologyKey":       "topology.kubernetes.io/zone",
				"maxSkew":           1,
				"whenUnsatisfiable": "ScheduleAnyway",
				"labelSelector": helm.Values{
					"matchLabels": helm.Values{
						"app.kubernetes.io/part-of": "cilium",
						"app.kubernetes.io/name":    "cilium-operator",
					},
				},
				"matchLabelKeys": []string{"pod-template-hash"},
			},
		}
	}

	return operatorConfig
}

// buildCiliumHubbleConfig creates Hubble observability configuration.
// See: terraform/cilium.tf lines 179-207
func buildCiliumHubbleConfig(cfg *config.Config) helm.Values {
	hubbleConfig := helm.Values{
		"enabled": true,
	}

	if cfg.Addons.Cilium.HubbleRelayEnabled {
		hubbleConfig["relay"] = helm.Values{
			"enabled": true,
			"tolerations": []helm.Values{
				{
					"key":      "node-role.kubernetes.io/control-plane",
					"effect":   "NoSchedule",
					"operator": "Exists",
				},
				{
					"key":      "node.cloudprovider.kubernetes.io/uninitialized",
					"operator": "Exists",
				},
			},
		}
	}

	if cfg.Addons.Cilium.HubbleUIEnabled {
		hubbleConfig["ui"] = helm.Values{
			"enabled": true,
		}
	}

	return hubbleConfig
}

// buildCiliumIPSecSecret generates the IPSec keys secret.
// See: terraform/cilium.tf lines 11-31
func buildCiliumIPSecSecret(cfg *config.Config) (string, error) {
	keySize := cfg.Addons.Cilium.IPSecKeySize
	if keySize == 0 {
		keySize = 128
	}

	keyID := cfg.Addons.Cilium.IPSecKeyID
	if keyID == 0 {
		keyID = 1
	}

	algorithm := cfg.Addons.Cilium.IPSecAlgorithm
	if algorithm == "" {
		algorithm = "rfc4106(gcm(aes))"
	}

	// Generate random key
	key, err := generateIPSecKey(keySize)
	if err != nil {
		return "", fmt.Errorf("failed to generate IPSec key: %w", err)
	}

	// Format: {keyID}+ {algorithm} {hexKey} 128
	keyFormat := fmt.Sprintf("%d+ %s %s 128", keyID, algorithm, key)
	base64Key := base64.StdEncoding.EncodeToString([]byte(keyFormat))

	secret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"type":       "Opaque",
		"metadata": map[string]any{
			"name":      "cilium-ipsec-keys",
			"namespace": "kube-system",
			"annotations": map[string]any{
				"cilium.io/key-id":        fmt.Sprintf("%d", keyID),
				"cilium.io/key-algorithm": algorithm,
				"cilium.io/key-size":      fmt.Sprintf("%d", keySize),
			},
		},
		"data": map[string]any{
			"keys": base64Key,
		},
	}

	yamlBytes, err := yaml.Marshal(secret)
	if err != nil {
		return "", fmt.Errorf("failed to marshal IPSec secret: %w", err)
	}

	return string(yamlBytes), nil
}

// generateIPSecKey generates a random IPSec key.
// See: terraform/cilium.tf lines 34-43
func generateIPSecKey(keySize int) (string, error) {
	// Key size in bytes + 4 bytes salt
	length := (keySize / 8) + 4

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	return hex.EncodeToString(bytes), nil
}
