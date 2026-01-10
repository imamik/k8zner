package provisioning

import (
	"testing"

	"hcloud-k8s/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationPhase_RequiredFields(t *testing.T) {
	// Missing cluster name
	cfg := &config.Config{
		Location: "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
	}

	ctx := &Context{
		Config: cfg,
		Logger: &DefaultLogger{},
	}

	validator := &RequiredFieldsValidator{}
	errors := validator.Validate(ctx)

	assert.Len(t, errors, 1)
	assert.Equal(t, "ClusterName", errors[0].Field)
	assert.Equal(t, "error", errors[0].Severity)
}

func TestValidationPhase_NetworkCIDR(t *testing.T) {
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
			cfg := &config.Config{
				Network: config.NetworkConfig{
					IPv4CIDR: tt.cidr,
				},
			}

			ctx := &Context{
				Config: cfg,
				Logger: &DefaultLogger{},
			}

			validator := &NetworkValidator{}
			errors := validator.Validate(ctx)

			if tt.expectError {
				hasError := false
				for _, e := range errors {
					if e.IsError() {
						hasError = true
						break
					}
				}
				assert.True(t, hasError, "expected an error")
			}

			if tt.expectWarning {
				hasWarning := false
				for _, e := range errors {
					if e.Severity == "warning" {
						hasWarning = true
						break
					}
				}
				assert.True(t, hasWarning, "expected a warning")
			}
		})
	}
}

func TestValidationPhase_ServerTypes(t *testing.T) {
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "cp1",
					ServerType: "", // Missing server type
					Count:      1,
				},
			},
		},
	}

	ctx := &Context{
		Config: cfg,
		Logger: &DefaultLogger{},
	}

	validator := &ServerTypeValidator{}
	errors := validator.Validate(ctx)

	assert.Len(t, errors, 1)
	assert.Contains(t, errors[0].Field, "ServerType")
	assert.Equal(t, "error", errors[0].Severity)
}

func TestValidationPhase_SSHKeys(t *testing.T) {
	cfg := &config.Config{
		SSHKeys: []string{}, // No SSH keys
	}

	ctx := &Context{
		Config: cfg,
		Logger: &DefaultLogger{},
	}

	validator := &SSHKeyValidator{}
	errors := validator.Validate(ctx)

	assert.Len(t, errors, 1)
	assert.Equal(t, "SSHKeys", errors[0].Field)
	assert.Equal(t, "warning", errors[0].Severity)
}

func TestValidationPhase_Versions(t *testing.T) {
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
			cfg := &config.Config{
				Talos: config.TalosConfig{
					Version: tt.talosVersion,
				},
				Kubernetes: config.KubernetesConfig{
					Version: tt.k8sVersion,
				},
			}

			ctx := &Context{
				Config: cfg,
				Logger: &DefaultLogger{},
			}

			validator := &VersionValidator{}
			errors := validator.Validate(ctx)

			warnings := 0
			for _, e := range errors {
				if e.Severity == "warning" {
					warnings++
				}
			}

			assert.Equal(t, tt.expectWarnings, warnings)
		})
	}
}

func TestValidationPhase_Integration(t *testing.T) {
	// Test the full validation phase
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
		Config: cfg,
		Logger: &DefaultLogger{},
	}

	phase := NewValidationPhase()
	err := phase.Provision(ctx)

	require.NoError(t, err)

	// Check that defaults were applied
	assert.Equal(t, "v1.8.3", cfg.Talos.Version)
	assert.Equal(t, "v1.31.0", cfg.Kubernetes.Version)

	// Check that subnets were calculated
	assert.NotEmpty(t, cfg.Network.NodeIPv4CIDR)
	assert.NotEmpty(t, cfg.Network.ServiceIPv4CIDR)
	assert.NotEmpty(t, cfg.Network.PodIPv4CIDR)
}

func TestValidationError(t *testing.T) {
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
