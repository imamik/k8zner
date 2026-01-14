package helm

import (
	"bytes"
	"fmt"
	"io/fs"
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
	// Load chart's default values from values.yaml
	// ch.Values is already a map[string]interface{} from the loaded chart
	chartDefaults := make(Values)
	if len(ch.Values) > 0 {
		chartDefaults = Values(ch.Values)
	}

	// Deep merge provided values with chart defaults
	// This ensures nested objects (like controller.podSecurityContext) are preserved
	mergedValues := DeepMerge(chartDefaults, values)

	// Prepare chart values
	chartValues := chartutil.Values(mergedValues)

	// Create release options
	releaseOptions := chartutil.ReleaseOptions{
		Name:      r.chartName,
		Namespace: r.namespace,
		IsInstall: true,
	}

	// Set capabilities for modern Kubernetes (1.31.0)
	// This ensures templates use current API versions (e.g., policy/v1 instead of v1beta1)
	capabilities := chartutil.DefaultCapabilities.Copy()
	capabilities.KubeVersion.Version = "v1.31.0"
	capabilities.KubeVersion.Major = "1"
	capabilities.KubeVersion.Minor = "31"

	// Prepare values for rendering (adds built-in values like .Release, .Chart, etc.)
	valuesToRender, err := chartutil.ToRenderValues(ch, chartValues, releaseOptions, capabilities)
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

	// Read all template files recursively using WalkDir
	templatesPath := filepath.Join(chartPath, "templates")
	err = fs.WalkDir(templatesFS, templatesPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Read the file
		data, err := templatesFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", path, err)
		}

		// Calculate relative path from chartPath
		relPath, err := filepath.Rel(chartPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		files = append(files, &loader.BufferedFile{
			Name: relPath,
			Data: data,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk templates directory: %w", err)
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
