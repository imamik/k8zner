package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCostCommand(t *testing.T) {
	cmd := Cost()

	if cmd.Use != "cost" {
		t.Errorf("Use = %q, want %q", cmd.Use, "cost")
	}

	if cmd.Short == "" {
		t.Error("Short description is empty")
	}
}

func TestCostCommand_WithConfig(t *testing.T) {
	// Create a temporary config file
	content := `
name: test-cluster
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

	// Change to temp dir so FindConfigFile works
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	cmd := Cost()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"-f", configPath})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestCostCommand_JSONOutput(t *testing.T) {
	content := `
name: test-json
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

	cmd := Cost()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"-f", configPath, "--json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestCostCommand_CompactOutput(t *testing.T) {
	content := `
name: test-compact
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

	cmd := Cost()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"-f", configPath, "--compact"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestCostCommand_NoConfig(t *testing.T) {
	// Use a directory with no config file
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	cmd := Cost()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("Execute() expected error when no config file exists")
	}
}
