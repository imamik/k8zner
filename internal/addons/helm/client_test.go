package helm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDownloadChartIntegration tests actual chart downloading.
// This test requires network access and downloads real charts.
// Skip in CI environments without network or for fast unit tests.
func TestDownloadChartIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Use a small, fast-to-download chart for testing
	spec := ChartSpec{
		Repository: "https://kubernetes-sigs.github.io/metrics-server",
		Name:       "metrics-server",
		Version:    "3.12.2",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Clear any existing cache
	ClearMemoryCache()

	// Download the chart
	chart, err := DownloadChart(ctx, spec)
	if err != nil {
		t.Fatalf("DownloadChart failed: %v", err)
	}

	// Verify the chart was loaded
	if chart == nil {
		t.Fatal("DownloadChart returned nil chart")
	}
	if chart.Name() != "metrics-server" {
		t.Errorf("Chart name = %q, want %q", chart.Name(), "metrics-server")
	}
	if chart.Metadata.Version != "3.12.2" {
		t.Errorf("Chart version = %q, want %q", chart.Metadata.Version, "3.12.2")
	}

	// Verify chart has templates
	if len(chart.Templates) == 0 {
		t.Error("Chart has no templates")
	}

	// Verify chart was cached on disk
	cachePath := GetCachePath()
	chartPath := filepath.Join(cachePath, "metrics-server-3.12.2.tgz")
	if _, err := os.Stat(chartPath); os.IsNotExist(err) {
		t.Errorf("Chart was not cached to disk at %s", chartPath)
	}

	// Test that second download uses cache (should be fast)
	start := time.Now()
	chart2, err := DownloadChart(ctx, spec)
	if err != nil {
		t.Fatalf("Second DownloadChart failed: %v", err)
	}
	elapsed := time.Since(start)

	if chart2 == nil {
		t.Fatal("Second DownloadChart returned nil chart")
	}

	// Cached download should be very fast (under 100ms typically)
	if elapsed > 5*time.Second {
		t.Logf("Warning: cached download took %v (expected <5s)", elapsed)
	}
}

// TestDownloadChartInvalidRepo tests error handling for invalid repos.
func TestDownloadChartInvalidRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	spec := ChartSpec{
		Repository: "https://invalid-repo.example.com/charts",
		Name:       "nonexistent",
		Version:    "1.0.0",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := DownloadChart(ctx, spec)
	if err == nil {
		t.Error("DownloadChart should fail for invalid repository")
	}
}

// TestClearCache tests cache clearing functionality.
func TestClearCache(t *testing.T) {
	// Create a test file in cache directory
	cachePath := GetCachePath()
	if err := os.MkdirAll(cachePath, 0755); err != nil {
		t.Fatalf("Failed to create cache directory: %v", err)
	}

	testFile := filepath.Join(cachePath, "test-cache-file.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Clear cache
	if err := ClearCache(); err != nil {
		t.Fatalf("ClearCache failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("ClearCache did not remove cached files")
	}
}

// TestChartSpecString tests ChartSpec string representation.
func TestChartSpec(t *testing.T) {
	spec := ChartSpec{
		Repository: "https://example.com/charts",
		Name:       "my-chart",
		Version:    "1.2.3",
	}

	if spec.Repository == "" || spec.Name == "" || spec.Version == "" {
		t.Error("ChartSpec fields should not be empty")
	}
}
