package addons

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/config"
)

func TestBuildOperatorValues(t *testing.T) {
	// Clean env for test isolation
	t.Setenv("K8ZNER_OPERATOR_VERSION", "")
	os.Unsetenv("K8ZNER_OPERATOR_VERSION")

	t.Run("HA mode - 3 control planes", func(t *testing.T) {
		cfg := &config.Config{
			HCloudToken: "test-token",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Count: 3},
				},
			},
			Addons: config.AddonsConfig{
				Operator: config.OperatorConfig{
					Enabled: true,
					Version: "v1.0.0",
				},
			},
		}

		values := buildOperatorValues(cfg)

		// HA: 2 replicas (3 CP nodes, hostNetwork off => not capped)
		assert.Equal(t, 2, values["replicaCount"])

		// Image
		image, ok := values["image"].(helm.Values)
		require.True(t, ok)
		assert.Equal(t, "ghcr.io/imamik/k8zner-operator", image["repository"])
		assert.Equal(t, "v1.0.0", image["tag"])
		assert.Equal(t, "Always", image["pullPolicy"])

		// Credentials
		creds, ok := values["credentials"].(helm.Values)
		require.True(t, ok)
		assert.Equal(t, "test-token", creds["hcloudToken"])

		// Leader election
		le, ok := values["leaderElection"].(helm.Values)
		require.True(t, ok)
		assert.Equal(t, true, le["enabled"])

		// Resources
		resources, ok := values["resources"].(helm.Values)
		require.True(t, ok)
		limits, ok := resources["limits"].(helm.Values)
		require.True(t, ok)
		assert.Equal(t, "200m", limits["cpu"])
		assert.Equal(t, "256Mi", limits["memory"])
	})

	t.Run("single control plane", func(t *testing.T) {
		cfg := &config.Config{
			HCloudToken: "test-token",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Count: 1},
				},
			},
			Addons: config.AddonsConfig{
				Operator: config.OperatorConfig{
					Enabled: true,
					Version: "v1.0.0",
				},
			},
		}

		values := buildOperatorValues(cfg)
		assert.Equal(t, 2, values["replicaCount"])
	})

	t.Run("default version when empty", func(t *testing.T) {
		cfg := &config.Config{
			HCloudToken: "test-token",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Count: 1},
				},
			},
			Addons: config.AddonsConfig{
				Operator: config.OperatorConfig{
					Enabled: true,
					Version: "", // empty
				},
			},
		}

		values := buildOperatorValues(cfg)
		image, ok := values["image"].(helm.Values)
		require.True(t, ok)
		assert.Equal(t, "main", image["tag"])
	})

	t.Run("env var overrides version", func(t *testing.T) {
		t.Setenv("K8ZNER_OPERATOR_VERSION", "test-branch")

		cfg := &config.Config{
			HCloudToken: "test-token",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Count: 1},
				},
			},
			Addons: config.AddonsConfig{
				Operator: config.OperatorConfig{
					Enabled: true,
					Version: "v1.0.0",
				},
			},
		}

		values := buildOperatorValues(cfg)
		image, ok := values["image"].(helm.Values)
		require.True(t, ok)
		assert.Equal(t, "test-branch", image["tag"])
	})

	t.Run("env var with slashes is sanitized", func(t *testing.T) {
		t.Setenv("K8ZNER_OPERATOR_VERSION", "refactor/k8s-operator-v3")

		cfg := &config.Config{
			HCloudToken: "test-token",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Count: 1},
				},
			},
			Addons: config.AddonsConfig{
				Operator: config.OperatorConfig{
					Enabled: true,
					Version: "v1.0.0",
				},
			},
		}

		values := buildOperatorValues(cfg)
		image, ok := values["image"].(helm.Values)
		require.True(t, ok)
		assert.Equal(t, "refactor-k8s-operator-v3", image["tag"])
	})

	t.Run("hostNetwork mode", func(t *testing.T) {
		cfg := &config.Config{
			HCloudToken: "test-token",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Count: 3},
				},
			},
			Addons: config.AddonsConfig{
				Operator: config.OperatorConfig{
					Enabled:     true,
					Version:     "v1.0.0",
					HostNetwork: true,
				},
			},
		}

		values := buildOperatorValues(cfg)

		assert.Equal(t, true, values["hostNetwork"])
		assert.Equal(t, "Default", values["dnsPolicy"])
		// With hostNetwork=true and 3 CPs, replicas should be 2 (min of 2, 3)
		assert.Equal(t, 2, values["replicaCount"])
	})

	t.Run("hostNetwork with 1 control plane caps replicas", func(t *testing.T) {
		cfg := &config.Config{
			HCloudToken: "test-token",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Count: 1},
				},
			},
			Addons: config.AddonsConfig{
				Operator: config.OperatorConfig{
					Enabled:     true,
					Version:     "v1.0.0",
					HostNetwork: true,
				},
			},
		}

		values := buildOperatorValues(cfg)

		assert.Equal(t, true, values["hostNetwork"])
		// With hostNetwork=true and 1 CP, replicas capped at 1
		assert.Equal(t, 1, values["replicaCount"])
	})

	t.Run("monitoring enabled adds ServiceMonitor", func(t *testing.T) {
		cfg := &config.Config{
			HCloudToken: "test-token",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Count: 1},
				},
			},
			Addons: config.AddonsConfig{
				Operator: config.OperatorConfig{
					Enabled: true,
					Version: "v1.0.0",
				},
				KubePrometheusStack: config.KubePrometheusStackConfig{
					Enabled: true,
				},
			},
		}

		values := buildOperatorValues(cfg)

		metrics, ok := values["metrics"].(helm.Values)
		require.True(t, ok)
		assert.Equal(t, true, metrics["enabled"])
		sm, ok := metrics["serviceMonitor"].(helm.Values)
		require.True(t, ok)
		assert.Equal(t, true, sm["enabled"])
	})

	t.Run("monitoring disabled omits ServiceMonitor", func(t *testing.T) {
		cfg := &config.Config{
			HCloudToken: "test-token",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Count: 1},
				},
			},
			Addons: config.AddonsConfig{
				Operator: config.OperatorConfig{
					Enabled: true,
					Version: "v1.0.0",
				},
			},
		}

		values := buildOperatorValues(cfg)
		_, hasMetrics := values["metrics"]
		assert.False(t, hasMetrics)
	})
}
