package helm

import (
	"bytes"
	"fmt"
	"path/filepath"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/engine"
)

// Renderer renders embedded helm charts with provided values.
type Renderer struct {
	chartName string
	namespace string
}

// NewRenderer creates a renderer for the specified chart.
func NewRenderer(chartName, namespace string) *Renderer {
	return &Renderer{
		chartName: chartName,
		namespace: namespace,
	}
}

// Render processes the chart with values and returns combined YAML manifests.
func (r *Renderer) Render(values Values) ([]byte, error) {
	// Load embedded chart
	chartPath := filepath.Join("templates", r.chartName)
	files, err := loadChartFiles(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart files: %w", err)
	}

	// Create chart from files
	loadedChart, err := loader.LoadFiles(files)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	// Render chart with values
	manifests, err := r.renderChart(loadedChart, values)
	if err != nil {
		return nil, fmt.Errorf("failed to render chart: %w", err)
	}

	return manifests, nil
}

// renderChart uses helm engine to render the chart with values.
func (r *Renderer) renderChart(ch *chart.Chart, values Values) ([]byte, error) {
	// Prepare chart values
	chartValues := chartutil.Values(values)

	// Create release options
	releaseOptions := chartutil.ReleaseOptions{
		Name:      r.chartName,
		Namespace: r.namespace,
		IsInstall: true,
	}

	// Merge chart values with defaults
	valuesToRender, err := chartutil.ToRenderValues(ch, chartValues, releaseOptions, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare values: %w", err)
	}

	// Render templates
	settings := cli.New()

	eng := engine.Engine{
		Strict:   false,
		LintMode: false,
	}

	rendered, err := eng.Render(ch, valuesToRender)
	if err != nil {
		return nil, fmt.Errorf("failed to render templates: %w", err)
	}

	// Combine all rendered manifests
	var combined bytes.Buffer
	for name, content := range rendered {
		// Skip empty files and notes
		if len(content) == 0 || filepath.Base(name) == "NOTES.txt" {
			continue
		}

		if combined.Len() > 0 {
			combined.WriteString("\n---\n")
		}
		combined.WriteString(content)
	}

	_ = settings // Avoid unused variable warning

	return combined.Bytes(), nil
}

// loadChartFiles reads all files from an embedded chart directory.
func loadChartFiles(chartPath string) ([]*loader.BufferedFile, error) {
	var files []*loader.BufferedFile

	entries, err := templatesFS.ReadDir(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read chart directory: %w", err)
	}

	// Read Chart.yaml
	chartFile := filepath.Join(chartPath, "Chart.yaml")
	chartData, err := templatesFS.ReadFile(chartFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read Chart.yaml: %w", err)
	}
	files = append(files, &loader.BufferedFile{
		Name: "Chart.yaml",
		Data: chartData,
	})

	// Read values.yaml if exists
	valuesFile := filepath.Join(chartPath, "values.yaml")
	valuesData, err := templatesFS.ReadFile(valuesFile)
	if err == nil {
		files = append(files, &loader.BufferedFile{
			Name: "values.yaml",
			Data: valuesData,
		})
	}

	// Read all template files
	templatesPath := filepath.Join(chartPath, "templates")
	templateEntries, err := templatesFS.ReadDir(templatesPath)
	if err == nil {
		for _, entry := range templateEntries {
			if entry.IsDir() {
				continue
			}

			filePath := filepath.Join(templatesPath, entry.Name())
			data, err := templatesFS.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read template %s: %w", entry.Name(), err)
			}

			files = append(files, &loader.BufferedFile{
				Name: filepath.Join("templates", entry.Name()),
				Data: data,
			})
		}
	}

	// Read CRDs if they exist
	crdsPath := filepath.Join(chartPath, "crds")
	crdEntries, err := templatesFS.ReadDir(crdsPath)
	if err == nil {
		for _, entry := range crdEntries {
			if entry.IsDir() {
				continue
			}

			filePath := filepath.Join(crdsPath, entry.Name())
			data, err := templatesFS.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read CRD %s: %w", entry.Name(), err)
			}

			files = append(files, &loader.BufferedFile{
				Name: filepath.Join("crds", entry.Name()),
				Data: data,
			})
		}
	}

	_ = entries // Avoid unused warning

	return files, nil
}

// RenderChart is a convenience function for rendering a chart with values.
func RenderChart(chartName, namespace string, values Values) ([]byte, error) {
	renderer := NewRenderer(chartName, namespace)
	return renderer.Render(values)
}
