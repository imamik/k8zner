package addons

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
)

// readAndProcessManifests reads manifest files for an addon from the embedded
// filesystem, processes them as Go templates with the provided data, and combines
// them into a single YAML document.
func readAndProcessManifests(addonName string, data any) ([]byte, error) {
	addonPath := filepath.Join("manifests", addonName)
	entries, err := manifestsFS.ReadDir(addonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifests for addon %s at %s: %w", addonName, addonPath, err)
	}

	var combined bytes.Buffer

	for _, entry := range entries {
		if entry.IsDir() || !isManifestFile(entry.Name()) {
			continue
		}

		content, err := readManifestFile(addonPath, entry.Name())
		if err != nil {
			return nil, err
		}

		processed, err := processTemplate(entry.Name(), content, data)
		if err != nil {
			return nil, err
		}

		appendYAML(&combined, processed)
	}

	if combined.Len() == 0 {
		return nil, fmt.Errorf("no YAML manifests found for addon %s", addonName)
	}

	return combined.Bytes(), nil
}

// isManifestFile checks if a filename is a YAML manifest file.
func isManifestFile(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

// readManifestFile reads a single manifest file from the embedded filesystem.
func readManifestFile(addonPath, fileName string) ([]byte, error) {
	filePath := filepath.Join(addonPath, fileName)
	content, err := manifestsFS.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file %s: %w", filePath, err)
	}
	return content, nil
}

// appendYAML appends a YAML document to the buffer with appropriate separators.
func appendYAML(buffer *bytes.Buffer, content string) {
	if buffer.Len() > 0 {
		buffer.WriteString("\n---\n")
	}
	buffer.WriteString(content)
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
