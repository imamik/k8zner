package v2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	t.Parallel(
	// Create a temporary config file
	)

	content := `
name: my-cluster
region: fsn1
mode: ha
workers:
  count: 3
  size: cx32
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "k8zner.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Name != "my-cluster" {
		t.Errorf("Name = %q, want %q", cfg.Name, "my-cluster")
	}
	if cfg.Region != RegionFalkenstein {
		t.Errorf("Region = %q, want %q", cfg.Region, RegionFalkenstein)
	}
	if cfg.Mode != ModeHA {
		t.Errorf("Mode = %q, want %q", cfg.Mode, ModeHA)
	}
	if cfg.Workers.Count != 3 {
		t.Errorf("Workers.Count = %d, want %d", cfg.Workers.Count, 3)
	}
	if cfg.Workers.Size != SizeCX32 {
		t.Errorf("Workers.Size = %q, want %q", cfg.Workers.Size, SizeCX32)
	}
}

func TestLoad_WithDomain(t *testing.T) {
	content := `
name: production
region: nbg1
mode: ha
workers:
  count: 3
  size: cx32
domain: example.com
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "k8zner.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Set CF_API_TOKEN for validation
	os.Setenv("CF_API_TOKEN", "test-token")
	defer os.Unsetenv("CF_API_TOKEN")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", cfg.Domain, "example.com")
	}
}

func TestLoad_MinimalDevConfig(t *testing.T) {
	t.Parallel()
	content := `
name: dev
region: fsn1
mode: dev
workers:
  count: 1
  size: cx23
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "k8zner.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Mode != ModeDev {
		t.Errorf("Mode = %q, want %q", cfg.Mode, ModeDev)
	}
	if cfg.ControlPlaneCount() != 1 {
		t.Errorf("ControlPlaneCount() = %d, want %d", cfg.ControlPlaneCount(), 1)
	}
	if cfg.LoadBalancerCount() != 1 {
		t.Errorf("LoadBalancerCount() = %d, want %d", cfg.LoadBalancerCount(), 1)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := Load("/nonexistent/path/k8zner.yaml")
	if err == nil {
		t.Error("Load() expected error for nonexistent file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	t.Parallel()
	content := `
name: my-cluster
region: [invalid yaml
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "k8zner.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() expected error for invalid YAML")
	}
}

func TestLoad_ValidationFailure(t *testing.T) {
	t.Parallel()
	content := `
name: INVALID_NAME
region: fsn1
mode: ha
workers:
  count: 3
  size: cx32
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "k8zner.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() expected validation error")
	}
}

func TestLoadWithoutValidation(t *testing.T) {
	t.Parallel()
	content := `
name: INVALID_NAME
region: invalid
mode: invalid
workers:
  count: 100
  size: invalid
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "k8zner.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// LoadWithoutValidation should not return validation errors
	cfg, err := LoadWithoutValidation(configPath)
	if err != nil {
		t.Fatalf("LoadWithoutValidation() error = %v", err)
	}

	if cfg.Name != "INVALID_NAME" {
		t.Errorf("Name = %q, want %q", cfg.Name, "INVALID_NAME")
	}
}

func TestLoadFromBytes(t *testing.T) {
	t.Parallel()
	content := []byte(`
name: test-cluster
region: hel1
mode: dev
workers:
  count: 2
  size: cx42
`)

	cfg, err := LoadFromBytes(content)
	if err != nil {
		t.Fatalf("LoadFromBytes() error = %v", err)
	}

	if cfg.Name != "test-cluster" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-cluster")
	}
	if cfg.Region != RegionHelsinki {
		t.Errorf("Region = %q, want %q", cfg.Region, RegionHelsinki)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	t.Parallel()
	path := DefaultConfigPath()
	if path == "" {
		t.Error("DefaultConfigPath() returned empty string")
	}
	if filepath.Base(path) != "k8zner.yaml" {
		t.Errorf("DefaultConfigPath() = %q, want filename k8zner.yaml", path)
	}
}
