package addons

import (
	"context"
	"testing"
	"time"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/provisioning"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildClusterAutoscalerValues(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *config.Config
		networkID  int64
		sshKeyName string
		firewallID int64
		validate   func(t *testing.T, values helm.Values)
	}{
		{
			name: "single control plane",
			cfg: &config.Config{
				ClusterName: "test-cluster",
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 1, ServerType: "cpx21", Location: "nbg1"},
					},
				},
				Autoscaler: config.AutoscalerConfig{
					NodePools: []config.AutoscalerNodePool{
						{
							Name:     "pool1",
							Location: "nbg1",
							Type:     "cpx31",
							Min:      1,
							Max:      5,
							Labels:   map[string]string{"role": "worker"},
							Taints:   []string{"dedicated=worker:NoSchedule"},
						},
					},
				},
			},
			networkID:  12345,
			sshKeyName: "test-key",
			firewallID: 67890,
			validate: func(t *testing.T, values helm.Values) {
				assert.Equal(t, "hetzner", values["cloudProvider"])
				assert.Equal(t, 1, values["replicaCount"], "single CP should have 1 replica")

				// Check autoscaling groups
				groups, ok := values["autoscalingGroups"].([]helm.Values)
				require.True(t, ok, "autoscalingGroups should be a slice")
				require.Len(t, groups, 1, "should have 1 autoscaling group")

				group := groups[0]
				assert.Equal(t, "test-cluster-pool1", group["name"])
				assert.Equal(t, 1, group["minSize"])
				assert.Equal(t, 5, group["maxSize"])
				assert.Equal(t, "cpx31", group["instanceType"])
				assert.Equal(t, "nbg1", group["region"])

				// Check image tag
				image, ok := values["image"].(helm.Values)
				require.True(t, ok)
				assert.Equal(t, "v1.33.3", image["tag"])

				// Check node selector
				nodeSelector, ok := values["nodeSelector"].(helm.Values)
				require.True(t, ok)
				assert.Equal(t, "", nodeSelector["node-role.kubernetes.io/control-plane"])

				// Check tolerations
				tolerations, ok := values["tolerations"].([]helm.Values)
				require.True(t, ok)
				require.Len(t, tolerations, 1)
				assert.Equal(t, "node-role.kubernetes.io/control-plane", tolerations[0]["key"])
				assert.Equal(t, "NoSchedule", tolerations[0]["effect"])

				// Check environment variables
				extraEnv, ok := values["extraEnv"].(helm.Values)
				require.True(t, ok)
				assert.Equal(t, "/config/cluster-config", extraEnv["HCLOUD_CLUSTER_CONFIG_FILE"])
				assert.Equal(t, "10", extraEnv["HCLOUD_SERVER_CREATION_TIMEOUT"])
				assert.Equal(t, "67890", extraEnv["HCLOUD_FIREWALL"])
				assert.Equal(t, "test-key", extraEnv["HCLOUD_SSH_KEY"])
				assert.Equal(t, "12345", extraEnv["HCLOUD_NETWORK"])
				assert.Equal(t, "true", extraEnv["HCLOUD_PUBLIC_IPV4"])
				assert.Equal(t, "false", extraEnv["HCLOUD_PUBLIC_IPV6"])

				// Check secret references
				extraEnvSecrets, ok := values["extraEnvSecrets"].(helm.Values)
				require.True(t, ok)
				tokenSecret, ok := extraEnvSecrets["HCLOUD_TOKEN"].(helm.Values)
				require.True(t, ok)
				assert.Equal(t, "hcloud", tokenSecret["name"])
				assert.Equal(t, "token", tokenSecret["key"])

				// Check volume secrets
				extraVolumeSecrets, ok := values["extraVolumeSecrets"].(helm.Values)
				require.True(t, ok)
				configSecret, ok := extraVolumeSecrets["cluster-autoscaler-hetzner-config"].(helm.Values)
				require.True(t, ok)
				assert.Equal(t, "cluster-autoscaler-hetzner-config", configSecret["name"])
				assert.Equal(t, "/config", configSecret["mountPath"])

				// Should NOT have topology spread constraints for single CP
				_, hasTopology := values["topologySpreadConstraints"]
				assert.False(t, hasTopology, "single CP should not have topology spread constraints")
			},
		},
		{
			name: "HA control plane with multiple nodepools",
			cfg: &config.Config{
				ClusterName: "ha-cluster",
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 3, ServerType: "cpx21", Location: "nbg1"},
					},
				},
				Autoscaler: config.AutoscalerConfig{
					NodePools: []config.AutoscalerNodePool{
						{
							Name:     "cpu-pool",
							Location: "nbg1",
							Type:     "cpx31",
							Min:      0,
							Max:      10,
						},
						{
							Name:     "gpu-pool",
							Location: "fsn1",
							Type:     "cpx41",
							Min:      0,
							Max:      5,
						},
					},
				},
			},
			networkID:  99999,
			sshKeyName: "ha-key",
			firewallID: 11111,
			validate: func(t *testing.T, values helm.Values) {
				assert.Equal(t, 2, values["replicaCount"], "HA CP should have 2 replicas")

				// Check autoscaling groups
				groups, ok := values["autoscalingGroups"].([]helm.Values)
				require.True(t, ok)
				require.Len(t, groups, 2, "should have 2 autoscaling groups")

				// First pool
				assert.Equal(t, "ha-cluster-cpu-pool", groups[0]["name"])
				assert.Equal(t, 0, groups[0]["minSize"])
				assert.Equal(t, 10, groups[0]["maxSize"])
				assert.Equal(t, "cpx31", groups[0]["instanceType"])
				assert.Equal(t, "nbg1", groups[0]["region"])

				// Second pool
				assert.Equal(t, "ha-cluster-gpu-pool", groups[1]["name"])
				assert.Equal(t, 0, groups[1]["minSize"])
				assert.Equal(t, 5, groups[1]["maxSize"])
				assert.Equal(t, "cpx41", groups[1]["instanceType"])
				assert.Equal(t, "fsn1", groups[1]["region"])

				// Check PDB
				pdb, ok := values["podDisruptionBudget"].(helm.Values)
				require.True(t, ok)
				assert.Nil(t, pdb["minAvailable"])
				assert.Equal(t, 1, pdb["maxUnavailable"])

				// Should have topology spread constraints for HA
				topologyConstraints, ok := values["topologySpreadConstraints"].([]helm.Values)
				require.True(t, ok, "HA CP should have topology spread constraints")
				require.Len(t, topologyConstraints, 1)

				constraint := topologyConstraints[0]
				assert.Equal(t, "kubernetes.io/hostname", constraint["topologyKey"])
				assert.Equal(t, 1, constraint["maxSkew"])
				assert.Equal(t, "DoNotSchedule", constraint["whenUnsatisfiable"])

				labelSelector, ok := constraint["labelSelector"].(helm.Values)
				require.True(t, ok)
				matchLabels, ok := labelSelector["matchLabels"].(helm.Values)
				require.True(t, ok)
				assert.Equal(t, "cluster-autoscaler", matchLabels["app.kubernetes.io/instance"])
				assert.Equal(t, "hetzner-cluster-autoscaler", matchLabels["app.kubernetes.io/name"])

				matchLabelKeys, ok := constraint["matchLabelKeys"].([]string)
				require.True(t, ok)
				assert.Equal(t, []string{"pod-template-hash"}, matchLabelKeys)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := buildClusterAutoscalerValues(tt.cfg, tt.networkID, tt.sshKeyName, tt.firewallID)
			tt.validate(t, values)
		})
	}
}

func TestBuildAutoscalerSecretData(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos: config.TalosConfig{
			Version: "v1.9.4",
		},
		Autoscaler: config.AutoscalerConfig{
			NodePools: []config.AutoscalerNodePool{
				{
					Name:   "pool1",
					Labels: map[string]string{"zone": "eu", "type": "compute"},
					Taints: []string{"dedicated=compute:NoSchedule"},
				},
				{
					Name:   "pool2",
					Labels: map[string]string{"type": "storage"},
					Taints: []string{},
				},
			},
		},
	}

	nodepoolConfigs := map[string]string{
		"pool1": "YmFzZTY0LWVuY29kZWQtY29uZmlnLTE=",
		"pool2": "YmFzZTY0LWVuY29kZWQtY29uZmlnLTI=",
	}

	secretData := buildAutoscalerSecretData(cfg, nodepoolConfigs)

	// Check imagesForArch
	imagesForArch, ok := secretData["imagesForArch"].(map[string]string)
	require.True(t, ok, "imagesForArch should be a map")
	assert.Equal(t, "talos=v1.9.4", imagesForArch["arm64"])
	assert.Equal(t, "talos=v1.9.4", imagesForArch["amd64"])

	// Check nodeConfigs
	nodeConfigs, ok := secretData["nodeConfigs"].(map[string]any)
	require.True(t, ok, "nodeConfigs should be a map")
	require.Len(t, nodeConfigs, 2, "should have 2 node configs")

	// Check pool1
	pool1Config, ok := nodeConfigs["test-cluster-pool1"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "YmFzZTY0LWVuY29kZWQtY29uZmlnLTE=", pool1Config["cloudInit"])

	pool1Labels, ok := pool1Config["labels"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "eu", pool1Labels["zone"])
	assert.Equal(t, "compute", pool1Labels["type"])

	pool1Taints, ok := pool1Config["taints"].([]map[string]string)
	require.True(t, ok)
	require.Len(t, pool1Taints, 1)

	// Check pool2
	pool2Config, ok := nodeConfigs["test-cluster-pool2"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "YmFzZTY0LWVuY29kZWQtY29uZmlnLTI=", pool2Config["cloudInit"])

	pool2Labels, ok := pool2Config["labels"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "storage", pool2Labels["type"])

	// Pool2 has no taints
	pool2Taints, ok := pool2Config["taints"].([]map[string]string)
	assert.True(t, ok)
	assert.Nil(t, pool2Taints, "pool2 should have nil taints since it has no taints configured")
}

func TestParseTaintsForAutoscaler(t *testing.T) {
	tests := []struct {
		name     string
		taints   []string
		expected int // expected number of parsed taints
	}{
		{
			name:     "no taints",
			taints:   []string{},
			expected: 0,
		},
		{
			name:     "nil taints",
			taints:   nil,
			expected: 0,
		},
		{
			name: "single taint",
			taints: []string{
				"dedicated=worker:NoSchedule",
			},
			expected: 1,
		},
		{
			name: "multiple taints",
			taints: []string{
				"dedicated=worker:NoSchedule",
				"gpu=nvidia:NoExecute",
				"storage=ssd:PreferNoSchedule",
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTaintsForAutoscaler(tt.taints)
			if tt.expected == 0 {
				assert.Nil(t, result, "should return nil for empty taints")
			} else {
				require.NotNil(t, result)
				assert.Len(t, result, tt.expected)
				// Each taint should be a map with at least a "key" field
				for i, taint := range result {
					_, hasKey := taint["key"]
					assert.True(t, hasKey, "taint %d should have a 'key' field", i)
				}
			}
		})
	}
}

func TestGenerateAutoscalerNodepoolConfigs(t *testing.T) {
	// Create mock talos generator
	mockGen := &mockTalosConfigProducer{
		autoscalerConfigs: make(map[string][]byte),
	}

	// Setup mock to return different configs for different pools
	mockGen.autoscalerConfigs["pool1"] = []byte("config-for-pool1")
	mockGen.autoscalerConfigs["pool2"] = []byte("config-for-pool2")

	cfg := &config.Config{
		Autoscaler: config.AutoscalerConfig{
			NodePools: []config.AutoscalerNodePool{
				{
					Name:   "pool1",
					Labels: map[string]string{"role": "worker"},
					Taints: []string{"dedicated=worker:NoSchedule"},
				},
				{
					Name:   "pool2",
					Labels: map[string]string{"role": "storage"},
					Taints: []string{},
				},
			},
		},
	}

	configs, err := generateAutoscalerNodepoolConfigs(cfg, mockGen)
	require.NoError(t, err)
	require.Len(t, configs, 2, "should generate configs for 2 pools")

	// Check that configs are base64 encoded
	pool1Config, ok := configs["pool1"]
	require.True(t, ok, "should have config for pool1")
	assert.NotEmpty(t, pool1Config)
	// Base64 encoded "config-for-pool1" should be: Y29uZmlnLWZvci1wb29sMQ==
	assert.Equal(t, "Y29uZmlnLWZvci1wb29sMQ==", pool1Config)

	pool2Config, ok := configs["pool2"]
	require.True(t, ok, "should have config for pool2")
	assert.NotEmpty(t, pool2Config)
	assert.Equal(t, "Y29uZmlnLWZvci1wb29sMg==", pool2Config)
}

func TestGenerateAutoscalerNodepoolConfigsError(t *testing.T) {
	mockGen := &mockTalosConfigProducer{
		autoscalerError: assert.AnError,
	}

	cfg := &config.Config{
		Autoscaler: config.AutoscalerConfig{
			NodePools: []config.AutoscalerNodePool{
				{Name: "pool1"},
			},
		},
	}

	configs, err := generateAutoscalerNodepoolConfigs(cfg, mockGen)
	assert.Error(t, err)
	assert.Nil(t, configs)
	assert.Contains(t, err.Error(), "failed to generate config for pool")
}

// mockTalosConfigProducer is a mock implementation of TalosConfigProducer for testing.
type mockTalosConfigProducer struct {
	autoscalerConfigs map[string][]byte
	autoscalerError   error
}

func (m *mockTalosConfigProducer) GenerateControlPlaneConfig(_ []string, _ string) ([]byte, error) {
	return nil, nil
}

func (m *mockTalosConfigProducer) GenerateWorkerConfig(_ string) ([]byte, error) {
	return nil, nil
}

func (m *mockTalosConfigProducer) GenerateAutoscalerConfig(poolName string, _ map[string]string, _ []string) ([]byte, error) {
	if m.autoscalerError != nil {
		return nil, m.autoscalerError
	}
	if config, ok := m.autoscalerConfigs[poolName]; ok {
		return config, nil
	}
	return []byte("default-config"), nil
}

func (m *mockTalosConfigProducer) GetClientConfig() ([]byte, error) {
	return nil, nil
}

func (m *mockTalosConfigProducer) SetEndpoint(_ string) {
}

func (m *mockTalosConfigProducer) GetNodeVersion(_ context.Context, _ string) (string, error) {
	return "v1.8.2", nil
}

func (m *mockTalosConfigProducer) UpgradeNode(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
	return nil
}

func (m *mockTalosConfigProducer) UpgradeKubernetes(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockTalosConfigProducer) WaitForNodeReady(_ context.Context, _ string, _ time.Duration) error {
	return nil
}

func (m *mockTalosConfigProducer) HealthCheck(_ context.Context, _ string) error {
	return nil
}

func (m *mockTalosConfigProducer) SetMachineConfigOptions(_ interface{}) {
}
