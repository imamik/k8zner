package addons

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
)

// readAndProcessManifests reads manifest files for an addon from the embedded
// filesystem, processes templates with the provided data, and combines them
// into a single YAML document.
func readAndProcessManifests(addonName string, data any) ([]byte, error) {
	addonPath := filepath.Join("manifests", addonName)
	entries, err := manifestsFS.ReadDir(addonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifests for addon %s at %s: %w", addonName, addonPath, err)
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
			return nil, fmt.Errorf("failed to read manifest file %s: %w", filePath, err)
		}

		var processedContent string

		if strings.HasSuffix(fileName, ".yaml.tmpl") {
			processedContent, err = processTemplate(fileName, content, data)
			if err != nil {
				return nil, err
			}
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
		return nil, fmt.Errorf("no YAML manifests found for addon %s", addonName)
	}

	return combinedYAML.Bytes(), nil
}

// processTemplate processes a template file with the provided data.
func processTemplate(name string, content []byte, data any) (string, error) {
	tmpl, err := template.New(name).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", name, err)
	}

	return buf.String(), nil
}
