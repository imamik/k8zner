// Package addons provides functionality for installing cluster addons.
//
// This package handles the application of Kubernetes manifests for various
// addons like Cloud Controller Manager (CCM), CSI drivers, and monitoring tools.
// Manifests are embedded at build time and can use Go templates for configuration.
package addons

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"hcloud-k8s/internal/config"
)

//go:embed manifests/*
var manifestsFS embed.FS

// Apply installs configured addons to the Kubernetes cluster.
//
// This function checks the addon configuration and applies the appropriate
// manifests to the cluster using kubectl. Currently supports:
//   - Hetzner Cloud Controller Manager (CCM)
//
// The kubeconfig must be valid and the cluster must be accessible.
// Addon manifests are embedded in the binary and processed as templates
// with cluster-specific configuration injected at runtime.
func Apply(ctx context.Context, cfg *config.Config, kubeconfig []byte, networkID int64) error {
	if len(kubeconfig) == 0 {
		return fmt.Errorf("kubeconfig is required for addon installation")
	}

	tmpKubeconfig, err := writeTempKubeconfig(kubeconfig)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmpKubeconfig)
	}()

	if cfg.Addons.CCM.Enabled {
		if err := applyCCM(ctx, tmpKubeconfig, cfg.HCloudToken, networkID); err != nil {
			return fmt.Errorf("failed to install CCM: %w", err)
		}
	}

	return nil
}

// applyCCM installs the Hetzner Cloud Controller Manager.
func applyCCM(ctx context.Context, kubeconfigPath, token string, networkID int64) error {
	log.Println("Installing Hetzner Cloud Controller Manager (CCM)...")

	templateData := map[string]string{
		"Token":     token,
		"NetworkID": fmt.Sprintf("%d", networkID),
	}

	if err := applyManifests(ctx, "hcloud-ccm", kubeconfigPath, templateData); err != nil {
		return err
	}

	log.Println("CCM installation completed")
	return nil
}

// applyManifests applies YAML manifests for a given addon.
//
// It reads manifests from the embedded filesystem, processes templates,
// and applies them to the cluster using kubectl.
//
// Template files (.yaml.tmpl) are processed with the provided data.
// Regular YAML files (.yaml) are used as-is.
//
// All manifests are combined and applied in a single kubectl apply call.
func applyManifests(ctx context.Context, addonName string, kubeconfigPath string, data any) error {
	log.Printf("Applying manifests for addon: %s", addonName)

	addonPath := filepath.Join("manifests", addonName)
	entries, err := manifestsFS.ReadDir(addonPath)
	if err != nil {
		return fmt.Errorf("failed to read addon directory %s: %w", addonPath, err)
	}

	var combinedYAML bytes.Buffer

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		filePath := filepath.Join(addonPath, fileName)

		content, err := manifestsFS.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		var processedContent string

		if strings.HasSuffix(fileName, ".yaml.tmpl") {
			tmpl, err := template.New(fileName).Parse(string(content))
			if err != nil {
				return fmt.Errorf("failed to parse template %s: %w", fileName, err)
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, data); err != nil {
				return fmt.Errorf("failed to execute template %s: %w", fileName, err)
			}

			processedContent = buf.String()
		} else if strings.HasSuffix(fileName, ".yaml") || strings.HasSuffix(fileName, ".yml") {
			processedContent = string(content)
		} else {
			continue
		}

		if combinedYAML.Len() > 0 {
			combinedYAML.WriteString("\n---\n")
		}
		combinedYAML.WriteString(processedContent)
	}

	if combinedYAML.Len() == 0 {
		return fmt.Errorf("no YAML manifests found for addon %s", addonName)
	}

	tmpfile, err := os.CreateTemp("", fmt.Sprintf("%s-*.yaml", addonName))
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmpfile.Name())
	}()

	if _, err := tmpfile.Write(combinedYAML.Bytes()); err != nil {
		_ = tmpfile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpfile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// #nosec G204 - kubeconfigPath is from internal config, tmpfile.Name() is a secure temp file we created
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", tmpfile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed for %s: %w\nOutput: %s", addonName, err, string(output))
	}

	log.Printf("Successfully applied %s addon", addonName)
	return nil
}

// writeTempKubeconfig writes kubeconfig to a temporary file and returns the path.
func writeTempKubeconfig(kubeconfig []byte) (string, error) {
	tmpfile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp kubeconfig: %w", err)
	}

	if _, err := tmpfile.Write(kubeconfig); err != nil {
		_ = tmpfile.Close()
		_ = os.Remove(tmpfile.Name())
		return "", fmt.Errorf("failed to write temp kubeconfig: %w", err)
	}

	if err := tmpfile.Close(); err != nil {
		_ = os.Remove(tmpfile.Name())
		return "", fmt.Errorf("failed to close temp kubeconfig: %w", err)
	}

	return tmpfile.Name(), nil
}
