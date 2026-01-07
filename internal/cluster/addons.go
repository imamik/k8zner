package cluster

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
)

//go:embed addons/*
var addonsFS embed.FS

// applyManifests applies YAML manifests for a given addon.
// It reads manifests from the embedded filesystem, processes templates,
// and applies them to the cluster using kubectl.
//
// Template files (.yaml.tmpl) are processed with the provided data.
// Regular YAML files (.yaml) are used as-is.
//
// All manifests are combined and applied in a single kubectl apply call.
func (r *Reconciler) applyManifests(ctx context.Context, addonName string, kubeconfigPath string, data any) error {
	log.Printf("Applying manifests for addon: %s", addonName)

	// Read addon directory
	addonPath := filepath.Join("addons", addonName)
	entries, err := addonsFS.ReadDir(addonPath)
	if err != nil {
		return fmt.Errorf("failed to read addon directory %s: %w", addonPath, err)
	}

	var combinedYAML bytes.Buffer

	// Process each manifest file
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		filePath := filepath.Join(addonPath, fileName)

		// Read file content
		content, err := addonsFS.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		var processedContent string

		// Check if it's a template file
		if strings.HasSuffix(fileName, ".yaml.tmpl") {
			// Parse and execute template
			tmpl, err := template.New(fileName).Parse(string(content))
			if err != nil {
				return fmt.Errorf("failed to parse template %s: %w", fileName, err)
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, data); err != nil {
				return fmt.Errorf("failed to execute template %s: %w", fileName, err)
			}

			processedContent = buf.String()
			log.Printf("  Processed template: %s", fileName)
		} else if strings.HasSuffix(fileName, ".yaml") || strings.HasSuffix(fileName, ".yml") {
			// Use YAML file as-is
			processedContent = string(content)
			log.Printf("  Included manifest: %s", fileName)
		} else {
			// Skip non-YAML files
			log.Printf("  Skipped non-YAML file: %s", fileName)
			continue
		}

		// Add to combined YAML with document separator
		if combinedYAML.Len() > 0 {
			combinedYAML.WriteString("\n---\n")
		}
		combinedYAML.WriteString(processedContent)
	}

	if combinedYAML.Len() == 0 {
		return fmt.Errorf("no YAML manifests found for addon %s", addonName)
	}

	// Write combined YAML to temp file
	tmpfile, err := os.CreateTemp("", fmt.Sprintf("%s-*.yaml", addonName))
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(combinedYAML.Bytes()); err != nil {
		tmpfile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpfile.Close()

	// Apply manifests using kubectl
	log.Printf("Applying combined manifests for %s using kubectl...", addonName)
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", tmpfile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed for %s: %w\nOutput: %s", addonName, err, string(output))
	}

	log.Printf("Successfully applied %s addon manifests", addonName)
	log.Printf("kubectl output:\n%s", string(output))

	return nil
}
