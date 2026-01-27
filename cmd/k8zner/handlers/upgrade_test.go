package handlers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/provisioning/upgrade"
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

func TestUpgrade_WithInjection(t *testing.T) {
	saveAndRestoreFactories(t)

	validConfig := &config.Config{
		ClusterName: "test-cluster",
		HCloudToken: "test-token",
		Location:    "nbg1",
		Talos:       config.TalosConfig{Version: "v1.9.0"},
		Kubernetes:  config.KubernetesConfig{Version: "v1.32.0"},
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp", ServerType: "cpx21", Count: 1},
			},
		},
	}

	t.Run("success flow", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			return validConfig, nil
		}

		newInfraClient = func(_ string) hcloud.InfrastructureManager {
			return &hcloud.MockClient{}
		}

		loadSecrets = func(_ string) (*secrets.Bundle, error) {
			return &secrets.Bundle{}, nil
		}

		newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
			return &mockTalosProducer{}
		}

		newProvisioningContext = func(_ context.Context, _ *config.Config, _ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer) *provisioning.Context {
			return &provisioning.Context{}
		}

		newUpgradeProvisioner = func(_ upgrade.ProvisionerOptions) Provisioner {
			return &mockUpgradeProvisioner{}
		}

		opts := UpgradeOptions{
			ConfigPath: "config.yaml",
		}

		err := Upgrade(context.Background(), opts)
		require.NoError(t, err)
	})

	t.Run("success with dry run", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			return validConfig, nil
		}

		newInfraClient = func(_ string) hcloud.InfrastructureManager {
			return &hcloud.MockClient{}
		}

		loadSecrets = func(_ string) (*secrets.Bundle, error) {
			return &secrets.Bundle{}, nil
		}

		newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
			return &mockTalosProducer{}
		}

		newProvisioningContext = func(_ context.Context, _ *config.Config, _ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer) *provisioning.Context {
			return &provisioning.Context{}
		}

		var capturedOpts upgrade.ProvisionerOptions
		newUpgradeProvisioner = func(opts upgrade.ProvisionerOptions) Provisioner {
			capturedOpts = opts
			return &mockUpgradeProvisioner{}
		}

		opts := UpgradeOptions{
			ConfigPath: "config.yaml",
			DryRun:     true,
		}

		err := Upgrade(context.Background(), opts)
		require.NoError(t, err)
		assert.True(t, capturedOpts.DryRun)
	})

	t.Run("config load error", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			return nil, errors.New("file not found")
		}

		opts := UpgradeOptions{ConfigPath: "missing.yaml"}
		err := Upgrade(context.Background(), opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("config validation error", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			return &config.Config{ClusterName: ""}, nil
		}

		opts := UpgradeOptions{ConfigPath: "config.yaml"}
		err := Upgrade(context.Background(), opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid configuration")
	})

	t.Run("secrets load error", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			return validConfig, nil
		}

		newInfraClient = func(_ string) hcloud.InfrastructureManager {
			return &hcloud.MockClient{}
		}

		loadSecrets = func(_ string) (*secrets.Bundle, error) {
			return nil, errors.New("secrets file not found")
		}

		opts := UpgradeOptions{ConfigPath: "config.yaml"}
		err := Upgrade(context.Background(), opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load Talos secrets")
	})

	t.Run("upgrade provisioner error", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			return validConfig, nil
		}

		newInfraClient = func(_ string) hcloud.InfrastructureManager {
			return &hcloud.MockClient{}
		}

		loadSecrets = func(_ string) (*secrets.Bundle, error) {
			return &secrets.Bundle{}, nil
		}

		newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
			return &mockTalosProducer{}
		}

		newProvisioningContext = func(_ context.Context, _ *config.Config, _ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer) *provisioning.Context {
			return &provisioning.Context{}
		}

		newUpgradeProvisioner = func(_ upgrade.ProvisionerOptions) Provisioner {
			return &mockUpgradeProvisioner{err: errors.New("upgrade failed: node unreachable")}
		}

		opts := UpgradeOptions{ConfigPath: "config.yaml"}
		err := Upgrade(context.Background(), opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upgrade failed")
	})

	t.Run("k8s version override", func(t *testing.T) {
		var capturedConfig *config.Config
		loadConfigFile = func(_ string) (*config.Config, error) {
			cfg := *validConfig // Copy
			capturedConfig = &cfg
			return &cfg, nil
		}

		newInfraClient = func(_ string) hcloud.InfrastructureManager {
			return &hcloud.MockClient{}
		}

		loadSecrets = func(_ string) (*secrets.Bundle, error) {
			return &secrets.Bundle{}, nil
		}

		newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
			return &mockTalosProducer{}
		}

		newProvisioningContext = func(_ context.Context, _ *config.Config, _ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer) *provisioning.Context {
			return &provisioning.Context{}
		}

		newUpgradeProvisioner = func(_ upgrade.ProvisionerOptions) Provisioner {
			return &mockUpgradeProvisioner{}
		}

		opts := UpgradeOptions{
			ConfigPath: "config.yaml",
			K8sVersion: "v1.33.0",
		}

		err := Upgrade(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, "v1.33.0", capturedConfig.Kubernetes.Version)
	})
}

// mockUpgradeProvisioner implements the Provisioner interface for testing.
type mockUpgradeProvisioner struct {
	err error
}

func (m *mockUpgradeProvisioner) Provision(_ *provisioning.Context) error {
	return m.err
}
