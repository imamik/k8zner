package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"hcloud-k8s/internal/addons/helm"
	"hcloud-k8s/internal/config"
)

func TestBuildMetricsServerValues(t *testing.T) {
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
			expectedTolerationsCount: 1,
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
			expectedTolerationsCount: 1,
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
