package provisioning

import (
	"testing"

	"github.com/imamik/k8zner/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ValidationPhase.Name() ---

func TestValidationPhase_Name(t *testing.T) {
	t.Parallel()
	phase := NewValidationPhase()
	assert.Equal(t, "validation", phase.Name())
}

// --- ValidationPhase.Provision: errors cause failure ---

func TestValidationPhase_Provision_WithErrors(t *testing.T) {
	t.Parallel()
	// Config that triggers validation errors (missing ClusterName, Location, Zone)
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	phase := NewValidationPhase()
	err := phase.Provision(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "configuration validation failed")
}

// --- ValidationPhase.Provision: warnings only should pass ---

func TestValidationPhase_Provision_WithWarningsOnly(t *testing.T) {
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
		// No SSH keys triggers a warning (not an error)
		SSHKeys: []string{},
		// Versions without 'v' prefix trigger warnings
		Talos: config.TalosConfig{
			Version: "1.8.3",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "1.31.0",
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	phase := NewValidationPhase()
	err := phase.Provision(ctx)

	require.NoError(t, err, "warnings-only should not cause an error")
}

// --- RequiredFieldsValidator: all fields missing ---

func TestRequiredFieldsValidator_AllFieldsMissing(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{} // all required fields empty

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &RequiredFieldsValidator{}
	errors := validator.Validate(ctx)

	require.Len(t, errors, 3, "should have 3 errors: ClusterName, Location, Network.Zone")
	fieldNames := make(map[string]bool)
	for _, e := range errors {
		fieldNames[e.Field] = true
		assert.Equal(t, "error", e.Severity)
	}
	assert.True(t, fieldNames["ClusterName"])
	assert.True(t, fieldNames["Location"])
	assert.True(t, fieldNames["Network.Zone"])
}

// --- RequiredFieldsValidator: missing Location only ---

func TestRequiredFieldsValidator_MissingLocation(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Network: config.NetworkConfig{
			Zone: "eu-central",
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &RequiredFieldsValidator{}
	errors := validator.Validate(ctx)

	require.Len(t, errors, 1)
	assert.Equal(t, "Location", errors[0].Field)
}

// --- RequiredFieldsValidator: missing Zone only ---

func TestRequiredFieldsValidator_MissingZone(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Location:    "nbg1",
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &RequiredFieldsValidator{}
	errors := validator.Validate(ctx)

	require.Len(t, errors, 1)
	assert.Equal(t, "Network.Zone", errors[0].Field)
}

// --- RequiredFieldsValidator: all fields present ---

func TestRequiredFieldsValidator_AllPresent(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			Zone: "eu-central",
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &RequiredFieldsValidator{}
	errors := validator.Validate(ctx)

	assert.Empty(t, errors)
}

// --- NetworkValidator: IPv6 CIDR (bits != 32) ---

func TestNetworkValidator_IPv6CIDR(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "2001:db8::/32",
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &NetworkValidator{}
	errors := validator.Validate(ctx)

	hasIPv4OnlyError := false
	for _, e := range errors {
		if e.Field == "Network.IPv4CIDR" && e.Message == "only IPv4 CIDRs are supported" {
			hasIPv4OnlyError = true
		}
	}
	assert.True(t, hasIPv4OnlyError, "should report IPv6 CIDRs as unsupported")
}

// --- ServerTypeValidator: worker pool with empty server type ---

func TestServerTypeValidator_WorkerEmptyServerType(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Workers: []config.WorkerNodePool{
			{
				Name:       "workers",
				ServerType: "",
				Count:      1,
			},
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &ServerTypeValidator{}
	errors := validator.Validate(ctx)

	require.Len(t, errors, 1)
	assert.Contains(t, errors[0].Field, "Workers[0].ServerType")
	assert.Equal(t, "error", errors[0].Severity)
}

// --- ServerTypeValidator: worker pool with negative count ---

func TestServerTypeValidator_WorkerNegativeCount(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Workers: []config.WorkerNodePool{
			{
				Name:       "workers",
				ServerType: "cx21",
				Count:      -1,
			},
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &ServerTypeValidator{}
	errors := validator.Validate(ctx)

	require.Len(t, errors, 1)
	assert.Contains(t, errors[0].Field, "Workers[0].Count")
	assert.Contains(t, errors[0].Message, "non-negative")
}

// --- ServerTypeValidator: control plane pool with count <= 0 ---

func TestServerTypeValidator_ControlPlaneZeroCount(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "cp",
					ServerType: "cx21",
					Count:      0,
				},
			},
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &ServerTypeValidator{}
	errors := validator.Validate(ctx)

	require.Len(t, errors, 1)
	assert.Contains(t, errors[0].Field, "ControlPlane.NodePools[0].Count")
	assert.Contains(t, errors[0].Message, "greater than 0")
}

// --- ServerTypeValidator: multiple pools with mixed errors ---

func TestServerTypeValidator_MultiplePoolsMixedErrors(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "cp1",
					ServerType: "",
					Count:      0,
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "workers1",
				ServerType: "cx21",
				Count:      2,
			},
			{
				Name:       "workers2",
				ServerType: "",
				Count:      -1,
			},
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &ServerTypeValidator{}
	errors := validator.Validate(ctx)

	// cp1: empty server type + count 0 = 2 errors
	// workers1: valid
	// workers2: empty server type + negative count = 2 errors
	require.Len(t, errors, 4)
}

// --- ServerTypeValidator: all valid pools ---

func TestServerTypeValidator_AllValid(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp1", ServerType: "cx21", Count: 1},
			},
		},
		Workers: []config.WorkerNodePool{
			{Name: "workers", ServerType: "cx31", Count: 3},
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &ServerTypeValidator{}
	errors := validator.Validate(ctx)

	assert.Empty(t, errors)
}

// --- SSHKeyValidator: keys present ---

func TestSSHKeyValidator_KeysPresent(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		SSHKeys: []string{"my-key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &SSHKeyValidator{}
	errors := validator.Validate(ctx)

	assert.Empty(t, errors)
}

// --- NetworkValidator: valid /8 CIDR (larger than /16) ---

func TestNetworkValidator_LargeCIDR(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/8",
		},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	validator := &NetworkValidator{}
	errors := validator.Validate(ctx)

	assert.Empty(t, errors, "a /8 CIDR should have no errors or warnings")
}
