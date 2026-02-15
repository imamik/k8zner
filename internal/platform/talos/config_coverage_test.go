package talos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGetClientConfig(t *testing.T) {
	t.Parallel()

	talosVersion := "v1.7.0"
	sb, err := NewSecrets(talosVersion)
	require.NoError(t, err)

	gen := NewGenerator("test-cluster", "v1.30.0", talosVersion, "https://1.2.3.4:6443", sb)
	require.NotNil(t, gen)

	configBytes, err := gen.GetClientConfig()
	require.NoError(t, err)
	assert.NotEmpty(t, configBytes)

	// Should be valid YAML
	var result map[string]interface{}
	err = yaml.Unmarshal(configBytes, &result)
	require.NoError(t, err)

	// Should contain context information
	assert.NotNil(t, result["contexts"])
}

func TestGetClientConfig_InvalidTalosVersion(t *testing.T) {
	t.Parallel()

	// Use a valid secrets bundle but invalid talos version for GetClientConfig
	talosVersion := "v1.7.0"
	sb, err := NewSecrets(talosVersion)
	require.NoError(t, err)

	gen := &Generator{
		clusterName:       "test-cluster",
		kubernetesVersion: "1.30.0",
		talosVersion:      "invalid-version",
		endpoint:          "https://1.2.3.4:6443",
		secretsBundle:     sb,
		machineOpts:       &MachineConfigOptions{},
	}

	_, err = gen.GetClientConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse version contract")
}

func TestNewSecrets_InvalidVersion(t *testing.T) {
	t.Parallel()

	_, err := NewSecrets("invalid-version")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse version contract")
}

func TestSaveSecrets_InvalidPath(t *testing.T) {
	t.Parallel()

	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	// Try to save to a nonexistent directory
	err = SaveSecrets("/nonexistent/dir/secrets.yaml", sb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write secrets file")
}

func TestSaveSecrets_PermissionCheck(t *testing.T) {
	t.Parallel()

	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "secrets.yaml")

	err = SaveSecrets(path, sb)
	require.NoError(t, err)

	// Verify the file was created with correct permissions
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestGetOrGenerateSecrets_NewSecretsInvalidVersion(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "secrets.yaml")

	_, err := GetOrGenerateSecrets(path, "invalid-version")
	require.Error(t, err)
}

func TestGetOrGenerateSecrets_SaveError(t *testing.T) {
	t.Parallel()

	// Path in nonexistent directory - file doesn't exist so it tries to generate + save
	_, err := GetOrGenerateSecrets("/nonexistent/dir/secrets.yaml", "v1.7.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write secrets file")
}

func TestGenerateBaseConfig_InvalidTalosVersion(t *testing.T) {
	t.Parallel()

	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	gen := &Generator{
		clusterName:       "test-cluster",
		kubernetesVersion: "1.30.0",
		talosVersion:      "invalid-version",
		endpoint:          "https://1.2.3.4:6443",
		secretsBundle:     sb,
		machineOpts:       &MachineConfigOptions{},
	}

	_, err = gen.GenerateControlPlaneConfig(nil, "cp-1", 12345)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse version contract")
}

func TestGenerateWorkerConfig_InvalidTalosVersion(t *testing.T) {
	t.Parallel()

	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	gen := &Generator{
		clusterName:       "test-cluster",
		kubernetesVersion: "1.30.0",
		talosVersion:      "invalid-version",
		endpoint:          "https://1.2.3.4:6443",
		secretsBundle:     sb,
		machineOpts:       &MachineConfigOptions{},
	}

	_, err = gen.GenerateWorkerConfig("worker-1", 12345)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse version contract")
}


func TestGenerateControlPlaneConfig_WithMachineOptions(t *testing.T) {
	t.Parallel()

	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	gen := NewGenerator("test-cluster", "v1.30.0", "v1.7.0", "https://1.2.3.4:6443", sb)

	opts := &MachineConfigOptions{
		SchematicID:                "test-schematic-123",
		StateEncryption:            true,
		EphemeralEncryption:        true,
		IPv6Enabled:                false,
		PublicIPv4Enabled:          true,
		PublicIPv6Enabled:          false,
		CoreDNSEnabled:             true,
		ClusterDomain:              "cluster.local",
		AllowSchedulingOnCP:        true,
		KubeProxyReplacement:       true,
		DiscoveryServiceEnabled:    true,
		DiscoveryKubernetesEnabled: false,
		NodeIPv4CIDR:               "10.0.0.0/16",
		PodIPv4CIDR:                "10.244.0.0/16",
		ServiceIPv4CIDR:            "10.96.0.0/12",
		EtcdSubnet:                 "10.0.0.0/16",
	}
	gen.SetMachineConfigOptions(opts)

	configBytes, err := gen.GenerateControlPlaneConfig(
		[]string{"api.example.com"},
		"cp-1",
		12345,
	)
	require.NoError(t, err)
	assert.NotEmpty(t, configBytes)

	var result map[string]interface{}
	err = yaml.Unmarshal(configBytes, &result)
	require.NoError(t, err)

	// Verify installer image uses factory URL
	machine := result["machine"].(map[string]interface{})
	install := machine["install"].(map[string]interface{})
	assert.Contains(t, install["image"].(string), "factory.talos.dev/installer/test-schematic-123")

	// Verify hostname
	network := machine["network"].(map[string]interface{})
	assert.Equal(t, "cp-1", network["hostname"])

	// Verify etcd config
	cluster := result["cluster"].(map[string]interface{})
	assert.NotNil(t, cluster["etcd"])

	// Verify control plane scheduling
	assert.Equal(t, true, cluster["allowSchedulingOnControlPlanes"])
}

func TestGenerateWorkerConfig_WithOptions(t *testing.T) {
	t.Parallel()

	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	gen := NewGenerator("test-cluster", "v1.30.0", "v1.7.0", "https://1.2.3.4:6443", sb)

	opts := &MachineConfigOptions{
		StateEncryption:     false,
		EphemeralEncryption: false,
		IPv6Enabled:         true,
		PublicIPv4Enabled:   true,
		PublicIPv6Enabled:   true,
		CoreDNSEnabled:      true,
		ClusterDomain:       "custom.local",
		NodeIPv4CIDR:        "10.0.0.0/16",
	}
	gen.SetMachineConfigOptions(opts)

	configBytes, err := gen.GenerateWorkerConfig("worker-1", 67890)
	require.NoError(t, err)
	assert.NotEmpty(t, configBytes)

	var result map[string]interface{}
	err = yaml.Unmarshal(configBytes, &result)
	require.NoError(t, err)

	machine := result["machine"].(map[string]interface{})
	assert.Equal(t, "worker", machine["type"])

	// Verify node label with server ID
	nodeLabels := machine["nodeLabels"].(map[string]interface{})
	assert.Equal(t, "67890", nodeLabels["nodeid"])
}


func TestUpgradeKubernetes_ReturnsNil(t *testing.T) {
	t.Parallel()

	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	gen := NewGenerator("test-cluster", "v1.30.0", "v1.7.0", "https://1.2.3.4:6443", sb)

	// UpgradeKubernetes is a no-op stub that always returns nil
	err = gen.UpgradeKubernetes(nil, "", "")
	require.NoError(t, err)
}

func TestDerefBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		b        *bool
		def      bool
		expected bool
	}{
		{
			name:     "nil returns default true",
			b:        nil,
			def:      true,
			expected: true,
		},
		{
			name:     "nil returns default false",
			b:        nil,
			def:      false,
			expected: false,
		},
		{
			name:     "true overrides default false",
			b:        boolPtr(true),
			def:      false,
			expected: true,
		},
		{
			name:     "false overrides default true",
			b:        boolPtr(false),
			def:      true,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := derefBool(tt.b, tt.def)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStripComments_InlineHashInValue(t *testing.T) {
	t.Parallel()

	// Hash inside a value should survive (the line is not a comment)
	input := []byte("url: https://example.com/#fragment\nkey: value")
	result := stripComments(input)
	assert.Contains(t, string(result), "url: https://example.com/#fragment")
	assert.Contains(t, string(result), "key: value")
}

func TestApplyConfigPatch_EmptyPatch(t *testing.T) {
	t.Parallel()

	baseConfig := []byte("machine:\n  type: worker\n")
	patch := map[string]any{}
	result, err := applyConfigPatch(baseConfig, patch)
	require.NoError(t, err)

	var config map[string]any
	err = yaml.Unmarshal(result, &config)
	require.NoError(t, err)

	machine := config["machine"].(map[string]any)
	assert.Equal(t, "worker", machine["type"])
}

func TestDeepMerge_NilValues(t *testing.T) {
	t.Parallel()

	dst := map[string]any{"key": "value"}
	src := map[string]any{"key": nil}
	deepMerge(dst, src)
	assert.Nil(t, dst["key"])
}

func TestDeepMerge_SliceOverwrite(t *testing.T) {
	t.Parallel()

	// Slices are not merged, they are overwritten
	dst := map[string]any{"items": []string{"a", "b"}}
	src := map[string]any{"items": []string{"c"}}
	deepMerge(dst, src)
	assert.Equal(t, []string{"c"}, dst["items"])
}

func TestLoadSecrets_EmptyFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	emptyFile := filepath.Join(tempDir, "empty.yaml")
	err := os.WriteFile(emptyFile, []byte(""), 0600)
	require.NoError(t, err)

	_, err = LoadSecrets(emptyFile)
	require.Error(t, err)
}

func TestGenerateControlPlaneConfig_NoSANs(t *testing.T) {
	t.Parallel()

	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	gen := NewGenerator("test-cluster", "v1.30.0", "v1.7.0", "https://1.2.3.4:6443", sb)

	configBytes, err := gen.GenerateControlPlaneConfig(nil, "cp-no-san", 12345)
	require.NoError(t, err)
	assert.NotEmpty(t, configBytes)

	var result map[string]interface{}
	err = yaml.Unmarshal(configBytes, &result)
	require.NoError(t, err)

	machine := result["machine"].(map[string]interface{})
	network := machine["network"].(map[string]interface{})
	assert.Equal(t, "cp-no-san", network["hostname"])
}

func TestGenerateControlPlaneConfig_NoServerID(t *testing.T) {
	t.Parallel()

	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	gen := NewGenerator("test-cluster", "v1.30.0", "v1.7.0", "https://1.2.3.4:6443", sb)

	configBytes, err := gen.GenerateControlPlaneConfig(nil, "cp-no-id", 0)
	require.NoError(t, err)
	assert.NotEmpty(t, configBytes)

	var result map[string]interface{}
	err = yaml.Unmarshal(configBytes, &result)
	require.NoError(t, err)

	machine := result["machine"].(map[string]interface{})
	// nodeLabels should not have nodeid when serverID is 0
	if nodeLabels, ok := machine["nodeLabels"].(map[string]interface{}); ok {
		_, hasNodeID := nodeLabels["nodeid"]
		assert.False(t, hasNodeID, "nodeid should not be set when serverID is 0")
	}
}

func TestGenerateWorkerConfig_NoServerID(t *testing.T) {
	t.Parallel()

	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	gen := NewGenerator("test-cluster", "v1.30.0", "v1.7.0", "https://1.2.3.4:6443", sb)

	configBytes, err := gen.GenerateWorkerConfig("worker-no-id", 0)
	require.NoError(t, err)
	assert.NotEmpty(t, configBytes)

	var result map[string]interface{}
	err = yaml.Unmarshal(configBytes, &result)
	require.NoError(t, err)

	machine := result["machine"].(map[string]interface{})
	if nodeLabels, ok := machine["nodeLabels"].(map[string]interface{}); ok {
		_, hasNodeID := nodeLabels["nodeid"]
		assert.False(t, hasNodeID, "nodeid should not be set when serverID is 0")
	}
}

func TestSetMachineConfigOptions_NilPointerToType(t *testing.T) {
	t.Parallel()

	sb, err := NewSecrets("v1.7.0")
	require.NoError(t, err)

	gen := NewGenerator("test", "v1.30.0", "v1.7.0", "https://endpoint", sb)
	original := gen.machineOpts

	// Pass a typed nil pointer
	var nilOpts *MachineConfigOptions
	gen.SetMachineConfigOptions(nilOpts)
	assert.Equal(t, original, gen.machineOpts, "should not change machineOpts for nil *MachineConfigOptions")
}
