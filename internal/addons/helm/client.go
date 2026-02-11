// Package helm provides Helm chart downloading and rendering capabilities.
package helm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

// ChartSpec defines the specification for downloading a Helm chart.
type ChartSpec struct {
	Repository string // e.g., "https://traefik.github.io/charts"
	Name       string // e.g., "traefik"
	Version    string // e.g., "39.0.0"
}

// DownloadChart downloads a chart from a repository and returns the loaded chart.
// Charts are cached on disk to avoid repeated downloads. Each call returns a
// freshly loaded *chart.Chart from the cached archive, avoiding shared mutable state.
func DownloadChart(ctx context.Context, spec ChartSpec) (*chart.Chart, error) {
	// Download to disk cache (no-op if already cached)
	chartPath, err := downloadChartToCache(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to download chart %s/%s:%s: %w", spec.Repository, spec.Name, spec.Version, err)
	}

	// Load a fresh chart from the cached archive
	loadedChart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart from %s: %w", chartPath, err)
	}

	return loadedChart, nil
}

// downloadChartToCache downloads a chart archive to the cache directory.
func downloadChartToCache(ctx context.Context, spec ChartSpec) (string, error) {
	cachePath := getCachePath()

	// Create cache directory if it doesn't exist
	// Using 0750 for directory permissions (owner rwx, group rx, others none)
	if err := os.MkdirAll(cachePath, 0750); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Check if chart already exists in disk cache
	chartFileName := fmt.Sprintf("%s-%s.tgz", spec.Name, spec.Version)
	chartPath := filepath.Join(cachePath, chartFileName)

	if _, err := os.Stat(chartPath); err == nil {
		// Chart already exists in cache
		return chartPath, nil
	}

	// Set up Helm CLI settings
	settings := cli.New()

	// Create a chart downloader
	getters := getter.All(settings)

	// Create index entry for the chart
	chartURL, err := repo.FindChartInRepoURL(spec.Repository, spec.Name, spec.Version, "", "", "", getters)
	if err != nil {
		return "", fmt.Errorf("failed to find chart URL: %w", err)
	}

	// Download the chart
	httpGetter, err := getter.NewHTTPGetter()
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP getter: %w", err)
	}

	data, err := httpGetter.Get(chartURL)
	if err != nil {
		return "", fmt.Errorf("failed to download chart from %s: %w", chartURL, err)
	}

	// Write the chart archive to cache
	// Using 0600 for file permissions (owner rw only)
	if err := os.WriteFile(chartPath, data.Bytes(), 0600); err != nil {
		return "", fmt.Errorf("failed to write chart to cache: %w", err)
	}

	return chartPath, nil
}

// getCachePath returns the cache directory for downloaded charts.
// Uses XDG_CACHE_HOME if set, otherwise ~/.cache/k8zner/charts
func getCachePath() string {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			// Fall back to current directory if home is not available
			return filepath.Join(".", ".cache", "k8zner", "charts")
		}
		cacheDir = filepath.Join(homeDir, ".cache")
	}
	return filepath.Join(cacheDir, "k8zner", "charts")
}

// clearCache removes all cached charts from disk.
func clearCache() error {
	cachePath := getCachePath()
	if err := os.RemoveAll(cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear cache directory: %w", err)
	}

	return nil
}

// loadChartFromPath loads a Helm chart from a local filesystem path.
// This is useful for charts embedded in the application or during development.
func loadChartFromPath(chartPath string) (*chart.Chart, error) {
	loadedChart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart from %s: %w", chartPath, err)
	}
	return loadedChart, nil
}
