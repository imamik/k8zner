package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_EmptyPath(t *testing.T) {
	_, err := loadConfig("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config file is required")
}

func TestLoadConfig_NonExistentFile(t *testing.T) {
	_, err := loadConfig("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestLoadConfig_ValidFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
cluster_name: test-cluster
hcloud_token: test-token
location: nbg1
network:
  ipv4_cidr: "10.0.0.0/16"
  zone: "eu-central"
control_plane:
  nodepools:
    - name: control-plane
      type: cx21
      count: 1
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	require.NoError(t, err)

	cfg, err := loadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "test-cluster", cfg.ClusterName)
	assert.Equal(t, "nbg1", cfg.Location)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	// Write invalid YAML
	err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0600)
	require.NoError(t, err)

	_, err = loadConfig(configPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestInitializeClient(t *testing.T) {
	// Test that initializeClient returns a non-nil client
	client := initializeClient()
	assert.NotNil(t, client)
}
