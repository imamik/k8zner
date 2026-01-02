package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadFile(t *testing.T) {
	// Create temporary config file
	content := `
cluster_name: "test-cluster"
hcloud_token: "token"
control_plane:
  endpoint: "https://1.2.3.4:6443"
talos:
  version: "v1.7.0"
  k8s_version: "v1.30.0"
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	assert.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write([]byte(content))
	assert.NoError(t, err)
	tmpfile.Close()

	// Test LoadFile
	cfg, err := LoadFile(tmpfile.Name())
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	assert.Equal(t, "test-cluster", cfg.ClusterName)
	assert.Equal(t, "https://1.2.3.4:6443", cfg.ControlPlane.Endpoint)
	assert.Equal(t, "v1.7.0", cfg.Talos.Version)
}
