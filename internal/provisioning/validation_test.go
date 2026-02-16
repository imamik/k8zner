package provisioning

import (
	"testing"

	"github.com/imamik/k8zner/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_RequiredFields(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Location: "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	// Should have exactly 1 error for missing ClusterName
	var fieldErrors []ValidationError
	for _, e := range errors {
		if e.IsError() && e.Field == "ClusterName" {
			fieldErrors = append(fieldErrors, e)
		}
	}
	assert.Len(t, fieldErrors, 1)
	assert.Equal(t, "error", fieldErrors[0].Severity)
}

func TestValidate_NetworkCIDR(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		cidr          string
		expectError   bool
		expectWarning bool
	}{
		{
			name:        "valid /16 CIDR",
			cidr:        "10.0.0.0/16",
			expectError: false,
		},
		{
			name:          "too small /24 CIDR",
			cidr:          "10.0.0.0/24",
			expectWarning: true,
		},
		{
			name:        "invalid CIDR format",
			cidr:        "not-a-cidr",
			expectError: true,
		},
		{
			name:        "empty CIDR",
			cidr:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{
				ClusterName: "test",
				Location:    "nbg1",
				Network: config.NetworkConfig{
					IPv4CIDR: tt.cidr,
					Zone:     "eu-central",
				},
				SSHKeys: []string{"key"},
			}

			ctx := &Context{
				Config:   cfg,
				Observer: NewConsoleObserver(),
			}

			errors := validate(ctx)

			if tt.expectError {
				hasError := false
				for _, e := range errors {
					if e.IsError() && e.Field == "Network.IPv4CIDR" {
						hasError = true
						break
					}
				}
				assert.True(t, hasError, "expected a network CIDR error")
			}

			if tt.expectWarning {
				hasWarning := false
				for _, e := range errors {
					if e.Severity == "warning" && e.Field == "Network.IPv4CIDR" {
						hasWarning = true
						break
					}
				}
				assert.True(t, hasWarning, "expected a warning")
			}
		})
	}
}

func TestValidate_ServerTypes(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "cp1",
					ServerType: "", // Missing server type
					Count:      1,
				},
			},
		},
		SSHKeys: []string{"key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	var serverTypeErrors []ValidationError
	for _, e := range errors {
		if e.IsError() && e.Field == "ControlPlane.NodePools[0].ServerType" {
			serverTypeErrors = append(serverTypeErrors, e)
		}
	}
	assert.Len(t, serverTypeErrors, 1)
}

func TestValidate_SSHKeys(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		SSHKeys: []string{}, // No SSH keys
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	var sshWarnings []ValidationError
	for _, e := range errors {
		if e.Field == "SSHKeys" && e.Severity == "warning" {
			sshWarnings = append(sshWarnings, e)
		}
	}
	assert.Len(t, sshWarnings, 1)
}

func TestValidate_Versions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		talosVersion   string
		k8sVersion     string
		expectWarnings int
	}{
		{
			name:           "valid versions with v prefix",
			talosVersion:   "v1.8.3",
			k8sVersion:     "v1.31.0",
			expectWarnings: 0,
		},
		{
			name:           "versions without v prefix",
			talosVersion:   "1.8.3",
			k8sVersion:     "1.31.0",
			expectWarnings: 2,
		},
		{
			name:           "empty versions (will get defaults)",
			talosVersion:   "",
			k8sVersion:     "",
			expectWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{
				ClusterName: "test",
				Location:    "nbg1",
				Network: config.NetworkConfig{
					IPv4CIDR: "10.0.0.0/16",
					Zone:     "eu-central",
				},
				SSHKeys: []string{"key"},
				Talos: config.TalosConfig{
					Version: tt.talosVersion,
				},
				Kubernetes: config.KubernetesConfig{
					Version: tt.k8sVersion,
				},
			}

			ctx := &Context{
				Config:   cfg,
				Observer: NewConsoleObserver(),
			}

			errors := validate(ctx)

			warnings := 0
			for _, e := range errors {
				if e.Severity == "warning" && (e.Field == "Talos.Version" || e.Field == "Kubernetes.Version") {
					warnings++
				}
			}

			assert.Equal(t, tt.expectWarnings, warnings)
		})
	}
}

func TestValidationPhase_Integration(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "cp1",
					ServerType: "cx21",
					Count:      1,
				},
			},
		},
		SSHKeys: []string{"my-key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	phase := NewValidationPhase()
	err := phase.Provision(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, cfg.Network.NodeIPv4CIDR)
	assert.NotEmpty(t, cfg.Network.ServiceIPv4CIDR)
	assert.NotEmpty(t, cfg.Network.PodIPv4CIDR)
}

func TestValidationError(t *testing.T) {
	t.Parallel()
	err := ValidationError{
		Field:    "test.field",
		Message:  "test message",
		Severity: "error",
	}

	assert.True(t, err.IsError())
	assert.Contains(t, err.Error(), "error")
	assert.Contains(t, err.Error(), "test.field")
	assert.Contains(t, err.Error(), "test message")

	warning := ValidationError{
		Field:    "test.field",
		Message:  "test warning",
		Severity: "warning",
	}

	assert.False(t, warning.IsError())
}
