package addons

import (
	"context"
	"fmt"
	"log"

	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

// Default values for the addon
const (
	defaultPrometheusOperatorCRDsVersion = "v0.87.1"
)

// applyPrometheusOperatorCRDs installs the Prometheus Operator Custom Resource Definitions.
func applyPrometheusOperatorCRDs(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	promConfig := cfg.Addons.PrometheusOperatorCRDs

	// Use default if not specified
	version := promConfig.Version
	if version == "" {
		version = defaultPrometheusOperatorCRDsVersion
	}

	// Build the manifest URL
	// Format: https://github.com/prometheus-operator/prometheus-operator/releases/download/{version}/stripped-down-crds.yaml
	manifestURL := fmt.Sprintf(
		"https://github.com/prometheus-operator/prometheus-operator/releases/download/%s/stripped-down-crds.yaml",
		version,
	)

	log.Printf("Installing Prometheus Operator CRDs %s...", version)

	if err := applyFromURL(ctx, client, "prometheus-operator-crds", manifestURL); err != nil {
		return fmt.Errorf("failed to apply Prometheus Operator CRDs from %s: %w", manifestURL, err)
	}

	log.Printf("Prometheus Operator CRDs %s installed successfully", version)
	return nil
}
