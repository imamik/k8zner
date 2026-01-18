package addons

import (
	"context"
	"fmt"
	"log"

	"hcloud-k8s/internal/config"
)

// Default values matching Terraform
const (
	defaultGatewayAPIVersion        = "v1.4.1"
	defaultGatewayAPIReleaseChannel = "standard"
)

// applyGatewayAPICRDs installs the Gateway API Custom Resource Definitions.
// See: terraform/talos_config.tf lines 35-37
func applyGatewayAPICRDs(ctx context.Context, kubeconfigPath string, cfg *config.Config) error {
	gatewayConfig := cfg.Addons.GatewayAPICRDs

	// Use defaults if not specified
	version := gatewayConfig.Version
	if version == "" {
		version = defaultGatewayAPIVersion
	}

	releaseChannel := gatewayConfig.ReleaseChannel
	if releaseChannel == "" {
		releaseChannel = defaultGatewayAPIReleaseChannel
	}

	// Validate release channel
	if releaseChannel != "standard" && releaseChannel != "experimental" {
		return fmt.Errorf("invalid Gateway API release channel %q: must be 'standard' or 'experimental'", releaseChannel)
	}

	// Build the manifest URL
	// Format: https://github.com/kubernetes-sigs/gateway-api/releases/download/{version}/{channel}-install.yaml
	manifestURL := fmt.Sprintf(
		"https://github.com/kubernetes-sigs/gateway-api/releases/download/%s/%s-install.yaml",
		version,
		releaseChannel,
	)

	log.Printf("Installing Gateway API CRDs %s (%s channel)...", version, releaseChannel)

	if err := applyFromURL(ctx, kubeconfigPath, "gateway-api-crds", manifestURL); err != nil {
		return fmt.Errorf("failed to apply Gateway API CRDs from %s: %w", manifestURL, err)
	}

	log.Printf("Gateway API CRDs %s installed successfully", version)
	return nil
}
