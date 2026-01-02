package talos

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"fmt"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestGenerateControlPlaneConfig(t *testing.T) {
	clusterName := "test-cluster"
	k8sVersion := "v1.30.0"
	talosVersion := "v1.7.0"
	endpoint := "https://1.2.3.4:6443"

	// Use temp file for secrets (ensure it doesn't exist)
	secretsFile := filepath.Join(os.TempDir(), fmt.Sprintf("secrets-cp-%d.yaml", time.Now().UnixNano()))
	defer os.Remove(secretsFile)

	gen, err := NewConfigGenerator(clusterName, k8sVersion, talosVersion, endpoint, secretsFile)
	assert.NoError(t, err)
	assert.NotNil(t, gen)

	san := []string{"api.test-cluster.com"}
	configBytes, err := gen.GenerateControlPlaneConfig(san)
	assert.NoError(t, err)
	assert.NotEmpty(t, configBytes)

	// Validate it is valid YAML
	var result map[string]interface{}
	err = yaml.Unmarshal(configBytes, &result)
	assert.NoError(t, err)

	// Check basic fields
	machine := result["machine"].(map[string]interface{})
	assert.Equal(t, "controlplane", machine["type"])

	// Verify secrets file was created
	_, err = os.Stat(secretsFile)
	assert.NoError(t, err)
}

func TestGenerateWorkerConfig(t *testing.T) {
	clusterName := "test-cluster"
	k8sVersion := "v1.30.0"
	talosVersion := "v1.7.0"
	endpoint := "https://1.2.3.4:6443"

	// Use temp file for secrets (ensure it doesn't exist)
	secretsFile := filepath.Join(os.TempDir(), fmt.Sprintf("secrets-worker-%d.yaml", time.Now().UnixNano()))
	defer os.Remove(secretsFile)

	gen, err := NewConfigGenerator(clusterName, k8sVersion, talosVersion, endpoint, secretsFile)
	assert.NoError(t, err)
	assert.NotNil(t, gen)

	configBytes, err := gen.GenerateWorkerConfig()
	assert.NoError(t, err)
	assert.NotEmpty(t, configBytes)

	// Validate it is valid YAML
	var result map[string]interface{}
	err = yaml.Unmarshal(configBytes, &result)
	assert.NoError(t, err)

	// Check basic fields
	machine := result["machine"].(map[string]interface{})
	assert.Equal(t, "worker", machine["type"])

	// Verify secrets file was created
	_, err = os.Stat(secretsFile)
	assert.NoError(t, err)
}
