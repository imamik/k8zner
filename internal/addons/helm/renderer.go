package helm

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

// Renderer renders Helm charts with provided values.
type Renderer struct {
	chartName string
	namespace string
}

// newRenderer creates a renderer for the specified chart.
func newRenderer(chartName, namespace string) *Renderer {
	return &Renderer{
		chartName: chartName,
		namespace: namespace,
	}
}

// RenderFromSpec downloads a chart at runtime and renders it with the provided values.
// This is the primary rendering function that downloads charts from their repositories.
func RenderFromSpec(ctx context.Context, spec ChartSpec, namespace string, values Values) ([]byte, error) {
	// Download the chart
	loadedChart, err := DownloadChart(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to download chart: %w", err)
	}

	// Create renderer and render the chart
	renderer := &Renderer{
		chartName: spec.Name,
		namespace: namespace,
	}

	manifests, err := renderer.renderChart(loadedChart, values)
	if err != nil {
		return nil, fmt.Errorf("failed to render chart: %w", err)
	}

	return manifests, nil
}

// RenderFromPath renders a chart from a local filesystem path with the provided values.
// This is useful for charts embedded in the application or during development.
func RenderFromPath(chartPath, releaseName, namespace string, values Values) ([]byte, error) {
	// Load the chart from the local path
	loadedChart, err := loadChartFromPath(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	// Create renderer and render the chart
	renderer := &Renderer{
		chartName: releaseName,
		namespace: namespace,
	}

	manifests, err := renderer.renderChart(loadedChart, values)
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
	mergedValues := deepMerge(chartDefaults, values)

	// Convert to plain map[string]interface{} recursively
	// This ensures nested Values types are converted to plain maps for Helm
	plainMap := mergedValues.ToMap()

	// Prepare chart values
	chartValues := chartutil.Values(plainMap)

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
	eng := engine.Engine{
		Strict:   false,
		LintMode: false,
	}

	rendered, err := eng.Render(ch, valuesToRender)
	if err != nil {
		return nil, fmt.Errorf("failed to render templates: %w", err)
	}

	// Combine all rendered template manifests
	var combined bytes.Buffer
	for name, content := range rendered {
		// Skip NOTES.txt
		if filepath.Base(name) == "NOTES.txt" {
			continue
		}

		// Skip empty or whitespace-only content
		trimmed := strings.TrimSpace(content)
		if trimmed == "" {
			continue
		}

		if combined.Len() > 0 {
			combined.WriteString("\n---\n")
		}
		combined.WriteString(trimmed)
		combined.WriteString("\n")
	}

	return combined.Bytes(), nil
}
