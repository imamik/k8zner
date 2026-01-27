package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpgradeOptions(t *testing.T) {
	opts := UpgradeOptions{
		ConfigPath:      "/path/to/config.yaml",
		DryRun:          true,
		SkipHealthCheck: true,
		K8sVersion:      "v1.31.0",
	}

	assert.Equal(t, "/path/to/config.yaml", opts.ConfigPath)
	assert.True(t, opts.DryRun)
	assert.True(t, opts.SkipHealthCheck)
	assert.Equal(t, "v1.31.0", opts.K8sVersion)
}

func TestUpgradeOptions_DefaultValues(t *testing.T) {
	opts := UpgradeOptions{}

	assert.Empty(t, opts.ConfigPath)
	assert.False(t, opts.DryRun)
	assert.False(t, opts.SkipHealthCheck)
	assert.Empty(t, opts.K8sVersion)
}

func TestUpgrade_InvalidConfigPath(t *testing.T) {
	opts := UpgradeOptions{
		ConfigPath: "/nonexistent/path/config.yaml",
	}

	err := Upgrade(t.Context(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestUpgrade_InvalidYAMLConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	// Write invalid YAML
	err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0600)
	require.NoError(t, err)

	opts := UpgradeOptions{
		ConfigPath: configPath,
	}

	err = Upgrade(t.Context(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestUpgrade_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write config that will fail validation (missing required fields)
	configContent := `
cluster_name: ""
location: ""
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	require.NoError(t, err)

	opts := UpgradeOptions{
		ConfigPath: configPath,
	}

	err = Upgrade(t.Context(), opts)
	require.Error(t, err)
	// Either fails on validation or config load
	assert.True(t,
		strings.Contains(err.Error(), "invalid configuration") ||
			strings.Contains(err.Error(), "failed to load config"),
	)
}

func TestUpgrade_MissingSecretsFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write a minimal valid config
	configContent := `
cluster_name: test-cluster
hcloud_token: test-token
location: nbg1
network:
  ipv4_cidr: "10.0.0.0/16"
control_plane:
  nodepools:
    - name: control-plane
      type: cx21
      count: 1
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	require.NoError(t, err)

	opts := UpgradeOptions{
		ConfigPath: configPath,
	}

	// This will fail because secrets file doesn't exist
	err = Upgrade(t.Context(), opts)
	require.Error(t, err)
	// Should fail on either secrets or validation
	assert.True(t,
		strings.Contains(err.Error(), "secrets") ||
			strings.Contains(err.Error(), "invalid configuration") ||
			strings.Contains(err.Error(), "failed to load"),
	)
}

func TestUpgrade_K8sVersionOverride(t *testing.T) {
	// This test verifies the opts structure supports version override
	opts := UpgradeOptions{
		ConfigPath: "/path/to/config.yaml",
		K8sVersion: "v1.32.0",
	}

	assert.Equal(t, "v1.32.0", opts.K8sVersion)
}
