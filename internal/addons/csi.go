package addons

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"hcloud-k8s/internal/addons/helm"
)

// applyCSI installs the Hetzner Cloud CSI driver.
// See: terraform/hcloud.tf (hcloud_csi)
func applyCSI(ctx context.Context, kubeconfigPath, token string, controlPlaneCount int, defaultStorageClass bool) error {
	// Generate encryption passphrase
	encryptionKey, err := generateEncryptionKey(32)
	if err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// Build CSI values matching terraform configuration
	values := buildCSIValues(controlPlaneCount, encryptionKey, defaultStorageClass)

	// Render helm chart with values
	manifestBytes, err := helm.RenderChart("hcloud-csi", "kube-system", values)
	if err != nil {
		return fmt.Errorf("failed to render CSI chart: %w", err)
	}

	// Apply manifests to cluster
	if err := applyWithKubectl(ctx, kubeconfigPath, "hcloud-csi", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply CSI manifests: %w", err)
	}

	return nil
}

// buildCSIValues creates helm values matching terraform configuration.
// See: terraform/hcloud.tf lines 119-156
func buildCSIValues(controlPlaneCount int, encryptionKey string, defaultStorageClass bool) helm.Values {
	replicas := 1
	if controlPlaneCount > 1 {
		replicas = 2
	}

	return helm.Values{
		"controller": helm.Values{
			"replicaCount": replicas,
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
		},
		"storageClasses": []helm.Values{
			{
				"name":                "hcloud-volumes",
				"defaultStorageClass": defaultStorageClass,
				"reclaimPolicy":       "Delete",
			},
		},
		"secret": helm.Values{
			"create": true,
			"data": helm.Values{
				"token":                 "", // Token set via hcloud secret
				"encryption-passphrase": encryptionKey,
			},
		},
	}
}

// generateEncryptionKey creates a random encryption key of the specified byte length.
func generateEncryptionKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
