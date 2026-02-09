package handlers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

func TestPhaseIndicator(t *testing.T) {
	t.Parallel()
	tests := []struct {
		phase    string
		expected string
	}{
		{"Running", "\u2705"},
		{"Provisioning", "\u23f3"},
		{"Failed", "\u274c"},
		{"Error", "\u274c"},
		{"Unknown", "\u2753"},
		{"", "\u2753"},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, phaseIndicator(tt.phase))
		})
	}
}

func TestPrintRow(t *testing.T) {
	t.Run("ready with extra", func(t *testing.T) {
		output := captureOutput(func() {
			printRow("Control Planes", true, "3/3")
		})
		assert.Contains(t, output, "\u2705")
		assert.Contains(t, output, "Control Planes")
		assert.Contains(t, output, "3/3")
	})

	t.Run("not ready with extra", func(t *testing.T) {
		output := captureOutput(func() {
			printRow("Workers", false, "1/2")
		})
		assert.Contains(t, output, "\u274c")
		assert.Contains(t, output, "Workers")
		assert.Contains(t, output, "1/2")
	})

	t.Run("ready without extra", func(t *testing.T) {
		output := captureOutput(func() {
			printRow("Network", true, "")
		})
		assert.Contains(t, output, "\u2705")
		assert.Contains(t, output, "Network")
	})
}

func TestAddonExtra(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		addon    AddonHealth
		expected string
	}{
		{
			name:     "not installed, no phase",
			addon:    AddonHealth{Installed: false, Phase: ""},
			expected: "",
		},
		{
			name:     "installed phase",
			addon:    AddonHealth{Installed: true, Phase: string(k8znerv1alpha1.AddonPhaseInstalled)},
			expected: "",
		},
		{
			name:     "installing phase",
			addon:    AddonHealth{Installed: false, Phase: string(k8znerv1alpha1.AddonPhaseInstalling)},
			expected: "Installing",
		},
		{
			name:     "pending phase",
			addon:    AddonHealth{Installed: false, Phase: string(k8znerv1alpha1.AddonPhasePending)},
			expected: "Pending",
		},
		{
			name:     "failed phase",
			addon:    AddonHealth{Installed: false, Phase: string(k8znerv1alpha1.AddonPhaseFailed)},
			expected: "Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, addonExtra(tt.addon))
		})
	}
}

func TestBuildAddonHealth(t *testing.T) {
	t.Parallel()
	t.Run("empty addons", func(t *testing.T) {
		t.Parallel()
		result := buildAddonHealth(nil)
		assert.Empty(t, result)
	})

	t.Run("converts addon statuses", func(t *testing.T) {
		t.Parallel()
		input := map[string]k8znerv1alpha1.AddonStatus{
			"cilium": {
				Installed: true,
				Healthy:   true,
				Phase:     k8znerv1alpha1.AddonPhaseInstalled,
				Message:   "Running",
			},
			"traefik": {
				Installed: false,
				Healthy:   false,
				Phase:     k8znerv1alpha1.AddonPhaseInstalling,
				Message:   "Waiting for chart",
			},
		}

		result := buildAddonHealth(input)

		require.Len(t, result, 2)

		cilium := result["cilium"]
		assert.True(t, cilium.Installed)
		assert.True(t, cilium.Healthy)
		assert.Equal(t, "Installed", cilium.Phase)
		assert.Equal(t, "Running", cilium.Message)

		traefik := result["traefik"]
		assert.False(t, traefik.Installed)
		assert.False(t, traefik.Healthy)
		assert.Equal(t, "Installing", traefik.Phase)
		assert.Equal(t, "Waiting for chart", traefik.Message)
	})
}

func TestPrintDoctorJSON(t *testing.T) {
	status := &DoctorStatus{
		ClusterName: "test-cluster",
		Region:      "nbg1",
		Phase:       "Running",
		Infrastructure: InfrastructureHealth{
			Network:      true,
			Firewall:     true,
			LoadBalancer: true,
		},
		ControlPlanes: NodeGroupHealth{
			Desired: 3, Ready: 3, Unhealthy: 0,
		},
		Workers: NodeGroupHealth{
			Desired: 2, Ready: 2, Unhealthy: 0,
		},
		Addons: map[string]AddonHealth{
			"cilium": {Installed: true, Healthy: true, Phase: "Installed"},
		},
	}

	output := captureOutput(func() {
		err := printDoctorJSON(status)
		require.NoError(t, err)
	})

	// Verify it's valid JSON
	var parsed DoctorStatus
	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)

	assert.Equal(t, "test-cluster", parsed.ClusterName)
	assert.Equal(t, "nbg1", parsed.Region)
	assert.Equal(t, "Running", parsed.Phase)
	assert.True(t, parsed.Infrastructure.Network)
	assert.Equal(t, 3, parsed.ControlPlanes.Ready)
	assert.Equal(t, 2, parsed.Workers.Ready)
	assert.True(t, parsed.Addons["cilium"].Healthy)
}

func TestPrintDoctorFormatted(t *testing.T) {
	t.Run("running cluster", func(t *testing.T) {
		status := &DoctorStatus{
			ClusterName: "prod",
			Region:      "fsn1",
			Phase:       "Running",
			Infrastructure: InfrastructureHealth{
				Network:      true,
				Firewall:     true,
				LoadBalancer: true,
			},
			ControlPlanes: NodeGroupHealth{
				Desired: 3, Ready: 3, Unhealthy: 0,
			},
			Workers: NodeGroupHealth{
				Desired: 2, Ready: 2, Unhealthy: 0,
			},
			Addons: map[string]AddonHealth{
				k8znerv1alpha1.AddonNameCilium: {Installed: true, Healthy: true, Phase: "Installed"},
				k8znerv1alpha1.AddonNameCCM:    {Installed: true, Healthy: true, Phase: "Installed"},
			},
		}

		output := captureOutput(func() {
			err := printDoctorFormatted(status)
			require.NoError(t, err)
		})

		// Verify sections present
		assert.Contains(t, output, "prod")
		assert.Contains(t, output, "fsn1")
		assert.Contains(t, output, "Running")
		assert.Contains(t, output, "Infrastructure")
		assert.Contains(t, output, "Network")
		assert.Contains(t, output, "Firewall")
		assert.Contains(t, output, "Load Balancer")
		assert.Contains(t, output, "Nodes")
		assert.Contains(t, output, "Control Planes")
		assert.Contains(t, output, "3/3")
		assert.Contains(t, output, "Workers")
		assert.Contains(t, output, "2/2")
		assert.Contains(t, output, "Addons")
		assert.Contains(t, output, "cilium")
		assert.Contains(t, output, "hcloud-ccm")
	})

	t.Run("provisioning cluster shows phase detail", func(t *testing.T) {
		status := &DoctorStatus{
			ClusterName:  "dev",
			Phase:        "Provisioning",
			Provisioning: "CNI",
			Infrastructure: InfrastructureHealth{
				Network: true,
			},
			ControlPlanes: NodeGroupHealth{
				Desired: 1, Ready: 1,
			},
		}

		output := captureOutput(func() {
			err := printDoctorFormatted(status)
			require.NoError(t, err)
		})

		assert.Contains(t, output, "Provisioning")
		assert.Contains(t, output, "CNI")
	})

	t.Run("running cluster does not show provisioning detail", func(t *testing.T) {
		status := &DoctorStatus{
			ClusterName:  "dev",
			Phase:        "Running",
			Provisioning: "CNI", // Should be suppressed when Running
		}

		output := captureOutput(func() {
			err := printDoctorFormatted(status)
			require.NoError(t, err)
		})

		assert.Contains(t, output, "Running")
		assert.NotContains(t, output, "(CNI)")
	})
}

func TestPrintDoctorPreCluster(t *testing.T) {
	t.Run("formatted output", func(t *testing.T) {
		output := captureOutput(func() {
			printHeader("my-cluster", "nbg1")
		})

		assert.Contains(t, output, "k8zner cluster: my-cluster")
		assert.Contains(t, output, "nbg1")
	})

	t.Run("header without region", func(t *testing.T) {
		output := captureOutput(func() {
			printHeader("test", "")
		})

		assert.Contains(t, output, "k8zner cluster: test")
		assert.NotContains(t, output, "()")
	})
}
