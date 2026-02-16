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

// --- validate: all required fields missing ---

func TestValidate_AllFieldsMissing(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{} // all required fields empty

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	// Should have at least the 3 required-field errors
	fieldNames := make(map[string]bool)
	for _, e := range errors {
		if e.IsError() {
			fieldNames[e.Field] = true
		}
	}
	assert.True(t, fieldNames["ClusterName"])
	assert.True(t, fieldNames["Location"])
	assert.True(t, fieldNames["Network.Zone"])
}

// --- validate: missing Location only ---

func TestValidate_MissingLocation(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		SSHKeys: []string{"key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	var locationErrors []ValidationError
	for _, e := range errors {
		if e.IsError() && e.Field == "Location" {
			locationErrors = append(locationErrors, e)
		}
	}
	require.Len(t, locationErrors, 1)
}

// --- validate: missing Zone only ---

func TestValidate_MissingZone(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		SSHKeys: []string{"key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	var zoneErrors []ValidationError
	for _, e := range errors {
		if e.IsError() && e.Field == "Network.Zone" {
			zoneErrors = append(zoneErrors, e)
		}
	}
	require.Len(t, zoneErrors, 1)
}

// --- validate: all fields present ---

func TestValidate_AllPresent(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		SSHKeys: []string{"key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	// Should have no errors (only fields present)
	for _, e := range errors {
		assert.False(t, e.IsError(), "unexpected error: %s", e.Error())
	}
}

// --- validate: IPv6 CIDR (bits != 32) ---

func TestValidate_IPv6CIDR(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "2001:db8::/32",
			Zone:     "eu-central",
		},
		SSHKeys: []string{"key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	hasIPv4OnlyError := false
	for _, e := range errors {
		if e.Field == "Network.IPv4CIDR" && e.Message == "only IPv4 CIDRs are supported" {
			hasIPv4OnlyError = true
		}
	}
	assert.True(t, hasIPv4OnlyError, "should report IPv6 CIDRs as unsupported")
}

// --- validate: worker pool with empty server type ---

func TestValidate_WorkerEmptyServerType(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "workers",
				ServerType: "",
				Count:      1,
			},
		},
		SSHKeys: []string{"key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	var workerErrors []ValidationError
	for _, e := range errors {
		if e.IsError() && e.Field == "Workers[0].ServerType" {
			workerErrors = append(workerErrors, e)
		}
	}
	require.Len(t, workerErrors, 1)
}

// --- validate: worker pool with negative count ---

func TestValidate_WorkerNegativeCount(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "workers",
				ServerType: "cx21",
				Count:      -1,
			},
		},
		SSHKeys: []string{"key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	var countErrors []ValidationError
	for _, e := range errors {
		if e.IsError() && e.Field == "Workers[0].Count" {
			countErrors = append(countErrors, e)
		}
	}
	require.Len(t, countErrors, 1)
	assert.Contains(t, countErrors[0].Message, "non-negative")
}

// --- validate: control plane pool with count <= 0 ---

func TestValidate_ControlPlaneZeroCount(t *testing.T) {
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
					Name:       "cp",
					ServerType: "cx21",
					Count:      0,
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

	var countErrors []ValidationError
	for _, e := range errors {
		if e.IsError() && e.Field == "ControlPlane.NodePools[0].Count" {
			countErrors = append(countErrors, e)
		}
	}
	require.Len(t, countErrors, 1)
	assert.Contains(t, countErrors[0].Message, "greater than 0")
}

// --- validate: multiple pools with mixed errors ---

func TestValidate_MultiplePoolsMixedErrors(t *testing.T) {
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
		SSHKeys: []string{"key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	// cp1: empty server type + count 0 = 2 errors
	// workers1: valid
	// workers2: empty server type + negative count = 2 errors
	var poolErrors int
	for _, e := range errors {
		if e.IsError() {
			switch e.Field {
			case "ControlPlane.NodePools[0].ServerType",
				"ControlPlane.NodePools[0].Count",
				"Workers[1].ServerType",
				"Workers[1].Count":
				poolErrors++
			}
		}
	}
	assert.Equal(t, 4, poolErrors)
}

// --- validate: all valid pools ---

func TestValidate_AllValidPools(t *testing.T) {
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
				{Name: "cp1", ServerType: "cx21", Count: 1},
			},
		},
		Workers: []config.WorkerNodePool{
			{Name: "workers", ServerType: "cx31", Count: 3},
		},
		SSHKeys: []string{"key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	for _, e := range errors {
		assert.False(t, e.IsError(), "unexpected error: %s", e.Error())
	}
}

// --- validate: SSH keys present ---

func TestValidate_SSHKeysPresent(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		SSHKeys: []string{"my-key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	for _, e := range errors {
		assert.NotEqual(t, "SSHKeys", e.Field, "SSH key warning should not appear when keys are present")
	}
}

// --- validate: valid /8 CIDR (larger than /16) ---

func TestValidate_LargeCIDR(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/8",
			Zone:     "eu-central",
		},
		SSHKeys: []string{"key"},
	}

	ctx := &Context{
		Config:   cfg,
		Observer: NewConsoleObserver(),
	}

	errors := validate(ctx)

	for _, e := range errors {
		if e.Field == "Network.IPv4CIDR" {
			assert.Fail(t, "a /8 CIDR should have no errors or warnings for Network.IPv4CIDR")
		}
	}
}
