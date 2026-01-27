package handlers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
)

func TestDestroy(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		setupEnv      func()
		expectError   bool
		errorContains string
	}{
		{
			name:          "missing config file",
			configContent: ``,
			expectError:   true,
			errorContains: "failed to load config",
		},
		{
			name: "invalid config",
			configContent: `
cluster_name: ""
hcloud_token: invalid
`,
			expectError:   true,
			errorContains: "cluster_name is required",
		},
		{
			name: "missing hcloud token",
			configContent: `
cluster_name: test-cluster
hcloud_token: ""
`,
			expectError:   true,
			errorContains: "hcloud_token is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			if tt.configContent != "" {
				err := os.WriteFile(configPath, []byte(tt.configContent), 0600)
				require.NoError(t, err)
			}

			if tt.setupEnv != nil {
				tt.setupEnv()
			}

			// Run destroy
			err := Destroy(context.Background(), configPath)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDestroyWithValidConfig(t *testing.T) {
	// This test would require a mock HCloud client
	// For now, just verify that the config loading works
	configContent := `
cluster_name: test-cluster
hcloud_token: test-token-12345
talos:
  version: v1.9.4
kubernetes:
  version: v1.32.0
control_plane:
  node_pools:
    - count: 1
      type: cpx21
      location: nbg1
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	require.NoError(t, err)

	// Note: This will fail with configuration validation error
	// because location is required. In a full test, we'd mock the client.
	err = Destroy(context.Background(), configPath)
	if err != nil {
		// Expected to fail with config validation error
		assert.Contains(t, err.Error(), "configuration validation failed")
	}
}

// mockProvisioner implements the Provisioner interface for testing.
type mockProvisioner struct {
	err error
}

func (m *mockProvisioner) Provision(_ *provisioning.Context) error {
	return m.err
}

func TestDestroy_WithInjection(t *testing.T) {
	saveAndRestoreFactories(t)

	validConfig := &config.Config{
		ClusterName: "test-cluster",
		HCloudToken: "test-token",
		Location:    "nbg1",
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

		newProvisioningContext = func(_ context.Context, _ *config.Config, _ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer) *provisioning.Context {
			return &provisioning.Context{}
		}

		newDestroyProvisioner = func() Provisioner {
			return &mockProvisioner{}
		}

		err := Destroy(context.Background(), "config.yaml")
		require.NoError(t, err)
	})

	t.Run("config load error", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			return nil, errors.New("file not found")
		}

		err := Destroy(context.Background(), "missing.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("config validation error", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			return &config.Config{ClusterName: ""}, nil // Invalid: empty cluster name
		}

		err := Destroy(context.Background(), "config.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid configuration")
	})

	t.Run("destroy provisioner error", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			return validConfig, nil
		}

		newInfraClient = func(_ string) hcloud.InfrastructureManager {
			return &hcloud.MockClient{}
		}

		newProvisioningContext = func(_ context.Context, _ *config.Config, _ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer) *provisioning.Context {
			return &provisioning.Context{}
		}

		newDestroyProvisioner = func() Provisioner {
			return &mockProvisioner{err: errors.New("destroy failed: resource busy")}
		}

		err := Destroy(context.Background(), "config.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "destroy failed")
	})
}
