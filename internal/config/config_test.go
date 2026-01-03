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
network:
  ipv4_cidr: "10.0.0.0/16"
control_plane:
  nodepools:
    - name: "control-plane-1"
      count: 3
talos:
  version: "v1.7.0"
kubernetes:
  version: "v1.30.0"
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	assert.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpfile.Name())
	}()

	_, err = tmpfile.Write([]byte(content))
	assert.NoError(t, err)
	_ = tmpfile.Close()

	// Test LoadFile
	cfg, err := LoadFile(tmpfile.Name())
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	assert.Equal(t, "test-cluster", cfg.ClusterName)
	assert.Equal(t, "10.0.0.0/16", cfg.Network.IPv4CIDR)
	assert.Equal(t, "v1.7.0", cfg.Talos.Version)
	assert.Equal(t, "v1.30.0", cfg.Kubernetes.Version)
	assert.Equal(t, 1, len(cfg.ControlPlane.NodePools))
}
