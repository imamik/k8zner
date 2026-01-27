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
location: "nbg1"
network:
  ipv4_cidr: "10.0.0.0/16"
control_plane:
  nodepools:
    - name: "control-plane-1"
      type: "cx11"
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

func TestLoadFile_OIDCDefaults(t *testing.T) {
	content := `
cluster_name: "test-cluster"
hcloud_token: "token"
location: "nbg1"
network:
  ipv4_cidr: "10.0.0.0/16"
control_plane:
  nodepools:
    - name: "cp"
      type: "cx11"
      count: 1
kubernetes:
  oidc:
    enabled: true
    issuer_url: "https://accounts.google.com"
    client_id: "my-client-id"
`
	tmpfile, err := os.CreateTemp("", "config-oidc-*.yaml")
	assert.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpfile.Name())
	}()

	_, err = tmpfile.Write([]byte(content))
	assert.NoError(t, err)
	_ = tmpfile.Close()

	cfg, err := LoadFile(tmpfile.Name())
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Check OIDC defaults were applied
	assert.Equal(t, "sub", cfg.Kubernetes.OIDC.UsernameClaim)
	assert.Equal(t, "groups", cfg.Kubernetes.OIDC.GroupsClaim)
}

func TestLoadFile_TalosCCMVersionDefault(t *testing.T) {
	content := `
cluster_name: "test-cluster"
hcloud_token: "token"
location: "nbg1"
network:
  ipv4_cidr: "10.0.0.0/16"
control_plane:
  nodepools:
    - name: "cp"
      type: "cx11"
      count: 1
addons:
  talos_ccm:
    enabled: true
`
	tmpfile, err := os.CreateTemp("", "config-talosccm-*.yaml")
	assert.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpfile.Name())
	}()

	_, err = tmpfile.Write([]byte(content))
	assert.NoError(t, err)
	_ = tmpfile.Close()

	cfg, err := LoadFile(tmpfile.Name())
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Check TalosCCM version default was applied
	assert.Equal(t, "v1.11.0", cfg.Addons.TalosCCM.Version)
}

func TestLoadFile_IngressLoadBalancerPoolCountDefault(t *testing.T) {
	content := `
cluster_name: "test-cluster"
hcloud_token: "token"
location: "nbg1"
network:
  ipv4_cidr: "10.0.0.0/16"
control_plane:
  nodepools:
    - name: "cp"
      type: "cx11"
      count: 1
ingress_load_balancer_pools:
  - name: "ingress"
    location: "nbg1"
`
	tmpfile, err := os.CreateTemp("", "config-ilb-*.yaml")
	assert.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpfile.Name())
	}()

	_, err = tmpfile.Write([]byte(content))
	assert.NoError(t, err)
	_ = tmpfile.Close()

	cfg, err := LoadFile(tmpfile.Name())
	assert.NoError(t, err)
	if assert.NotNil(t, cfg) && assert.NotEmpty(t, cfg.IngressLoadBalancerPools) {
		// Check IngressLoadBalancerPool count default was applied
		assert.Equal(t, 1, cfg.IngressLoadBalancerPools[0].Count)
	}
}

func TestLoadFile_ImageBuilderDefaults(t *testing.T) {
	content := `
cluster_name: "test-cluster"
hcloud_token: "token"
location: "nbg1"
network:
  ipv4_cidr: "10.0.0.0/16"
control_plane:
  nodepools:
    - name: "cp"
      type: "cx11"
      count: 1
`
	tmpfile, err := os.CreateTemp("", "config-imagebuilder-*.yaml")
	assert.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpfile.Name())
	}()

	_, err = tmpfile.Write([]byte(content))
	assert.NoError(t, err)
	_ = tmpfile.Close()

	cfg, err := LoadFile(tmpfile.Name())
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Check image builder defaults were applied
	assert.Equal(t, "cpx11", cfg.Talos.ImageBuilder.AMD64.ServerType)
	assert.Equal(t, "ash", cfg.Talos.ImageBuilder.AMD64.ServerLocation)
	assert.Equal(t, "cax11", cfg.Talos.ImageBuilder.ARM64.ServerType)
	assert.Equal(t, "nbg1", cfg.Talos.ImageBuilder.ARM64.ServerLocation)
}

func TestLoadFile_NonExistentFile(t *testing.T) {
	_, err := LoadFile("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestLoadFile_InvalidYAML(t *testing.T) {
	content := `invalid: yaml: content: [`
	tmpfile, err := os.CreateTemp("", "config-invalid-*.yaml")
	assert.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpfile.Name())
	}()

	_, err = tmpfile.Write([]byte(content))
	assert.NoError(t, err)
	_ = tmpfile.Close()

	_, err = LoadFile(tmpfile.Name())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal yaml")
}
