package v2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	t.Parallel()
	// Create a temporary config file

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

func TestLoadFromBytes_ValidationError(t *testing.T) {
	t.Parallel()
	// Valid YAML but invalid config (uppercase name)
	content := []byte(`
name: INVALID
region: fsn1
mode: ha
workers:
  count: 3
  size: cx32
`)
	_, err := LoadFromBytes(content)
	if err == nil {
		t.Error("LoadFromBytes() expected validation error for invalid name")
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

func TestSave(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Name:   "test-cluster",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "output.yaml")

	err := Save(cfg, savePath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file was written and can be loaded back
	loaded, err := LoadWithoutValidation(savePath)
	if err != nil {
		t.Fatalf("LoadWithoutValidation() error = %v", err)
	}
	if loaded.Name != "test-cluster" {
		t.Errorf("Name = %q, want %q", loaded.Name, "test-cluster")
	}
}

func TestSave_InvalidPath(t *testing.T) {
	t.Parallel()
	cfg := &Config{Name: "test"}
	err := Save(cfg, "/nonexistent/directory/k8zner.yaml")
	if err == nil {
		t.Error("Save() expected error for invalid path")
	}
}

func TestFindConfigFile(t *testing.T) {
	// Create a temp directory with a config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "k8zner.yaml")
	if err := os.WriteFile(configPath, []byte("name: test"), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Change to temp dir for the test
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

// TestLoadFromBytes_InvalidYAML tests the parseConfig error branch inside LoadFromBytes
// (lines 42-43 in loader.go). When invalid YAML is passed, parseConfig returns an error
// which LoadFromBytes should propagate.
func TestLoadFromBytes_InvalidYAML(t *testing.T) {
	t.Parallel()
	_, err := LoadFromBytes([]byte("{{{{invalid yaml"))
	if err == nil {
		t.Fatal("LoadFromBytes() expected error for invalid YAML, got nil")
	}
}

// TestLoadFromBytesWithoutValidation_InvalidYAML tests the parseConfig error path
// through LoadFromBytesWithoutValidation.
func TestLoadFromBytesWithoutValidation_InvalidYAML(t *testing.T) {
	t.Parallel()
	_, err := LoadFromBytesWithoutValidation([]byte("{{{{not valid yaml"))
	if err == nil {
		t.Fatal("LoadFromBytesWithoutValidation() expected error for invalid YAML, got nil")
	}
}

// TestLoadFromBytesWithoutValidation_Valid tests LoadFromBytesWithoutValidation
// with valid YAML that would fail validation (uppercase name).
func TestLoadFromBytesWithoutValidation_Valid(t *testing.T) {
	t.Parallel()
	content := []byte(`
name: INVALID
region: fsn1
mode: ha
workers:
  count: 3
  size: cx32
`)
	cfg, err := LoadFromBytesWithoutValidation(content)
	if err != nil {
		t.Fatalf("LoadFromBytesWithoutValidation() error = %v", err)
	}
	if cfg.Name != "INVALID" {
		t.Errorf("Name = %q, want %q", cfg.Name, "INVALID")
	}
}

// TestFindConfigFile_InParentDirectory tests FindConfigFile walking up the directory tree
// to find a config file in a parent directory (lines 92-106 in loader.go).
func TestFindConfigFile_InParentDirectory(t *testing.T) {
	// Create a temp directory structure: parent/child/
	// Place config file in parent, chdir to child
	tmpDir := t.TempDir()
	childDir := filepath.Join(tmpDir, "child")
	if err := os.Mkdir(childDir, 0755); err != nil {
		t.Fatalf("Failed to create child dir: %v", err)
	}

	// Write config file in parent (tmpDir)
	configPath := filepath.Join(tmpDir, DefaultConfigFilename)
	if err := os.WriteFile(configPath, []byte("name: test"), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Change to child directory
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

// TestDefaultConfigPath_ReturnsJoinedPath verifies that DefaultConfigPath returns
// the config filename joined with the current working directory.
func TestDefaultConfigPath_ReturnsJoinedPath(t *testing.T) {
	t.Parallel()
	path := DefaultConfigPath()

	// The path should be an absolute path ending with the default config filename
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

// TestSave_RoundTrip verifies that Save followed by Load produces the same configuration.
func TestSave_RoundTrip(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Name:   "round-trip",
		Region: RegionFalkenstein,
		Mode:   ModeDev,
		Workers: Worker{
			Count: 2,
			Size:  SizeCX42,
		},
	}

	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "roundtrip.yaml")

	if err := Save(cfg, savePath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := LoadWithoutValidation(savePath)
	if err != nil {
		t.Fatalf("LoadWithoutValidation() error = %v", err)
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

// TestLoadWithoutValidation_FileNotFound tests the error path when the file does not exist.
func TestLoadWithoutValidation_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadWithoutValidation("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("LoadWithoutValidation() expected error for nonexistent file")
	}
}

// TestLoadWithoutValidation_InvalidYAML tests the parseConfig error path
// through LoadWithoutValidation when the file contains invalid YAML.
func TestLoadWithoutValidation_InvalidYAML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "bad.yaml")
	if err := os.WriteFile(configPath, []byte("{{{{not valid"), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := LoadWithoutValidation(configPath)
	if err == nil {
		t.Error("LoadWithoutValidation() expected error for invalid YAML")
	}
}
