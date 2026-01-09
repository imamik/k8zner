package talos

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestGenerateControlPlaneConfig(t *testing.T) {
	clusterName := "test-cluster"
	k8sVersion := "v1.30.0"
	talosVersion := "v1.7.0"
	endpoint := "https://1.2.3.4:6443"

	sb, err := NewSecrets(talosVersion)
	assert.NoError(t, err)

	gen := NewGenerator(clusterName, k8sVersion, talosVersion, endpoint, sb)
	assert.NotNil(t, gen)

	san := []string{"api.test-cluster.com"}
	hostname := "test-control-plane-1"
	configBytes, err := gen.GenerateControlPlaneConfig(san, hostname)
	assert.NoError(t, err)
	assert.NotEmpty(t, configBytes)

	// Validate it is valid YAML
	var result map[string]interface{}
	err = yaml.Unmarshal(configBytes, &result)
	assert.NoError(t, err)

	// Check basic fields
	machine := result["machine"].(map[string]interface{})
	assert.Equal(t, "controlplane", machine["type"])

	// Check patches
	cluster := result["cluster"].(map[string]interface{})
	assert.Equal(t, true, cluster["externalCloudProvider"].(map[string]interface{})["enabled"])

	network := machine["network"].(map[string]interface{})
	assert.Equal(t, hostname, network["hostname"])
}

func TestGenerateWorkerConfig(t *testing.T) {
	clusterName := "test-cluster"
	k8sVersion := "v1.30.0"
	talosVersion := "v1.7.0"
	endpoint := "https://1.2.3.4:6443"

	sb, err := NewSecrets(talosVersion)
	assert.NoError(t, err)

	gen := NewGenerator(clusterName, k8sVersion, talosVersion, endpoint, sb)
	assert.NotNil(t, gen)

	hostname := "test-worker-1"
	configBytes, err := gen.GenerateWorkerConfig(hostname)
	assert.NoError(t, err)
	assert.NotEmpty(t, configBytes)

	// Validate it is valid YAML
	var result map[string]interface{}
	err = yaml.Unmarshal(configBytes, &result)
	assert.NoError(t, err)

	// Check basic fields
	machine := result["machine"].(map[string]interface{})
	assert.Equal(t, "worker", machine["type"])

	// Check patches
	network := machine["network"].(map[string]interface{})
	assert.Equal(t, hostname, network["hostname"])
}

func TestSecrets(t *testing.T) {
	talosVersion := "v1.7.0"
	tempDir := t.TempDir()
	secretsFile := filepath.Join(tempDir, "secrets.yaml")

	// 1. NewSecrets
	sb, err := NewSecrets(talosVersion)
	assert.NoError(t, err)
	assert.NotNil(t, sb)

	// 2. SaveSecrets
	err = SaveSecrets(secretsFile, sb)
	assert.NoError(t, err)

	// 3. LoadSecrets
	loaded, err := LoadSecrets(secretsFile)
	assert.NoError(t, err)
	assert.NotNil(t, loaded)

	// Verify Load/Save works by checking if LoadBundle succeeds after SaveBundle
	assert.NotNil(t, loaded)
	// Clock is reset in LoadSecrets, so we can check if it's set
	assert.NotNil(t, loaded.Clock)
}
