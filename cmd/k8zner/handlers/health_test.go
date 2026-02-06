package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

func TestBuildBasicHealth(t *testing.T) {
	clusterName := "test-cluster"
	status := buildBasicHealth(clusterName)

	require.NotNil(t, status)
	assert.Equal(t, clusterName, status.ClusterName)
	assert.Equal(t, "Unknown", status.Phase)
	assert.False(t, status.Infrastructure.Network)
	assert.False(t, status.Infrastructure.Firewall)
	assert.False(t, status.Infrastructure.LoadBalancer)
}

func TestBuildAddonHealth(t *testing.T) {
	tests := []struct {
		name     string
		addons   map[string]k8znerv1alpha1.AddonStatus
		expected map[string]AddonHealth
	}{
		{
			name:     "nil addons",
			addons:   nil,
			expected: map[string]AddonHealth{},
		},
		{
			name:     "empty addons",
			addons:   map[string]k8znerv1alpha1.AddonStatus{},
			expected: map[string]AddonHealth{},
		},
		{
			name: "single addon",
			addons: map[string]k8znerv1alpha1.AddonStatus{
				"cilium": {
					Installed: true,
					Healthy:   true,
					Phase:     k8znerv1alpha1.AddonPhaseInstalled,
					Message:   "Running",
				},
			},
			expected: map[string]AddonHealth{
				"cilium": {
					Installed: true,
					Healthy:   true,
					Phase:     string(k8znerv1alpha1.AddonPhaseInstalled),
					Message:   "Running",
				},
			},
		},
		{
			name: "multiple addons",
			addons: map[string]k8znerv1alpha1.AddonStatus{
				"cilium": {
					Installed: true,
					Healthy:   true,
					Phase:     k8znerv1alpha1.AddonPhaseInstalled,
				},
				"traefik": {
					Installed: true,
					Healthy:   false,
					Phase:     k8znerv1alpha1.AddonPhaseInstalling,
					Message:   "Waiting for pods",
				},
			},
			expected: map[string]AddonHealth{
				"cilium": {
					Installed: true,
					Healthy:   true,
					Phase:     string(k8znerv1alpha1.AddonPhaseInstalled),
				},
				"traefik": {
					Installed: true,
					Healthy:   false,
					Phase:     string(k8znerv1alpha1.AddonPhaseInstalling),
					Message:   "Waiting for pods",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAddonHealth(tt.addons)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHealthStatus_JSON(t *testing.T) {
	status := &HealthStatus{
		ClusterName: "test-cluster",
		Region:      "nbg1",
		Phase:       "Running",
		Infrastructure: InfrastructureHealth{
			Network:      true,
			Firewall:     true,
			LoadBalancer: true,
		},
		ControlPlanes: NodeGroupHealth{
			Desired:   3,
			Ready:     3,
			Unhealthy: 0,
		},
		Workers: NodeGroupHealth{
			Desired:   5,
			Ready:     5,
			Unhealthy: 0,
		},
		Addons: map[string]AddonHealth{
			"cilium": {
				Installed: true,
				Healthy:   true,
				Phase:     "Installed",
			},
		},
	}

	// Verify we can print JSON without error
	err := printHealthJSON(status)
	require.NoError(t, err)
}

func TestPrintStatusLine(t *testing.T) {
	// These tests just ensure no panic - output goes to stdout
	tests := []struct {
		name  string
		label string
		ready bool
		extra string
	}{
		{"ready status", "Network", true, ""},
		{"not ready status", "Workers", false, "(0/3)"},
		{"installing status", "Traefik", false, "(installing...)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This shouldn't panic
			printStatusLine(tt.label, tt.ready, tt.extra)
		})
	}
}
