package addons

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyCSI installs the Hetzner Cloud CSI driver.
// See: terraform/hcloud.tf (hcloud_csi)
func applyCSI(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	// Generate encryption passphrase
	encryptionKey, err := generateEncryptionKey(32)
	if err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// Create hcloud-csi-secret for encryption
	if err := createCSISecret(ctx, client, encryptionKey); err != nil {
		return fmt.Errorf("failed to create CSI secret: %w", err)
	}

	// Build CSI values matching terraform configuration
	values := buildCSIValues(cfg)

	// Get chart spec with any config overrides
	spec := helm.GetChartSpec("hcloud-csi", cfg.Addons.CSI.Helm)

	// Render helm chart with values
	manifestBytes, err := helm.RenderFromSpec(ctx, spec, "kube-system", values)
	if err != nil {
		return fmt.Errorf("failed to render CSI chart: %w", err)
	}

	// Post-render: inject dnsPolicy since the CSI chart doesn't support it natively.
	// Using host DNS avoids the CoreDNS dependency during bootstrap — without this,
	// the CSI controller can't resolve api.hetzner.cloud and enters CrashLoopBackOff.
	manifestBytes, err = patchDeploymentDNSPolicy(manifestBytes, "hcloud-csi-controller", "Default")
	if err != nil {
		return fmt.Errorf("failed to patch CSI controller dnsPolicy: %w", err)
	}

	// Apply manifests to cluster
	if err := applyManifests(ctx, client, "hcloud-csi", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply CSI manifests: %w", err)
	}

	return nil
}

// buildCSIValues creates helm values matching terraform configuration.
// See: terraform/hcloud.tf lines 119-156
func buildCSIValues(cfg *config.Config) helm.Values {
	controlPlaneCount := getControlPlaneCount(cfg)
	defaultStorageClass := cfg.Addons.CSI.DefaultStorageClass

	replicas := 1
	if controlPlaneCount > 1 {
		replicas = 2
	}

	// Storage classes with encryption support (matches terraform defaults)
	// Note: The Hetzner CSI chart already sets volumeBindingMode: WaitForFirstConsumer by default
	storageClasses := []helm.Values{
		{
			"name":                "hcloud-volumes-encrypted",
			"defaultStorageClass": defaultStorageClass,
			"reclaimPolicy":       "Delete",
			"extraParameters": helm.Values{
				"csi.storage.k8s.io/node-publish-secret-name":      "hcloud-csi-secret",
				"csi.storage.k8s.io/node-publish-secret-namespace": "kube-system",
			},
		},
		{
			"name":                "hcloud-volumes",
			"defaultStorageClass": false,
			"reclaimPolicy":       "Delete",
		},
	}

	values := helm.Values{
		"controller": helm.Values{
			"replicaCount": replicas,
			"hcloudToken": helm.Values{
				"existingSecret": helm.Values{
					"name": "hcloud",
					"key":  "token",
				},
			},
			"podDisruptionBudget": helm.Values{
				"create":         true,
				"minAvailable":   nil,
				"maxUnavailable": "1",
			},
			"topologySpreadConstraints": []helm.Values{
				{
					"topologyKey":       "kubernetes.io/hostname",
					"maxSkew":           1,
					"whenUnsatisfiable": "DoNotSchedule",
					"labelSelector": helm.Values{
						"matchLabels": helm.Values{
							"app.kubernetes.io/name":      "hcloud-csi",
							"app.kubernetes.io/instance":  "hcloud-csi",
							"app.kubernetes.io/component": "controller",
						},
					},
					"matchLabelKeys": []string{"pod-template-hash"},
				},
			},
			"nodeSelector": helm.ControlPlaneNodeSelector(),
			"tolerations":  helm.BootstrapTolerations(),
		},
		"storageClasses": storageClasses,
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, cfg.Addons.CSI.Helm.Values)
}

// createCSISecret creates the hcloud-csi-secret for volume encryption.
func createCSISecret(ctx context.Context, client k8sclient.Client, encryptionKey string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hcloud-csi-secret",
			Namespace: "kube-system",
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"encryption-passphrase": encryptionKey,
		},
	}

	if err := client.CreateSecret(ctx, secret); err != nil {
		return fmt.Errorf("failed to create hcloud-csi-secret: %w", err)
	}

	return nil
}

// generateEncryptionKey creates a random encryption key of the specified byte length.
func generateEncryptionKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
