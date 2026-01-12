// Package helm provides a lightweight abstraction for rendering Helm charts
// as Kubernetes manifests. Charts are pre-rendered at build time and embedded
// in the binary, matching Terraform's offline-first approach.
package helm

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

//go:embed templates/*
var templatesFS embed.FS

// Chart represents metadata about an embedded helm chart.
type Chart struct {
	Name       string
	Version    string
	Repository string
}

// listCharts returns all embedded helm chart names.
func listCharts() ([]string, error) {
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory: %w", err)
	}

	charts := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			charts = append(charts, entry.Name())
		}
	}
	return charts, nil
}

// readChartManifests reads all YAML files from an embedded chart directory.
func readChartManifests(chartName string) ([][]byte, error) {
	chartPath := filepath.Join("templates", chartName)

	var manifests [][]byte
	err := fs.WalkDir(templatesFS, chartPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isYAMLFile(d.Name()) {
			return nil
		}

		content, err := templatesFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}
		manifests = append(manifests, content)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to read chart manifests for %s: %w", chartName, err)
	}

	if len(manifests) == 0 {
		return nil, fmt.Errorf("no manifests found for chart %s", chartName)
	}

	return manifests, nil
}

// isYAMLFile checks if a filename is a YAML manifest file.
func isYAMLFile(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}
