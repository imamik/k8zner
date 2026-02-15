package addons

import (
	"context"
	"fmt"
	"log"

	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// applyTalosCCM installs the Talos Cloud Controller Manager.
// This is separate from the Hetzner CCM - it's the Siderolabs Talos CCM
// which provides node lifecycle management features.
// Note: Default version is set in load.go during config loading.
func applyTalosCCM(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	version := cfg.Addons.TalosCCM.Version

	// Build the manifest URL
	// Format: https://raw.githubusercontent.com/siderolabs/talos-cloud-controller-manager/{version}/docs/deploy/cloud-controller-manager-daemonset.yml
	manifestURL := fmt.Sprintf(
		"https://raw.githubusercontent.com/siderolabs/talos-cloud-controller-manager/%s/docs/deploy/cloud-controller-manager-daemonset.yml",
		version,
	)

	log.Printf("Installing Talos CCM %s...", version)

	manifestBytes, err := fetchManifestURL(ctx, manifestURL)
	if err != nil {
		return fmt.Errorf("failed to fetch Talos CCM from %s: %w", manifestURL, err)
	}

	// Talos CCM runs with hostNetwork on control plane nodes.
	// When kube-proxy is disabled, inject env vars to reach the API server directly.
	manifestBytes, err = patchHostNetworkAPIAccess(manifestBytes, "talos-cloud-controller-manager")
	if err != nil {
		return fmt.Errorf("failed to patch Talos CCM API access: %w", err)
	}

	if err := applyManifests(ctx, client, "talos-ccm", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply Talos CCM: %w", err)
	}

	log.Printf("Talos CCM %s installed successfully", version)
	return nil
}
