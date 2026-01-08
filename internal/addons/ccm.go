package addons

import (
	"context"
	"fmt"
)

// applyCCM installs the Hetzner Cloud Controller Manager.
func applyCCM(ctx context.Context, kubeconfigPath, token string, networkID int64) error {
	templateData := map[string]string{
		"Token":     token,
		"NetworkID": fmt.Sprintf("%d", networkID),
	}

	manifestBytes, err := readAndProcessManifests("hcloud-ccm", templateData)
	if err != nil {
		return fmt.Errorf("failed to prepare CCM manifests: %w", err)
	}

	if err := applyWithKubectl(ctx, kubeconfigPath, "hcloud-ccm", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply CCM manifests: %w", err)
	}

	return nil
}
