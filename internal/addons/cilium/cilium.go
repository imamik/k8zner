// Package cilium provides the Cilium CNI addon.
package cilium

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/addons"
	"github.com/sak-d/hcloud-k8s/internal/config"
)

const (
	addonName        = "cilium"
	namespace        = "kube-system"
	ipsecSecretName  = "cilium-ipsec-keys"
	operatorName     = "cilium-operator"
	daemonSetName    = "cilium"

	// Default Helm configuration
	defaultHelmRepository = "https://helm.cilium.io"
	defaultHelmChart      = "cilium"
	defaultHelmVersion    = "1.16.5"

	// KubePrism endpoint (Talos-specific)
	kubePrismHost = "127.0.0.1"
	kubePrismPort = "7445"
)

// Cilium implements the Cilium CNI addon.
type Cilium struct {
	config      *config.Config
	addonConfig *config.CiliumConfig
}

// New creates a new Cilium addon instance.
func New(cfg *config.Config, addonCfg *config.CiliumConfig) *Cilium {
	return &Cilium{
		config:      cfg,
		addonConfig: addonCfg,
	}
}

// Name returns the addon name.
func (c *Cilium) Name() string {
	return addonName
}

// Enabled returns whether the addon is enabled.
func (c *Cilium) Enabled() bool {
	return c.addonConfig.Enabled
}

// Dependencies returns addon dependencies.
// Cilium can be installed in parallel with CCM (no dependencies).
func (c *Cilium) Dependencies() []string {
	return []string{}
}

// GenerateManifests generates Kubernetes manifests for Cilium.
func (c *Cilium) GenerateManifests(ctx context.Context) ([]string, error) {
	manifests := []string{}

	// 1. Generate IPSec secret if encryption is enabled
	if c.addonConfig.EncryptionEnabled && c.addonConfig.EncryptionType == "ipsec" {
		ipsecManifest, err := c.generateIPSecSecret()
		if err != nil {
			return nil, fmt.Errorf("failed to generate ipsec secret: %w", err)
		}
		manifests = append(manifests, ipsecManifest)
		log.Printf("Generated IPSec secret for Cilium")
	}

	// 2. Generate Helm chart manifests
	helmManifest, err := c.generateHelmManifests(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate helm manifests: %w", err)
	}
	manifests = append(manifests, helmManifest)

	return manifests, nil
}

// generateIPSecSecret generates the Cilium IPSec keys secret.
func (c *Cilium) generateIPSecSecret() (string, error) {
	keyID := c.addonConfig.IPSecKeyID
	if keyID == 0 {
		keyID = 3 // Default key ID
	}

	algorithm := c.addonConfig.IPSecAlgorithm
	if algorithm == "" {
		algorithm = "gcm-aes-128" // Default algorithm
	}

	keySize := c.addonConfig.IPSecKeySize
	if keySize == 0 {
		keySize = 128 // Default key size in bits
	}

	// Generate key: key size in bytes + 4 bytes for salt
	keySizeBytes := (keySize / 8) + 4
	key := make([]byte, keySizeBytes)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}

	hexKey := hex.EncodeToString(key)

	// Format: "{keyID}+ {algorithm} {hexKey} 128"
	// The format is specified by Cilium's IPSec implementation
	keyFormat := fmt.Sprintf("%d+ %s %s 128", keyID, algorithm, hexKey)
	keyBase64 := base64.StdEncoding.EncodeToString([]byte(keyFormat))

	secret := fmt.Sprintf(`apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: %s
  namespace: %s
  annotations:
    cilium.io/key-id: "%d"
    cilium.io/key-algorithm: "%s"
    cilium.io/key-size: "%d"
data:
  keys: %s
`, ipsecSecretName, namespace, keyID, algorithm, keySize, keyBase64)

	return secret, nil
}

// generateHelmManifests generates the Helm chart manifests for Cilium.
func (c *Cilium) generateHelmManifests(ctx context.Context) (string, error) {
	// Prepare Helm values
	values := c.buildHelmValues()

	// Get Helm configuration
	helmRepo := c.addonConfig.HelmRepository
	if helmRepo == "" {
		helmRepo = defaultHelmRepository
	}

	helmChart := c.addonConfig.HelmChart
	if helmChart == "" {
		helmChart = defaultHelmChart
	}

	helmVersion := c.addonConfig.HelmVersion
	if helmVersion == "" {
		helmVersion = defaultHelmVersion
	}

	// Render Helm chart
	renderer := addons.NewHelmRenderer()
	manifest, err := renderer.RenderChart(helmRepo, helmChart, helmVersion, namespace, values)
	if err != nil {
		return "", fmt.Errorf("failed to render helm chart: %w", err)
	}

	return manifest, nil
}

// buildHelmValues constructs the Helm values for Cilium.
func (c *Cilium) buildHelmValues() map[string]interface{} {
	// Routing mode
	routingMode := c.addonConfig.RoutingMode
	if routingMode == "" {
		routingMode = "native" // Default to native routing
	}

	// BPF datapath mode
	bpfDatapathMode := c.addonConfig.BPFDatapathMode
	if bpfDatapathMode == "" {
		bpfDatapathMode = "veth" // Default
	}

	// Encryption type
	encryptionType := c.addonConfig.EncryptionType
	if encryptionType == "" && c.addonConfig.EncryptionEnabled {
		encryptionType = "ipsec" // Default if encryption enabled
	}

	// Calculate operator replicas based on control plane count
	operatorReplicas := 1
	if c.config.ControlPlane.NodePools != nil && len(c.config.ControlPlane.NodePools) > 0 {
		totalCPNodes := 0
		for _, pool := range c.config.ControlPlane.NodePools {
			totalCPNodes += pool.Count
		}
		if totalCPNodes >= 2 {
			operatorReplicas = 2
		}
	}

	values := map[string]interface{}{
		"ipam": map[string]interface{}{
			"mode": "kubernetes",
		},
		"routingMode":           routingMode,
		"ipv4NativeRoutingCIDR": c.config.Network.NodeIPv4CIDR,
		"k8s": map[string]interface{}{
			"requireIPv4PodCIDR": true,
		},
		"k8sServiceHost":        kubePrismHost,
		"k8sServicePort":        kubePrismPort,
		"kubeProxyReplacement":  c.addonConfig.KubeProxyReplacement,
		"bpf": map[string]interface{}{
			"masquerade":        c.addonConfig.KubeProxyReplacement,
			"datapathMode":      bpfDatapathMode,
			"hostLegacyRouting": c.addonConfig.EncryptionType == "ipsec",
		},
		"operator": map[string]interface{}{
			"replicas": operatorReplicas,
		},
	}

	// Add kube-proxy replacement specific settings
	if c.addonConfig.KubeProxyReplacement {
		values["kubeProxyReplacementHealthzBindAddr"] = "0.0.0.0:10256"
		values["installNoConntrackIptablesRules"] = routingMode == "native"
		values["socketLB"] = map[string]interface{}{
			"hostNamespaceOnly": true,
		}
	}

	// Add encryption configuration
	if c.addonConfig.EncryptionEnabled {
		values["encryption"] = map[string]interface{}{
			"enabled": true,
			"type":    encryptionType,
		}
	}

	// Add Hubble configuration if enabled
	if c.addonConfig.HubbleEnabled {
		values["hubble"] = map[string]interface{}{
			"enabled": true,
			"relay": map[string]interface{}{
				"enabled": true,
			},
			"ui": map[string]interface{}{
				"enabled": true,
			},
		}
	}

	// Add Gateway API configuration if enabled
	if c.addonConfig.GatewayAPIEnabled {
		values["gatewayAPI"] = map[string]interface{}{
			"enabled": true,
		}
	}

	// Add policy CIDR match mode if configured
	if len(c.addonConfig.PolicyCIDRMatchMode) > 0 {
		values["policyC IDRMatchMode"] = c.addonConfig.PolicyCIDRMatchMode
	}

	// Merge custom Helm values if provided
	if len(c.addonConfig.HelmValues) > 0 {
		for k, v := range c.addonConfig.HelmValues {
			values[k] = v
		}
	}

	return values
}

// Verify checks if Cilium is installed and running correctly.
func (c *Cilium) Verify(ctx context.Context, k8sClient addons.K8sClient) error {
	log.Printf("Verifying %s installation...", addonName)

	// If IPSec is enabled, check if secret exists
	if c.addonConfig.EncryptionEnabled && c.addonConfig.EncryptionType == "ipsec" {
		secretExists, err := k8sClient.SecretExists(ctx, namespace, ipsecSecretName)
		if err != nil {
			return fmt.Errorf("failed to check ipsec secret existence: %w", err)
		}
		if !secretExists {
			return fmt.Errorf("ipsec secret %s not found in namespace %s", ipsecSecretName, namespace)
		}
	}

	// Wait for Cilium DaemonSet to be ready
	log.Printf("Waiting for %s DaemonSet to be ready...", addonName)
	if err := k8sClient.WaitForDaemonSet(ctx, namespace, daemonSetName, 5*time.Minute); err != nil {
		return fmt.Errorf("daemonset not ready: %w", err)
	}

	// Wait for Cilium Operator Deployment to be ready
	log.Printf("Waiting for %s operator to be ready...", addonName)
	if err := k8sClient.WaitForDeployment(ctx, namespace, operatorName, 5*time.Minute); err != nil {
		return fmt.Errorf("operator deployment not ready: %w", err)
	}

	// Verify Cilium agent pods
	agentPods, err := k8sClient.GetPods(ctx, namespace, fmt.Sprintf("k8s-app=%s", addonName))
	if err != nil {
		return fmt.Errorf("failed to get agent pods: %w", err)
	}

	// Verify operator pods
	operatorPods, err := k8sClient.GetPods(ctx, namespace, fmt.Sprintf("io.cilium/app=%s", operatorName))
	if err != nil {
		return fmt.Errorf("failed to get operator pods: %w", err)
	}

	log.Printf("%s verified successfully (agent: %d pods, operator: %d pods)",
		addonName, len(agentPods), len(operatorPods))
	return nil
}
