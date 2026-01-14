package addons

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
)

// applyCilium installs the Cilium CNI network plugin.
// See: terraform/cilium.tf
func applyCilium(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
	if !cfg.Addons.Cilium.Enabled {
		return nil
	}

	// Generate and apply IPSec secret if IPSec encryption is enabled
	if cfg.Addons.Cilium.EncryptionEnabled && cfg.Addons.Cilium.EncryptionType == "ipsec" {
		secretManifest, err := buildCiliumIPSecSecret(cfg)
		if err != nil {
			return fmt.Errorf("failed to generate IPSec secret: %w", err)
		}

		if err := applyWithKubectl(ctx, kubeconfigPath, "cilium-ipsec-keys", []byte(secretManifest)); err != nil {
			return fmt.Errorf("failed to apply Cilium IPSec secret: %w", err)
		}
	}

	// Build Cilium helm values
	values := buildCiliumValues(cfg)

	// Render helm chart
	manifestBytes, err := helm.RenderChart("cilium", "kube-system", values)
	if err != nil {
		return fmt.Errorf("failed to render Cilium chart: %w", err)
	}

	// Apply manifests in two phases to avoid namespace race conditions
	// Phase 1: Apply namespaces first
	namespaces, rest := splitManifestsByKind(string(manifestBytes), "Namespace")
	if len(namespaces) > 0 {
		if err := applyWithKubectl(ctx, kubeconfigPath, "cilium-namespaces", []byte(namespaces)); err != nil {
			return fmt.Errorf("failed to apply Cilium namespaces: %w", err)
		}
		// Brief wait for namespace propagation
		time.Sleep(2 * time.Second)
	}

	// Phase 2: Apply remaining resources
	if err := applyWithKubectl(ctx, kubeconfigPath, "cilium", []byte(rest)); err != nil {
		return fmt.Errorf("failed to apply Cilium manifests: %w", err)
	}

	return nil
}

// buildCiliumValues creates helm values matching terraform configuration.
// See: terraform/cilium.tf lines 45-207
func buildCiliumValues(cfg *config.Config) helm.Values {
	controlPlaneCount := getControlPlaneCount(cfg)

	// Native routing CIDR (use network CIDR or explicit native routing CIDR)
	nativeRoutingCIDR := cfg.Network.IPv4CIDR
	if cfg.Network.NativeRoutingIPv4CIDR != "" {
		nativeRoutingCIDR = cfg.Network.NativeRoutingIPv4CIDR
	}

	values := helm.Values{
		"ipam": helm.Values{
			"mode": "kubernetes",
		},
		"routingMode":           cfg.Addons.Cilium.RoutingMode,
		"ipv4NativeRoutingCIDR": nativeRoutingCIDR,
		"policyCIDRMatchMode":   []string{"nodes"},
		"bpf": helm.Values{
			"masquerade":   cfg.Addons.Cilium.KubeProxyReplacementEnabled,
			"datapathMode": "veth",
			// hostLegacyRouting MUST be true on Talos 1.8+ due to DNS forwarding
			// See: https://github.com/siderolabs/talos/issues/9132
			"hostLegacyRouting": true,
		},
		"encryption": helm.Values{
			"enabled": cfg.Addons.Cilium.EncryptionEnabled,
			"type":    cfg.Addons.Cilium.EncryptionType,
		},
		"k8s": helm.Values{
			"requireIPv4PodCIDR": true,
		},
		"k8sServiceHost":                  "127.0.0.1",
		"k8sServicePort":                  7445,
		"kubeProxyReplacement":            cfg.Addons.Cilium.KubeProxyReplacementEnabled,
		"installNoConntrackIptablesRules": cfg.Addons.Cilium.KubeProxyReplacementEnabled && cfg.Addons.Cilium.RoutingMode == "native",
		"socketLB": helm.Values{
			"hostNamespaceOnly": false,
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
			"enabled": cfg.Addons.Cilium.EgressGatewayEnabled,
		},
		"loadBalancer": helm.Values{
			"acceleration": "native",
		},
		// Enable agent to tolerate its own taint during startup
		// See: https://github.com/cilium/cilium/issues/40312
		"agentNotReadyTaintKey": "node.cilium.io/agent-not-ready",
	}

	// KubeProxy replacement specific settings
	if cfg.Addons.Cilium.KubeProxyReplacementEnabled {
		values["kubeProxyReplacementHealthzBindAddr"] = "0.0.0.0:10256"
	}

	// Gateway API configuration
	if cfg.Addons.Cilium.GatewayAPIEnabled {
		values["gatewayAPI"] = helm.Values{
			"enabled": true,
		}
	}

	// Hubble configuration
	if cfg.Addons.Cilium.HubbleEnabled {
		values["hubble"] = buildCiliumHubbleConfig(cfg)
	}

	// Operator configuration
	values["operator"] = buildCiliumOperatorConfig(cfg, controlPlaneCount)

	return values
}

// buildCiliumOperatorConfig creates operator configuration.
// See: terraform/cilium.tf lines 139-177
func buildCiliumOperatorConfig(cfg *config.Config, controlPlaneCount int) helm.Values {
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

// splitManifestsByKind splits YAML manifests into two groups: matching kind and rest.
// Returns (matching, rest) where matching contains only manifests of the specified kind.
func splitManifestsByKind(manifests, kind string) (string, string) {
	docs := strings.Split(manifests, "\n---\n")
	var matching, rest []string

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		// Simple check: does this doc have "kind: <kind>" in it?
		if strings.Contains(doc, "kind: "+kind) {
			matching = append(matching, doc)
		} else {
			rest = append(rest, doc)
		}
	}

	matchingStr := strings.Join(matching, "\n---\n")
	restStr := strings.Join(rest, "\n---\n")

	return matchingStr, restStr
}
