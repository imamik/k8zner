package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/config"
)

func TestBuildMetricsServerValues(t *testing.T) {
	// Helper for creating bool pointers
	boolPtr := func(b bool) *bool { return &b }
	intPtr := func(i int) *int { return &i }

	tests := []struct {
		name                     string
		cfg                      *config.Config
		expectedReplicas         int
		expectedScheduleOnCP     bool
		expectedNodeSelectorKeys []string
		expectedTolerationsCount int
	}{
		{
			name: "single control plane with workers",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 1},
					},
				},
				Workers: []config.WorkerNodePool{
					{Count: 3},
				},
			},
			expectedReplicas:         2,
			expectedScheduleOnCP:     false,
			expectedNodeSelectorKeys: nil,
			expectedTolerationsCount: 0,
		},
		{
			name: "HA control plane with workers",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 3},
					},
				},
				Workers: []config.WorkerNodePool{
					{Count: 5},
				},
			},
			expectedReplicas:         2,
			expectedScheduleOnCP:     false,
			expectedNodeSelectorKeys: nil,
			expectedTolerationsCount: 0,
		},
		{
			name: "control plane only cluster",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 3},
					},
				},
				Workers: []config.WorkerNodePool{},
			},
			expectedReplicas:         2,
			expectedScheduleOnCP:     true,
			expectedNodeSelectorKeys: []string{"node-role.kubernetes.io/control-plane"},
			expectedTolerationsCount: 2, // control-plane + CCM uninitialized
		},
		{
			name: "single control plane only",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 1},
					},
				},
				Workers: []config.WorkerNodePool{},
			},
			expectedReplicas:         1,
			expectedScheduleOnCP:     true,
			expectedNodeSelectorKeys: []string{"node-role.kubernetes.io/control-plane"},
			expectedTolerationsCount: 2, // control-plane + CCM uninitialized
		},
		// New tests for explicit config overrides
		{
			name: "explicit schedule_on_control_plane override",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 3},
					},
				},
				Workers: []config.WorkerNodePool{
					{Count: 5},
				},
				Addons: config.AddonsConfig{
					MetricsServer: config.MetricsServerConfig{
						Enabled:                true,
						ScheduleOnControlPlane: boolPtr(true), // Force scheduling on CP even with workers
					},
				},
			},
			expectedReplicas:         2, // Based on CP count since scheduling on CP
			expectedScheduleOnCP:     true,
			expectedNodeSelectorKeys: []string{"node-role.kubernetes.io/control-plane"},
			expectedTolerationsCount: 2,
		},
		{
			name: "explicit replicas override",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 1},
					},
				},
				Workers: []config.WorkerNodePool{},
				Addons: config.AddonsConfig{
					MetricsServer: config.MetricsServerConfig{
						Enabled:  true,
						Replicas: intPtr(3), // Override to 3 replicas
					},
				},
			},
			expectedReplicas:         3, // Explicit override
			expectedScheduleOnCP:     true,
			expectedNodeSelectorKeys: []string{"node-role.kubernetes.io/control-plane"},
			expectedTolerationsCount: 2,
		},
		{
			name: "explicit schedule_on_control_plane disabled with no workers",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Count: 3},
					},
				},
				Workers: []config.WorkerNodePool{},
				Addons: config.AddonsConfig{
					MetricsServer: config.MetricsServerConfig{
						Enabled:                true,
						ScheduleOnControlPlane: boolPtr(false), // Force disable CP scheduling
					},
				},
			},
			// Note: This is a misconfiguration (no workers, but CP scheduling disabled)
			// With 0 workers and CP scheduling disabled, nodeCount = 0, so replicas = 1
			expectedReplicas:         1,
			expectedScheduleOnCP:     false,
			expectedNodeSelectorKeys: nil,
			expectedTolerationsCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := buildMetricsServerValues(tt.cfg)

			// Check replicas
			assert.Equal(t, tt.expectedReplicas, values["replicas"])

			// Check topology spread constraints
			tsc, ok := values["topologySpreadConstraints"].([]helm.Values)
			assert.True(t, ok)
			assert.Len(t, tsc, 2) // hostname + zone

			// Check node selector
			if tt.expectedScheduleOnCP {
				nodeSelector, ok := values["nodeSelector"].(helm.Values)
				assert.True(t, ok)
				for _, key := range tt.expectedNodeSelectorKeys {
					assert.Contains(t, nodeSelector, key)
				}
			} else {
				assert.Nil(t, values["nodeSelector"])
			}

			// Check tolerations
			if tt.expectedTolerationsCount > 0 {
				tolerations, ok := values["tolerations"].([]helm.Values)
				assert.True(t, ok)
				assert.Len(t, tolerations, tt.expectedTolerationsCount)
			} else {
				assert.Nil(t, values["tolerations"])
			}
		})
	}
}
