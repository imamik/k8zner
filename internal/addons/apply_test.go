package addons

import (
	"context"
	"os"
	"testing"

	"hcloud-k8s/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestApply_EmptyKubeconfig(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{Enabled: true},
		},
	}

	err := Apply(context.Background(), cfg, nil, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig is required")
}

func TestApply_NoCCMConfigured(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{Enabled: false},
		},
	}

	// Even with valid kubeconfig, if CCM is disabled, Apply should succeed
	// (assuming no other addons are configured)
	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters: []
contexts: []
current-context: ""
users: []`)

	err := Apply(context.Background(), cfg, kubeconfig, 1)
	// Should succeed since CCM is disabled and nothing to install
	assert.NoError(t, err)
}

func TestWriteTempKubeconfig(t *testing.T) {
	kubeconfig := []byte("test kubeconfig content")

	path, err := writeTempKubeconfig(kubeconfig)
	assert.NoError(t, err)
	assert.NotEmpty(t, path)

	// Cleanup
	defer func() { _ = os.Remove(path) }()

	// Verify file exists and has correct content
	content, err := os.ReadFile(path) //nolint:gosec // G304: path is from our own test function
	assert.NoError(t, err)
	assert.Equal(t, kubeconfig, content)
}

func TestWriteTempKubeconfig_EmptyContent(t *testing.T) {
	path, err := writeTempKubeconfig([]byte{})
	assert.NoError(t, err)

	// Cleanup
	defer func() { _ = os.Remove(path) }()

	// File should exist but be empty
	content, err := os.ReadFile(path) //nolint:gosec // G304: path is from our own test function
	assert.NoError(t, err)
	assert.Empty(t, content)
}
