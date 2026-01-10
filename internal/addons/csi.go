package addons

import (
	"context"
	"fmt"
)

// applyCSI installs the Hetzner Cloud CSI driver.
func applyCSI(ctx context.Context, kubeconfigPath, token string, controlPlaneCount int, defaultStorageClass bool) error {
	// Determine controller replicas based on control plane count
	controllerReplicas := 1
	if controlPlaneCount > 1 {
		controllerReplicas = 2
	}

	templateData := map[string]string{
		"Token":                 token,
		"ControllerReplicas":    fmt.Sprintf("%d", controllerReplicas),
		"IsDefaultStorageClass": fmt.Sprintf("%t", defaultStorageClass),
	}

	manifestBytes, err := readAndProcessManifests("hcloud-csi", templateData)
	if err != nil {
		return fmt.Errorf("failed to prepare CSI manifests: %w", err)
	}

	if err := applyWithKubectl(ctx, kubeconfigPath, "hcloud-csi", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply CSI manifests: %w", err)
	}

	return nil
}
