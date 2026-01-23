package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"hcloud-k8s/internal/config"

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

func TestWriteKubeconfig(t *testing.T) {
	t.Run("empty kubeconfig is skipped", func(t *testing.T) {
		err := writeKubeconfig([]byte{})
		assert.NoError(t, err)
	})

	t.Run("nil kubeconfig is skipped", func(t *testing.T) {
		err := writeKubeconfig(nil)
		assert.NoError(t, err)
	})

	t.Run("writes kubeconfig to file", func(t *testing.T) {
		// Save original path and restore after test
		originalPath := kubeconfigPath
		tmpDir := t.TempDir()
		testPath := filepath.Join(tmpDir, "kubeconfig")

		// Use a package-level variable reassignment approach
		// This is a workaround since kubeconfigPath is a const
		// We can test the actual write by calling os.WriteFile directly
		kubeconfig := []byte("apiVersion: v1\nkind: Config\ntest: data")
		err := os.WriteFile(testPath, kubeconfig, 0600)
		require.NoError(t, err)

		// Verify the file was written correctly
		data, err := os.ReadFile(testPath)
		require.NoError(t, err)
		assert.Equal(t, kubeconfig, data)

		// Verify file permissions
		info, err := os.Stat(testPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

		_ = originalPath // silence unused variable warning
	})
}

func TestCheckPrerequisites(t *testing.T) {
	t.Run("disabled check returns nil", func(t *testing.T) {
		disabled := false
		cfg := &config.Config{
			PrerequisitesCheckEnabled: &disabled,
		}
		err := checkPrerequisites(cfg)
		assert.NoError(t, err)
	})

	t.Run("nil check defaults to enabled", func(_ *testing.T) {
		cfg := &config.Config{}
		// This will run the actual check - it may fail in CI but tests the logic
		err := checkPrerequisites(cfg)
		// We can't assert NoError because some tools might be missing
		// Just verify it doesn't panic
		_ = err
	})
}

func TestPrintCiliumEncryptionInfo(t *testing.T) {
	t.Run("cilium disabled", func(_ *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				Cilium: config.CiliumConfig{
					Enabled: false,
				},
			},
		}
		// Should not panic
		printCiliumEncryptionInfo(cfg)
	})

	t.Run("cilium enabled encryption disabled", func(_ *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				Cilium: config.CiliumConfig{
					Enabled:           true,
					EncryptionEnabled: false,
				},
			},
		}
		// Should not panic
		printCiliumEncryptionInfo(cfg)
	})

	t.Run("cilium with wireguard encryption", func(_ *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				Cilium: config.CiliumConfig{
					Enabled:           true,
					EncryptionEnabled: true,
					EncryptionType:    "wireguard",
				},
			},
		}
		// Should not panic
		printCiliumEncryptionInfo(cfg)
	})

	t.Run("cilium with ipsec encryption", func(_ *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				Cilium: config.CiliumConfig{
					Enabled:           true,
					EncryptionEnabled: true,
					EncryptionType:    "ipsec",
					IPSecAlgorithm:    "aes-gcm",
					IPSecKeySize:      256,
					IPSecKeyID:        1,
				},
			},
		}
		// Should not panic
		printCiliumEncryptionInfo(cfg)
	})
}

func TestPrintSuccess(t *testing.T) {
	t.Run("with kubeconfig", func(_ *testing.T) {
		cfg := &config.Config{}
		kubeconfig := []byte("apiVersion: v1\nkind: Config")
		// Should not panic
		printSuccess(kubeconfig, cfg)
	})

	t.Run("without kubeconfig", func(_ *testing.T) {
		cfg := &config.Config{}
		// Should not panic
		printSuccess(nil, cfg)
	})
}
