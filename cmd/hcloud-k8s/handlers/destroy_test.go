package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			name: "missing config file",
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
			errorContains: "invalid configuration",
		},
		{
			name: "missing hcloud token",
			configContent: `
cluster_name: test-cluster
hcloud_token: ""
`,
			expectError:   true,
			errorContains: "invalid configuration",
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

	// Note: This will fail with "failed to create Hetzner Cloud client"
	// because we're using a fake token. In a full test, we'd mock the client.
	err = Destroy(context.Background(), configPath)
	if err != nil {
		// Expected to fail with API error or connection error
		assert.Contains(t, err.Error(), "Hetzner Cloud client")
	}
}
