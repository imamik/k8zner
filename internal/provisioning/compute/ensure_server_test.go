package compute

import (
	"context"
	"fmt"
	"sync"
	"testing"

	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serverTracker provides a stateful mock that tracks created servers.
// ensureServer calls GetServerID twice: before and after creation.
type serverTracker struct {
	mu      sync.Mutex
	servers map[string]string // name -> ID
}

func newServerTracker() *serverTracker {
	return &serverTracker{servers: make(map[string]string)}
}

func (st *serverTracker) getID(_ context.Context, name string) (string, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.servers[name], nil
}

func (st *serverTracker) create(id string) func(context.Context, hcloud_internal.ServerCreateOpts) (string, error) {
	return func(_ context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		st.mu.Lock()
		st.servers[opts.Name] = id
		st.mu.Unlock()
		return id, nil
	}
}

func TestEnsureServer_CreatesNewServer(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	tracker := newServerTracker()

	mockInfra.GetServerIDFunc = tracker.getID

	var capturedOpts hcloud_internal.ServerCreateOpts
	mockInfra.CreateServerFunc = func(ctx context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		capturedOpts = opts
		return tracker.create("42")(ctx, opts)
	}

	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "203.0.113.10", nil
	}

	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 789, Description: "talos-v1.8.3-k8s-v1.31.0-amd64"}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	info, err := ensureServer(ctx, ServerSpec{
		Name:     "test-cluster-cp-1",
		Type:     "cx21",
		Location: "nbg1",
		Image:    "",
		Role:     "control-plane",
		Pool:     "control-plane",
	})

	require.NoError(t, err)
	assert.Equal(t, "203.0.113.10", info.IP)
	assert.Equal(t, int64(42), info.ServerID)
	assert.Equal(t, "test-cluster-cp-1", capturedOpts.Name)
	assert.Equal(t, "cx21", capturedOpts.ServerType)
	assert.Equal(t, "nbg1", capturedOpts.Location)
	assert.Equal(t, "789", capturedOpts.ImageType) // Snapshot ID from ensureImage
	assert.Equal(t, "control-plane", capturedOpts.Labels["role"])
}

func TestEnsureServer_ReturnsExistingServer(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "99", nil // Server already exists
	}
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "203.0.113.50", nil
	}

	createCalled := false
	mockInfra.CreateServerFunc = func(_ context.Context, _ hcloud_internal.ServerCreateOpts) (string, error) {
		createCalled = true
		return "", fmt.Errorf("should not be called")
	}

	ctx := createTestContext(t, mockInfra, cfg)

	info, err := ensureServer(ctx, ServerSpec{
		Name:     "test-cluster-cp-1",
		Type:     "cx21",
		Location: "nbg1",
		Role:     "control-plane",
		Pool:     "control-plane",
	})

	require.NoError(t, err)
	assert.False(t, createCalled, "CreateServer should not be called for existing server")
	assert.Equal(t, "203.0.113.50", info.IP)
	assert.Equal(t, int64(99), info.ServerID)
}

func TestEnsureServer_DualStackDefaultsWhenNoneSet(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	tracker := newServerTracker()

	mockInfra.GetServerIDFunc = tracker.getID

	var capturedOpts hcloud_internal.ServerCreateOpts
	mockInfra.CreateServerFunc = func(ctx context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		capturedOpts = opts
		return tracker.create("1")(ctx, opts)
	}
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.0.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 1}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	_, err := ensureServer(ctx, ServerSpec{
		Name:             "test-cluster-cp-1",
		Type:             "cx21",
		Location:         "nbg1",
		Role:             "control-plane",
		Pool:             "control-plane",
		EnablePublicIPv4: false,
		EnablePublicIPv6: false,
	})

	require.NoError(t, err)
	// When neither is set, defaults to dual-stack
	assert.True(t, capturedOpts.EnablePublicIPv4, "should default to IPv4 enabled")
	assert.True(t, capturedOpts.EnablePublicIPv6, "should default to IPv6 enabled")
}

func TestEnsureServer_RespectsExplicitIPConfig(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	tracker := newServerTracker()

	mockInfra.GetServerIDFunc = tracker.getID

	var capturedOpts hcloud_internal.ServerCreateOpts
	mockInfra.CreateServerFunc = func(ctx context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		capturedOpts = opts
		return tracker.create("1")(ctx, opts)
	}
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.0.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 1}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	// IPv4 only
	_, err := ensureServer(ctx, ServerSpec{
		Name:             "test-cluster-cp-1",
		Type:             "cx21",
		Location:         "nbg1",
		Role:             "control-plane",
		Pool:             "control-plane",
		EnablePublicIPv4: true,
		EnablePublicIPv6: false,
	})

	require.NoError(t, err)
	assert.True(t, capturedOpts.EnablePublicIPv4)
	assert.False(t, capturedOpts.EnablePublicIPv6)
}

func TestEnsureServer_CustomImage(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	tracker := newServerTracker()

	mockInfra.GetServerIDFunc = tracker.getID

	var capturedOpts hcloud_internal.ServerCreateOpts
	mockInfra.CreateServerFunc = func(ctx context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		capturedOpts = opts
		return tracker.create("1")(ctx, opts)
	}
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.0.1", nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	_, err := ensureServer(ctx, ServerSpec{
		Name:     "test-cluster-w-1",
		Type:     "cx21",
		Location: "nbg1",
		Image:    "custom-image-123",
		Role:     "worker",
		Pool:     "pool-a",
	})

	require.NoError(t, err)
	// Custom image should bypass ensureImage
	assert.Equal(t, "custom-image-123", capturedOpts.ImageType)
}

func TestEnsureServer_NilNetworkState(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 1}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)
	ctx.State.Network = nil // No network

	_, err := ensureServer(ctx, ServerSpec{
		Name:     "test-cluster-cp-1",
		Type:     "cx21",
		Location: "nbg1",
		Role:     "control-plane",
		Pool:     "control-plane",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "network not initialized")
}

func TestEnsureServer_GetServerIDError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("API connection failed")
	}

	ctx := createTestContext(t, mockInfra, cfg)

	_, err := ensureServer(ctx, ServerSpec{
		Name: "test-cluster-cp-1",
		Type: "cx21",
		Role: "control-plane",
		Pool: "control-plane",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "API connection failed")
}

func TestEnsureServer_CreateServerError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "", nil
	}
	mockInfra.CreateServerFunc = func(_ context.Context, _ hcloud_internal.ServerCreateOpts) (string, error) {
		return "", fmt.Errorf("server quota exceeded")
	}
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 1}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	_, err := ensureServer(ctx, ServerSpec{
		Name:     "test-cluster-cp-1",
		Type:     "cx21",
		Location: "nbg1",
		Role:     "control-plane",
		Pool:     "control-plane",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create server")
}

func TestEnsureServer_LabelsIncludeExtraAndTestID(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	cfg.TestID = "e2e-abc123"
	tracker := newServerTracker()

	mockInfra.GetServerIDFunc = tracker.getID

	var capturedOpts hcloud_internal.ServerCreateOpts
	mockInfra.CreateServerFunc = func(ctx context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		capturedOpts = opts
		return tracker.create("1")(ctx, opts)
	}
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.0.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 1}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	_, err := ensureServer(ctx, ServerSpec{
		Name:        "test-cluster-w-1",
		Type:        "cx21",
		Location:    "nbg1",
		Image:       "custom",
		Role:        "worker",
		Pool:        "pool-a",
		ExtraLabels: map[string]string{"env": "test"},
	})

	require.NoError(t, err)
	assert.Equal(t, "worker", capturedOpts.Labels["role"])
	assert.Equal(t, "pool-a", capturedOpts.Labels["pool"])
	assert.Equal(t, "test", capturedOpts.Labels["env"])
	assert.Equal(t, "e2e-abc123", capturedOpts.Labels["test-id"])
}
