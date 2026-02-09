package addons

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/addons/k8sclient"
	"github.com/imamik/k8zner/internal/config"
)

//go:embed operator-chart/* operator-chart/templates/* operator-chart/crds/*
var operatorChartFS embed.FS

// applyOperator installs the k8zner-operator for self-healing functionality.
func applyOperator(ctx context.Context, client k8sclient.Client, cfg *config.Config) error {
	if !cfg.Addons.Operator.Enabled {
		return nil
	}

	// Extract embedded chart to temp directory
	chartPath, cleanup, err := extractOperatorChart()
	if err != nil {
		return fmt.Errorf("failed to extract operator chart: %w", err)
	}
	defer cleanup()

	// Build operator values
	values := buildOperatorValues(cfg)

	// Apply CRDs first (they are in the crds subdirectory of the chart)
	crdPath := filepath.Join(chartPath, "crds")
	if err := applyCRDs(ctx, client, crdPath); err != nil {
		return fmt.Errorf("failed to apply operator CRDs: %w", err)
	}

	// Render helm chart from local path
	manifestBytes, err := helm.RenderFromPath(chartPath, "k8zner-operator", "k8zner-system", values)
	if err != nil {
		return fmt.Errorf("failed to render operator chart: %w", err)
	}

	// Create namespace if it doesn't exist
	nsManifest := `apiVersion: v1
kind: Namespace
metadata:
  name: k8zner-system
  labels:
    app.kubernetes.io/name: k8zner-operator
    app.kubernetes.io/managed-by: k8zner`
	if err := client.ApplyManifests(ctx, []byte(nsManifest), "k8zner"); err != nil {
		return fmt.Errorf("failed to create operator namespace: %w", err)
	}

	// Apply manifests to cluster
	if err := applyManifests(ctx, client, "k8zner-operator", manifestBytes); err != nil {
		return fmt.Errorf("failed to apply operator manifests: %w", err)
	}

	return nil
}

// extractOperatorChart extracts the embedded chart to a temp directory.
func extractOperatorChart() (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "k8zner-operator-chart-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	// Walk and extract embedded files
	err = fs.WalkDir(operatorChartFS, "operator-chart", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate destination path
		relPath, _ := filepath.Rel("operator-chart", path)
		destPath := filepath.Join(tempDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0750)
		}

		// Read and write file
		data, err := operatorChartFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}

		return os.WriteFile(destPath, data, 0600)
	})

	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to extract chart: %w", err)
	}

	return tempDir, cleanup, nil
}

// applyCRDs applies the CRD manifests from the specified directory.
func applyCRDs(ctx context.Context, client k8sclient.Client, crdPath string) error {
	entries, err := os.ReadDir(crdPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No CRDs to apply
		}
		return fmt.Errorf("failed to read CRD directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		crdFile := filepath.Join(crdPath, entry.Name())
		data, err := os.ReadFile(crdFile)
		if err != nil {
			return fmt.Errorf("failed to read CRD file %s: %w", entry.Name(), err)
		}

		if err := client.ApplyManifests(ctx, data, "k8zner"); err != nil {
			return fmt.Errorf("failed to apply CRD %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// buildOperatorValues creates helm values for the operator deployment.
func buildOperatorValues(cfg *config.Config) helm.Values {
	// Default to "main" if no version specified
	version := cfg.Addons.Operator.Version
	if version == "" {
		version = "main"
	}
	// Allow environment variable override for E2E testing with custom operator images
	if envVersion := os.Getenv("K8ZNER_OPERATOR_VERSION"); envVersion != "" {
		version = envVersion
	}
	// Docker image tags don't support slashes; sanitize branch names like "refactor/foo"
	version = strings.ReplaceAll(version, "/", "-")

	// With hostNetwork, each replica needs exclusive host ports (8080/8081),
	// so we can't run more replicas than available control plane nodes.
	replicaCount := 2
	controlPlaneCount := getControlPlaneCount(cfg)
	if cfg.Addons.Operator.HostNetwork && controlPlaneCount < replicaCount {
		replicaCount = controlPlaneCount
	}

	values := helm.Values{
		"replicaCount": replicaCount,
		"image": helm.Values{
			"repository": "ghcr.io/imamik/k8zner-operator",
			"pullPolicy": "Always",
			"tag":        version,
		},
		"credentials": helm.Values{
			"hcloudToken": cfg.HCloudToken,
		},
		"leaderElection": helm.Values{
			"enabled":      true,
			"resourceName": "k8zner-operator",
		},
		"resources": helm.Values{
			"limits": helm.Values{
				"cpu":    "200m",
				"memory": "256Mi",
			},
			"requests": helm.Values{
				"cpu":    "100m",
				"memory": "128Mi",
			},
		},
		// Use chart defaults for nodeSelector and tolerations:
		// - nodeSelector: node-role.kubernetes.io/control-plane: ""
		// - tolerations: control-plane NoSchedule taint
		// Talos control plane nodes have both the label and taint
	}

	// Enable hostNetwork mode for running before CNI is installed
	// This allows the operator to run on the bootstrap node before Cilium is deployed
	if cfg.Addons.Operator.HostNetwork {
		values["hostNetwork"] = true
		// Use host's DNS resolver directly (not Kubernetes DNS)
		// This is required because CoreDNS needs CNI, but the operator must
		// download the Cilium chart before CNI is installed
		values["dnsPolicy"] = "Default"
	}

	// Enable ServiceMonitor if monitoring is enabled
	if cfg.Addons.KubePrometheusStack.Enabled {
		values["metrics"] = helm.Values{
			"enabled": true,
			"port":    8080,
			"serviceMonitor": helm.Values{
				"enabled":  true,
				"interval": "30s",
			},
		}
	}

	return values
}
