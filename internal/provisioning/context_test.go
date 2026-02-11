package provisioning

import (
	"context"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewState(t *testing.T) {
	t.Parallel()
	state := NewState()

	require.NotNil(t, state)
	assert.NotNil(t, state.ControlPlaneIPs)
	assert.NotNil(t, state.WorkerIPs)
	assert.NotNil(t, state.ControlPlaneServerIDs)
	assert.NotNil(t, state.WorkerServerIDs)

	// Maps should be empty but initialized
	assert.Empty(t, state.ControlPlaneIPs)
	assert.Empty(t, state.WorkerIPs)

	// Other fields should be nil/zero
	assert.Nil(t, state.Network)
	assert.Nil(t, state.Firewall)
	assert.Nil(t, state.LoadBalancer)
	assert.Empty(t, state.Kubeconfig)
	assert.Empty(t, state.TalosConfig)
}

func TestNewContext(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test",
	}
	mockInfra := &hcloud_internal.MockClient{}

	ctx := NewContext(context.Background(), cfg, mockInfra, nil)

	require.NotNil(t, ctx)
	assert.Equal(t, cfg, ctx.Config)
	assert.Equal(t, mockInfra, ctx.Infra)
	assert.NotNil(t, ctx.State)
	assert.NotNil(t, ctx.Observer)
	assert.NotNil(t, ctx.Logger)
	assert.NotNil(t, ctx.Timeouts)

	// State should be initialized with empty maps
	assert.NotNil(t, ctx.State.ControlPlaneIPs)
	assert.NotNil(t, ctx.State.WorkerIPs)
}

func TestNewContext_ObserverImplementsLogger(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	ctx := NewContext(context.Background(), cfg, &hcloud_internal.MockClient{}, nil)

	// Observer and Logger should point to the same object
	assert.Equal(t, ctx.Observer, ctx.Logger)
}

func TestDefaultLogger(t *testing.T) {
	t.Parallel()
	logger := &DefaultLogger{}

	// Should not panic
	logger.Printf("test message: %s %d", "hello", 42)
}
