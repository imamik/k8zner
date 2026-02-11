package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSpec_ValidConfig(t *testing.T) {
	t.Parallel()
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

	cfg, err := LoadSpec(configPath)
	if err != nil {
		t.Fatalf("LoadSpec() error = %v", err)
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

func TestLoadSpec_WithDomain(t *testing.T) {
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

	os.Setenv("CF_API_TOKEN", "test-token")
	defer os.Unsetenv("CF_API_TOKEN")

	cfg, err := LoadSpec(configPath)
	if err != nil {
		t.Fatalf("LoadSpec() error = %v", err)
	}

	if cfg.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", cfg.Domain, "example.com")
	}
}

func TestLoadSpec_MinimalDevConfig(t *testing.T) {
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

	cfg, err := LoadSpec(configPath)
	if err != nil {
		t.Fatalf("LoadSpec() error = %v", err)
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

func TestLoadSpec_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadSpec("/nonexistent/path/k8zner.yaml")
	if err == nil {
		t.Error("LoadSpec() expected error for nonexistent file")
	}
}

func TestLoadSpec_InvalidYAML(t *testing.T) {
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

	_, err := LoadSpec(configPath)
	if err == nil {
		t.Error("LoadSpec() expected error for invalid YAML")
	}
}

func TestLoadSpec_ValidationFailure(t *testing.T) {
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

	_, err := LoadSpec(configPath)
	if err == nil {
		t.Error("LoadSpec() expected validation error")
	}
}

func TestLoadSpecWithoutValidation(t *testing.T) {
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

	cfg, err := LoadSpecWithoutValidation(configPath)
	if err != nil {
		t.Fatalf("LoadSpecWithoutValidation() error = %v", err)
	}

	if cfg.Name != "INVALID_NAME" {
		t.Errorf("Name = %q, want %q", cfg.Name, "INVALID_NAME")
	}
}

func TestLoadSpecFromBytes(t *testing.T) {
	t.Parallel()
	content := []byte(`
name: test-cluster
region: hel1
mode: dev
workers:
  count: 2
  size: cx42
`)

	cfg, err := LoadSpecFromBytes(content)
	if err != nil {
		t.Fatalf("LoadSpecFromBytes() error = %v", err)
	}

	if cfg.Name != "test-cluster" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-cluster")
	}
	if cfg.Region != RegionHelsinki {
		t.Errorf("Region = %q, want %q", cfg.Region, RegionHelsinki)
	}
}

func TestLoadSpecFromBytes_ValidationError(t *testing.T) {
	t.Parallel()
	content := []byte(`
name: INVALID
region: fsn1
mode: ha
workers:
  count: 3
  size: cx32
`)
	_, err := LoadSpecFromBytes(content)
	if err == nil {
		t.Error("LoadSpecFromBytes() expected validation error for invalid name")
	}
	if !containsString(err.Error(), "validation failed") {
		t.Errorf("Expected validation error, got: %v", err)
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

func TestSaveSpec(t *testing.T) {
	t.Parallel()
	cfg := &Spec{
		Name:   "test-cluster",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: WorkerSpec{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "output.yaml")

	err := SaveSpec(cfg, savePath)
	if err != nil {
		t.Fatalf("SaveSpec() error = %v", err)
	}

	loaded, err := LoadSpecWithoutValidation(savePath)
	if err != nil {
		t.Fatalf("LoadSpecWithoutValidation() error = %v", err)
	}
	if loaded.Name != "test-cluster" {
		t.Errorf("Name = %q, want %q", loaded.Name, "test-cluster")
	}
}

func TestSaveSpec_InvalidPath(t *testing.T) {
	t.Parallel()
	cfg := &Spec{Name: "test"}
	err := SaveSpec(cfg, "/nonexistent/directory/k8zner.yaml")
	if err == nil {
		t.Error("SaveSpec() expected error for invalid path")
	}
}

func TestFindConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "k8zner.yaml")
	if err := os.WriteFile(configPath, []byte("name: test"), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}
	defer os.Chdir(originalDir)

	found, err := FindConfigFile()
	if err != nil {
		t.Fatalf("FindConfigFile() error = %v", err)
	}
	if found != configPath {
		t.Errorf("FindConfigFile() = %q, want %q", found, configPath)
	}
}

func TestFindConfigFile_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}
	defer os.Chdir(originalDir)

	_, err = FindConfigFile()
	if err == nil {
		t.Error("FindConfigFile() expected error when no config file exists")
	}
}

func TestLoadSpecFromBytes_InvalidYAML(t *testing.T) {
	t.Parallel()
	_, err := LoadSpecFromBytes([]byte("{{{{invalid yaml"))
	if err == nil {
		t.Fatal("LoadSpecFromBytes() expected error for invalid YAML, got nil")
	}
}

func TestLoadSpecFromBytesWithoutValidation_InvalidYAML(t *testing.T) {
	t.Parallel()
	_, err := LoadSpecFromBytesWithoutValidation([]byte("{{{{not valid yaml"))
	if err == nil {
		t.Fatal("LoadSpecFromBytesWithoutValidation() expected error for invalid YAML, got nil")
	}
}

func TestLoadSpecFromBytesWithoutValidation_Valid(t *testing.T) {
	t.Parallel()
	content := []byte(`
name: INVALID
region: fsn1
mode: ha
workers:
  count: 3
  size: cx32
`)
	cfg, err := LoadSpecFromBytesWithoutValidation(content)
	if err != nil {
		t.Fatalf("LoadSpecFromBytesWithoutValidation() error = %v", err)
	}
	if cfg.Name != "INVALID" {
		t.Errorf("Name = %q, want %q", cfg.Name, "INVALID")
	}
}

func TestFindConfigFile_InParentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	childDir := filepath.Join(tmpDir, "child")
	if err := os.Mkdir(childDir, 0755); err != nil {
		t.Fatalf("Failed to create child dir: %v", err)
	}

	configPath := filepath.Join(tmpDir, DefaultConfigFilename)
	if err := os.WriteFile(configPath, []byte("name: test"), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}
	if err := os.Chdir(childDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}
	defer os.Chdir(originalDir)

	found, err := FindConfigFile()
	if err != nil {
		t.Fatalf("FindConfigFile() error = %v", err)
	}
	if found != configPath {
		t.Errorf("FindConfigFile() = %q, want %q", found, configPath)
	}
}

func TestDefaultConfigPath_ReturnsJoinedPath(t *testing.T) {
	t.Parallel()
	path := DefaultConfigPath()

	if !filepath.IsAbs(path) {
		t.Errorf("DefaultConfigPath() = %q, expected absolute path", path)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}
	expected := filepath.Join(cwd, DefaultConfigFilename)
	if path != expected {
		t.Errorf("DefaultConfigPath() = %q, want %q", path, expected)
	}
}

func TestSaveSpec_RoundTrip(t *testing.T) {
	t.Parallel()
	cfg := &Spec{
		Name:   "round-trip",
		Region: RegionFalkenstein,
		Mode:   ModeDev,
		Workers: WorkerSpec{
			Count: 2,
			Size:  SizeCX42,
		},
	}

	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "roundtrip.yaml")

	if err := SaveSpec(cfg, savePath); err != nil {
		t.Fatalf("SaveSpec() error = %v", err)
	}

	loaded, err := LoadSpecWithoutValidation(savePath)
	if err != nil {
		t.Fatalf("LoadSpecWithoutValidation() error = %v", err)
	}

	if loaded.Name != cfg.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, cfg.Name)
	}
	if loaded.Region != cfg.Region {
		t.Errorf("Region = %q, want %q", loaded.Region, cfg.Region)
	}
	if loaded.Mode != cfg.Mode {
		t.Errorf("Mode = %q, want %q", loaded.Mode, cfg.Mode)
	}
	if loaded.Workers.Count != cfg.Workers.Count {
		t.Errorf("Workers.Count = %d, want %d", loaded.Workers.Count, cfg.Workers.Count)
	}
}

func TestLoadSpecWithoutValidation_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadSpecWithoutValidation("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("LoadSpecWithoutValidation() expected error for nonexistent file")
	}
}

func TestLoadSpecWithoutValidation_InvalidYAML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "bad.yaml")
	if err := os.WriteFile(configPath, []byte("{{{{not valid"), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := LoadSpecWithoutValidation(configPath)
	if err == nil {
		t.Error("LoadSpecWithoutValidation() expected error for invalid YAML")
	}
}
