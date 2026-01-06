package addons

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// HelmRenderer handles Helm chart templating.
type HelmRenderer struct{}

// NewHelmRenderer creates a new Helm renderer.
func NewHelmRenderer() *HelmRenderer {
	return &HelmRenderer{}
}

// RenderChart renders a Helm chart with the given values using the helm CLI.
// This uses `helm template` command to render manifests locally without requiring Tiller.
func (h *HelmRenderer) RenderChart(
	repoURL, chartName, version, namespace string,
	values map[string]interface{},
) (string, error) {
	// Check if helm CLI is available
	if _, err := exec.LookPath("helm"); err != nil {
		return "", fmt.Errorf("helm CLI not found in PATH: %w", err)
	}

	// Create temporary directory for helm operations
	tmpDir, err := os.MkdirTemp("", "helm-render-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write values to a temporary file
	valuesFile := filepath.Join(tmpDir, "values.yaml")
	if err := h.writeValuesFile(valuesFile, values); err != nil {
		return "", fmt.Errorf("failed to write values file: %w", err)
	}

	// Construct chart reference
	// If repoURL contains a URL scheme, we need to add the repo first
	chartRef := chartName
	if repoURL != "" && repoURL != chartName {
		// Add helm repository
		repoName := fmt.Sprintf("addon-repo-%s", chartName)
		addRepoCmd := exec.Command("helm", "repo", "add", repoName, repoURL)
		if output, err := addRepoCmd.CombinedOutput(); err != nil {
			log.Printf("Note: helm repo add failed (may already exist): %v\n%s", err, output)
		}

		// Update repositories to ensure we have latest charts
		updateCmd := exec.Command("helm", "repo", "update")
		if output, err := updateCmd.CombinedOutput(); err != nil {
			log.Printf("Warning: helm repo update failed: %v\n%s", err, output)
		}

		chartRef = fmt.Sprintf("%s/%s", repoName, chartName)
	}

	// Build helm template command
	args := []string{
		"template",
		chartName, // release name
		chartRef,  // chart reference
		"--namespace", namespace,
		"--values", valuesFile,
	}

	if version != "" {
		args = append(args, "--version", version)
	}

	log.Printf("Rendering Helm chart: %s (version: %s) in namespace: %s", chartRef, version, namespace)

	// Execute helm template
	cmd := exec.Command("helm", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("helm template failed: %w\nstderr: %s", err, stderr.String())
	}

	manifest := stdout.String()
	if manifest == "" {
		return "", fmt.Errorf("helm template produced empty output")
	}

	log.Printf("Successfully rendered Helm chart %s (%d bytes)", chartName, len(manifest))
	return manifest, nil
}

// RenderChartFromValues renders a chart with YAML values string.
func (h *HelmRenderer) RenderChartFromValues(
	repoURL, chartName, version, namespace, valuesYAML string,
) (string, error) {
	// Parse YAML values string into map
	values := make(map[string]interface{})
	if valuesYAML != "" {
		if err := yaml.Unmarshal([]byte(valuesYAML), &values); err != nil {
			return "", fmt.Errorf("failed to parse values YAML: %w", err)
		}
	}

	return h.RenderChart(repoURL, chartName, version, namespace, values)
}

// writeValuesFile writes Helm values to a YAML file.
func (h *HelmRenderer) writeValuesFile(path string, values map[string]interface{}) error {
	data, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("failed to marshal values: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
