package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
)

func TestBuildLonghornValues(t *testing.T) {
	tests := []struct {
		name                         string
		defaultStorageClass          bool
		expectedDefaultStorageClass  bool
	}{
		{
			name:                        "default storage class enabled",
			defaultStorageClass:         true,
			expectedDefaultStorageClass: true,
		},
		{
			name:                        "default storage class disabled",
			defaultStorageClass:         false,
			expectedDefaultStorageClass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Addons: config.AddonsConfig{
					Longhorn: config.LonghornConfig{
						Enabled:             true,
						DefaultStorageClass: tt.defaultStorageClass,
					},
				},
			}

			values := buildLonghornValues(cfg)

			// Check manager image hotfix
			image, ok := values["image"].(helm.Values)
			require.True(t, ok)
			longhorn, ok := image["longhorn"].(helm.Values)
			require.True(t, ok)
			manager, ok := longhorn["manager"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, "v1.10.1-hotfix-1", manager["tag"])

			// Check preUpgradeChecker
			preUpgrade, ok := values["preUpgradeChecker"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, false, preUpgrade["upgradeVersionCheck"])

			// Check default settings
			defaultSettings, ok := values["defaultSettings"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, false, defaultSettings["allowCollectingLonghornUsageMetrics"])
			assert.Equal(t, false, defaultSettings["kubernetesClusterAutoscalerEnabled"])
			assert.Equal(t, false, defaultSettings["upgradeChecker"])

			// Check network policies
			networkPolicies, ok := values["networkPolicies"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, true, networkPolicies["enabled"])
			assert.Equal(t, "rke1", networkPolicies["type"])

			// Check persistence
			persistence, ok := values["persistence"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, tt.expectedDefaultStorageClass, persistence["defaultClass"])
		})
	}
}

func TestHasClusterAutoscaler(t *testing.T) {
	cfg := &config.Config{
		Workers: config.WorkersConfig{
			NodePools: []config.NodePoolConfig{
				{Count: 3},
			},
		},
	}

	// Currently always returns false since autoscaling config not implemented
	result := hasClusterAutoscaler(cfg)
	assert.Equal(t, false, result)
}

func TestCreateLonghornNamespace(t *testing.T) {
	ns := createLonghornNamespace()

	assert.Contains(t, ns, "apiVersion: v1")
	assert.Contains(t, ns, "kind: Namespace")
	assert.Contains(t, ns, "name: longhorn-system")
	assert.Contains(t, ns, "pod-security.kubernetes.io/enforce: privileged")
	assert.Contains(t, ns, "pod-security.kubernetes.io/audit: privileged")
	assert.Contains(t, ns, "pod-security.kubernetes.io/warn: privileged")
}
