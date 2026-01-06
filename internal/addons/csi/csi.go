// Package csi provides the Hetzner CSI Driver addon.
package csi

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
	addonName  = "hcloud-csi"
	namespace  = "kube-system"
	secretName = "hcloud-csi-secret"

	// Controller and node names for verification
	controllerName = "hcloud-csi-controller"
	nodeName       = "hcloud-csi-node"

	// Default Helm configuration
	defaultHelmRepository = "https://charts.hetzner.cloud"
	defaultHelmChart      = "hcloud-csi"
	defaultHelmVersion    = "2.12.0"
)

// CSI implements the Hetzner CSI Driver addon.
type CSI struct {
	config               *config.Config
	addonConfig          *config.CSIConfig
	hcloudToken          string
	encryptionPassphrase string
}

// New creates a new CSI addon instance.
func New(cfg *config.Config, addonCfg *config.CSIConfig, hcloudToken string) *CSI {
	return &CSI{
		config:               cfg,
		addonConfig:          addonCfg,
		hcloudToken:          hcloudToken,
		encryptionPassphrase: addonCfg.EncryptionPassphrase,
	}
}

// Name returns the addon name.
func (c *CSI) Name() string {
	return addonName
}

// Enabled returns whether the addon is enabled.
func (c *CSI) Enabled() bool {
	return c.addonConfig.Enabled
}

// Dependencies returns addon dependencies.
// CSI depends on CCM for network integration.
func (c *CSI) Dependencies() []string {
	return []string{"hcloud-ccm"}
}

// GenerateManifests generates Kubernetes manifests for CSI.
func (c *CSI) GenerateManifests(ctx context.Context) ([]string, error) {
	manifests := []string{}

	// Generate encryption key if not provided
	if c.encryptionPassphrase == "" {
		key, err := c.generateEncryptionKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate encryption key: %w", err)
		}
		c.encryptionPassphrase = key
		log.Printf("Generated new encryption passphrase for CSI")
	}

	// 1. Generate hcloud-csi-secret
	secretManifest, err := c.generateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	manifests = append(manifests, secretManifest)

	// 2. Generate Helm chart manifests
	helmManifest, err := c.generateHelmManifests(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate helm manifests: %w", err)
	}
	manifests = append(manifests, helmManifest)

	return manifests, nil
}

// generateEncryptionKey generates a random 32-byte encryption key.
func (c *CSI) generateEncryptionKey() (string, error) {
	key := make([]byte, 32) // 256 bits
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}

	return hex.EncodeToString(key), nil
}

// generateSecret generates the hcloud-csi-secret manifest.
func (c *CSI) generateSecret() (string, error) {
	passphraseBase64 := base64.StdEncoding.EncodeToString([]byte(c.encryptionPassphrase))

	secret := fmt.Sprintf(`apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: %s
  namespace: %s
data:
  encryption-passphrase: %s
`, secretName, namespace, passphraseBase64)

	return secret, nil
}

// generateHelmManifests generates the Helm chart manifests for CSI.
func (c *CSI) generateHelmManifests(ctx context.Context) (string, error) {
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

// buildHelmValues constructs the Helm values for CSI.
func (c *CSI) buildHelmValues() map[string]interface{} {
	// Calculate controller replicas based on control plane count
	controllerReplicas := 1
	if c.config.ControlPlane.NodePools != nil && len(c.config.ControlPlane.NodePools) > 0 {
		totalCPNodes := 0
		for _, pool := range c.config.ControlPlane.NodePools {
			totalCPNodes += pool.Count
		}
		if totalCPNodes >= 2 {
			controllerReplicas = 2
		}
	}

	values := map[string]interface{}{
		"controller": map[string]interface{}{
			"replicas": controllerReplicas,
		},
		"storageClasses": c.buildStorageClasses(),
	}

	// Merge custom Helm values if provided
	if len(c.addonConfig.HelmValues) > 0 {
		for k, v := range c.addonConfig.HelmValues {
			values[k] = v
		}
	}

	return values
}

// buildStorageClasses constructs storage class configurations.
func (c *CSI) buildStorageClasses() []map[string]interface{} {
	// Use configured storage classes or create default
	if len(c.addonConfig.StorageClasses) > 0 {
		classes := make([]map[string]interface{}, 0, len(c.addonConfig.StorageClasses))
		for _, sc := range c.addonConfig.StorageClasses {
			class := map[string]interface{}{
				"name":                sc.Name,
				"reclaimPolicy":       sc.ReclaimPolicy,
				"defaultStorageClass": sc.DefaultStorageClass,
			}

			// Add encryption parameters if encrypted
			if sc.Encrypted {
				class["extraParameters"] = map[string]interface{}{
					"csi.storage.k8s.io/node-publish-secret-name":      secretName,
					"csi.storage.k8s.io/node-publish-secret-namespace": namespace,
				}
			}

			// Add extra labels if provided
			if len(sc.ExtraLabels) > 0 {
				class["extraLabels"] = sc.ExtraLabels
			}

			classes = append(classes, class)
		}
		return classes
	}

	// Default storage class with encryption
	return []map[string]interface{}{
		{
			"name":                "hcloud-volumes",
			"reclaimPolicy":       "Delete",
			"defaultStorageClass": true,
			"extraParameters": map[string]interface{}{
				"csi.storage.k8s.io/node-publish-secret-name":      secretName,
				"csi.storage.k8s.io/node-publish-secret-namespace": namespace,
			},
		},
	}
}

// Verify checks if CSI is installed and running correctly.
func (c *CSI) Verify(ctx context.Context, k8sClient addons.K8sClient) error {
	log.Printf("Verifying %s installation...", addonName)

	// Check if secret exists
	secretExists, err := k8sClient.SecretExists(ctx, namespace, secretName)
	if err != nil {
		return fmt.Errorf("failed to check secret existence: %w", err)
	}
	if !secretExists {
		return fmt.Errorf("secret %s not found in namespace %s", secretName, namespace)
	}

	// Wait for controller deployment to be ready
	log.Printf("Waiting for %s controller to be ready...", addonName)
	if err := k8sClient.WaitForDeployment(ctx, namespace, controllerName, 5*time.Minute); err != nil {
		return fmt.Errorf("controller deployment not ready: %w", err)
	}

	// Wait for node daemonset to be ready
	log.Printf("Waiting for %s node daemonset to be ready...", addonName)
	if err := k8sClient.WaitForDaemonSet(ctx, namespace, nodeName, 5*time.Minute); err != nil {
		return fmt.Errorf("node daemonset not ready: %w", err)
	}

	// Verify controller pods
	controllerPods, err := k8sClient.GetPods(ctx, namespace, fmt.Sprintf("app=%s", controllerName))
	if err != nil {
		return fmt.Errorf("failed to get controller pods: %w", err)
	}

	// Verify node pods
	nodePods, err := k8sClient.GetPods(ctx, namespace, fmt.Sprintf("app=%s", nodeName))
	if err != nil {
		return fmt.Errorf("failed to get node pods: %w", err)
	}

	log.Printf("%s verified successfully (controller: %d pods, node: %d pods)",
		addonName, len(controllerPods), len(nodePods))
	return nil
}
