package addons

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
)

// applyCSI installs the Hetzner Cloud CSI driver.
// See: terraform/hcloud.tf (hcloud_csi)
func applyCSI(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
	// Generate encryption passphrase
	encryptionKey, err := generateEncryptionKey(32)
	if err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// Create hcloud-csi-secret for encryption
	if err := createCSISecret(ctx, kubeconfigPath, encryptionKey); err != nil {
		return fmt.Errorf("failed to create CSI secret: %w", err)
	}

	// Build CSI values matching terraform configuration
	values := buildCSIValues(cfg)

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
func buildCSIValues(cfg *config.Config) helm.Values {
	controlPlaneCount := getControlPlaneCount(cfg)
	defaultStorageClass := cfg.Addons.CSI.DefaultStorageClass

	replicas := 1
	if controlPlaneCount > 1 {
		replicas = 2
	}

	// Storage classes with encryption support (matches terraform defaults)
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
					"key":      "node.cloudprovider.kubernetes.io/uninitialized",
					"operator": "Exists",
				},
			},
		},
		"storageClasses": storageClasses,
	}

	// Merge custom Helm values from config
	return helm.MergeCustomValues(values, cfg.Addons.CSI.Helm.Values)
}

// createCSISecret creates the hcloud-csi-secret for volume encryption.
func createCSISecret(ctx context.Context, kubeconfigPath, encryptionKey string) error {
	// Delete existing secret if it exists (ignore errors)
	deleteCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"delete", "secret", "hcloud-csi-secret",
		"--namespace", "kube-system",
		"--ignore-not-found",
	)
	_ = deleteCmd.Run()

	// Create new secret
	//nolint:gosec // kubectl command with internally generated encryption key
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"create", "secret", "generic", "hcloud-csi-secret",
		"--namespace", "kube-system",
		"--from-literal=encryption-passphrase="+encryptionKey,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create hcloud-csi-secret: %w\nOutput: %s", err, output)
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
