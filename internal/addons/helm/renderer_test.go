package helm

import (
	"io/fs"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/chart/loader"
)

// TestEmbedIncludesHelperFiles verifies that _helpers.tpl files are embedded.
func TestEmbedIncludesHelperFiles(t *testing.T) {
	charts := []string{
		"hcloud-ccm",
		"metrics-server",
		"cert-manager",
		"ingress-nginx",
		"longhorn",
		"traefik",
	}

	for _, chartName := range charts {
		t.Run(chartName, func(t *testing.T) {
			helperPath := "templates/" + chartName + "/templates/_helpers.tpl"
			content, err := templatesFS.ReadFile(helperPath)
			if err != nil {
				t.Fatalf("Failed to read %s: %v", helperPath, err)
			}
			if len(content) == 0 {
				t.Errorf("_helpers.tpl for %s is empty", chartName)
			}
		})
	}
}

// TestEmbedIncludesCSIHelperFiles verifies CSI helper files are embedded.
func TestEmbedIncludesCSIHelperFiles(t *testing.T) {
	helperFiles := []string{
		"templates/hcloud-csi/templates/_common_images.tpl",
		"templates/hcloud-csi/templates/_common_labels.tpl",
		"templates/hcloud-csi/templates/_common_name.tpl",
	}

	for _, helperPath := range helperFiles {
		t.Run(helperPath, func(t *testing.T) {
			content, err := templatesFS.ReadFile(helperPath)
			if err != nil {
				t.Fatalf("Failed to read %s: %v", helperPath, err)
			}
			if len(content) == 0 {
				t.Errorf("Helper file %s is empty", helperPath)
			}
		})
	}
}

// TestLoadChartFilesIncludesHelpers verifies loadChartFiles loads helper templates.
func TestLoadChartFilesIncludesHelpers(t *testing.T) {
	files, err := loadChartFiles("templates/hcloud-ccm")
	if err != nil {
		t.Fatalf("loadChartFiles failed: %v", err)
	}

	// Check that _helpers.tpl is included
	var foundHelper bool
	for _, f := range files {
		if strings.Contains(f.Name, "_helpers.tpl") {
			foundHelper = true
			if len(f.Data) == 0 {
				t.Error("_helpers.tpl file is empty")
			}
			break
		}
	}

	if !foundHelper {
		t.Error("_helpers.tpl not found in loaded chart files")
	}
}

// TestLoadChartFilesLoadsAllTemplates verifies all template files are loaded.
func TestLoadChartFilesLoadsAllTemplates(t *testing.T) {
	chartPath := "templates/hcloud-ccm"
	files, err := loadChartFiles(chartPath)
	if err != nil {
		t.Fatalf("loadChartFiles failed: %v", err)
	}

	// Verify Chart.yaml is loaded
	var foundChart bool
	for _, f := range files {
		if f.Name == "Chart.yaml" {
			foundChart = true
			break
		}
	}
	if !foundChart {
		t.Error("Chart.yaml not found in loaded files")
	}

	// Verify at least some template files are loaded
	templateCount := 0
	for _, f := range files {
		if strings.Contains(f.Name, "templates/") {
			templateCount++
		}
	}
	if templateCount == 0 {
		t.Error("No template files found")
	}
}

// TestChartLoaderAcceptsFiles verifies Helm loader accepts our file structure.
func TestChartLoaderAcceptsFiles(t *testing.T) {
	files, err := loadChartFiles("templates/hcloud-ccm")
	if err != nil {
		t.Fatalf("loadChartFiles failed: %v", err)
	}

	// Try to load the chart with Helm loader
	chart, err := loader.LoadFiles(files)
	if err != nil {
		t.Fatalf("Helm loader.LoadFiles failed: %v", err)
	}

	// Verify chart loaded successfully
	if chart.Name() == "" {
		t.Error("Chart name is empty")
	}
	if len(chart.Templates) == 0 {
		t.Error("No templates found in loaded chart")
	}

	// Verify _helpers.tpl is in the templates
	var foundHelper bool
	for _, tmpl := range chart.Templates {
		if strings.Contains(tmpl.Name, "_helpers.tpl") {
			foundHelper = true
			break
		}
	}
	if !foundHelper {
		t.Error("_helpers.tpl not found in chart templates")
	}
}

// TestTraefikChartCanBeLoaded verifies the Traefik chart can be loaded by Helm.
func TestTraefikChartCanBeLoaded(t *testing.T) {
	files, err := loadChartFiles("templates/traefik")
	if err != nil {
		t.Fatalf("loadChartFiles failed for traefik: %v", err)
	}

	// Try to load the chart with Helm loader
	chart, err := loader.LoadFiles(files)
	if err != nil {
		t.Fatalf("Helm loader.LoadFiles failed for traefik: %v", err)
	}

	// Verify chart loaded successfully
	if chart.Name() != "traefik" {
		t.Errorf("Expected chart name 'traefik', got '%s'", chart.Name())
	}
	if len(chart.Templates) == 0 {
		t.Error("No templates found in loaded traefik chart")
	}

	// Verify _helpers.tpl is in the templates
	var foundHelper bool
	for _, tmpl := range chart.Templates {
		if strings.Contains(tmpl.Name, "_helpers.tpl") {
			foundHelper = true
			break
		}
	}
	if !foundHelper {
		t.Error("_helpers.tpl not found in traefik chart templates")
	}
}

// TestWalkDirFindsAllFiles verifies fs.WalkDir finds all files including helpers.
func TestWalkDirFindsAllFiles(t *testing.T) {
	templatesPath := "templates/hcloud-ccm/templates"
	var files []string

	err := fs.WalkDir(templatesFS, templatesPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WalkDir failed: %v", err)
	}

	// Verify _helpers.tpl is found
	var foundHelper bool
	for _, path := range files {
		if strings.Contains(path, "_helpers.tpl") {
			foundHelper = true
			break
		}
	}

	if !foundHelper {
		t.Errorf("_helpers.tpl not found in WalkDir results. Found files: %v", files)
	}

	// Verify we found multiple template files
	if len(files) < 3 {
		t.Errorf("Expected multiple template files, found only %d", len(files))
	}
}
