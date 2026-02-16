package compute

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvisionWorkers_EmptyPools(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	cfg.Workers = nil

	ctx := createTestContext(t, mockInfra, cfg)

	err := ProvisionWorkers(ctx)
	require.NoError(t, err)
	assert.Empty(t, ctx.State.WorkerIPs)
}

func TestProvisionWorkers_MultiplePools(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	cfg.Workers = []config.WorkerNodePool{
		{Name: "pool-a", ServerType: "cx31", Count: 2, Location: "nbg1"},
		{Name: "pool-b", ServerType: "cx41", Count: 3, Location: "fsn1"},
	}
	tracker := newPoolServerTracker(300)

	var serverTypes []string
	var mu sync.Mutex
	mockInfra.GetServerIDFunc = tracker.getServerID
	mockInfra.CreateServerFunc = func(ctx context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		mu.Lock()
		serverTypes = append(serverTypes, opts.ServerType)
		mu.Unlock()
		return tracker.createServer(ctx, opts)
	}
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.2.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 1}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	err := ProvisionWorkers(ctx)
	require.NoError(t, err)

	// Both pools should have provisioned
	assert.NotEmpty(t, ctx.State.WorkerIPs)
	assert.NotEmpty(t, ctx.State.WorkerServerIDs)

	// Verify both server types were used (both pools were provisioned)
	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, serverTypes, "cx31", "pool-a should be provisioned")
	assert.Contains(t, serverTypes, "cx41", "pool-b should be provisioned")
}

func TestProvisionWorkers_ErrorPropagation(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	cfg.Workers = []config.WorkerNodePool{
		{Name: "pool-a", ServerType: "cx31", Count: 1, Location: "nbg1"},
	}

	mockInfra.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "", nil
	}
	mockInfra.CreateServerFunc = func(_ context.Context, _ hcloud_internal.ServerCreateOpts) (string, error) {
		return "", fmt.Errorf("server type not available")
	}
	mockInfra.GetSnapshotByLabelsFunc = func(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
		return &hcloud.Image{ID: 1}, nil
	}

	ctx := createTestContext(t, mockInfra, cfg)

	err := ProvisionWorkers(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to provision worker pools")
}
