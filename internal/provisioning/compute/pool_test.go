package compute

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/util/naming"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// poolServerTracker provides a thread-safe mock that tracks server creation
// across parallel pool provisioning. It returns the correct server ID on both
// the initial check (not found) and the post-creation lookup.
type poolServerTracker struct {
	mu      sync.Mutex
	servers map[string]string // name -> ID
	counter int64
}

func newPoolServerTracker(startID int64) *poolServerTracker {
	return &poolServerTracker{
		servers: make(map[string]string),
		counter: startID,
	}
}

func (pst *poolServerTracker) getServerID(_ context.Context, name string) (string, error) {
	pst.mu.Lock()
	defer pst.mu.Unlock()
	return pst.servers[name], nil
}

func (pst *poolServerTracker) createServer(_ context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
	pst.mu.Lock()
	defer pst.mu.Unlock()
	pst.counter++
	id := fmt.Sprintf("%d", pst.counter)
	pst.servers[opts.Name] = id
	return id, nil
}

func defaultSnapshotFunc(_ context.Context, _ map[string]string) (*hcloud.Image, error) {
	return &hcloud.Image{ID: 1}, nil
}

func TestReconcileNodePool_SingleControlPlane(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	tracker := newPoolServerTracker(100)

	mockInfra.GetServerIDFunc = tracker.getServerID
	mockInfra.CreateServerFunc = tracker.createServer
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "203.0.113.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = defaultSnapshotFunc

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	pgID := int64(1)
	result, err := p.reconcileNodePool(ctx, NodePoolSpec{
		Name:             "control-plane",
		Count:            1,
		ServerType:       "cx21",
		Location:         "nbg1",
		Role:             "control-plane",
		PlacementGroupID: &pgID,
		PoolIndex:        0,
	})

	require.NoError(t, err)
	assert.Len(t, result.IPs, 1)
	assert.Len(t, result.ServerIDs, 1)

	// Verify the server follows the random naming convention: {cluster}-cp-{5char}
	for name := range result.IPs {
		assert.Contains(t, name, "test-cluster-cp-")
		assert.Len(t, name, len("test-cluster-cp-")+naming.IDLength, "should have 5-char random suffix")
	}
}

func TestReconcileNodePool_MultipleWorkers(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	cfg.Workers = []config.WorkerNodePool{
		{Name: "pool-a", ServerType: "cx31", Count: 3, Location: "nbg1"},
	}
	tracker := newPoolServerTracker(200)

	mockInfra.GetServerIDFunc = tracker.getServerID
	mockInfra.CreateServerFunc = tracker.createServer
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.2.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = defaultSnapshotFunc

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	result, err := p.reconcileNodePool(ctx, NodePoolSpec{
		Name:       "pool-a",
		Count:      3,
		ServerType: "cx31",
		Location:   "nbg1",
		Role:       "worker",
		PoolIndex:  0,
	})

	require.NoError(t, err)
	assert.Len(t, result.IPs, 3, "should create 3 worker servers")
	assert.Len(t, result.ServerIDs, 3)

	// Verify worker naming
	for name := range result.IPs {
		assert.Contains(t, name, "test-cluster-w-")
	}
}

func TestReconcileNodePool_ControlPlanePrivateIPCalculation(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	tracker := newPoolServerTracker(0)

	var capturedPrivateIPs []string
	var mu sync.Mutex

	mockInfra.GetServerIDFunc = tracker.getServerID
	mockInfra.CreateServerFunc = func(ctx context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		mu.Lock()
		capturedPrivateIPs = append(capturedPrivateIPs, opts.PrivateIP)
		mu.Unlock()
		return tracker.createServer(ctx, opts)
	}
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.0.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = defaultSnapshotFunc

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	pgID := int64(1)
	_, err := p.reconcileNodePool(ctx, NodePoolSpec{
		Name:             "control-plane",
		Count:            3,
		ServerType:       "cx21",
		Location:         "nbg1",
		Role:             "control-plane",
		PlacementGroupID: &pgID,
		PoolIndex:        0,
	})

	require.NoError(t, err)
	require.Len(t, capturedPrivateIPs, 3)

	// PoolIndex=0, j goes from 1..3
	// hostNum = poolIndex*10 + (j-1) + 2 = 0*10 + 0 + 2 = 2, 3, 4
	// These are offsets into the CP subnet (10.0.64.0/25)
	// So: 10.0.64.2, 10.0.64.3, 10.0.64.4
	assert.Contains(t, capturedPrivateIPs, "10.0.64.2")
	assert.Contains(t, capturedPrivateIPs, "10.0.64.3")
	assert.Contains(t, capturedPrivateIPs, "10.0.64.4")
}

func TestReconcileNodePool_WorkerPrivateIPCalculation(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	cfg.Workers = []config.WorkerNodePool{
		{Name: "pool-a", ServerType: "cx31", Count: 2, Location: "nbg1"},
	}
	tracker := newPoolServerTracker(0)

	var capturedPrivateIPs []string
	var mu sync.Mutex

	mockInfra.GetServerIDFunc = tracker.getServerID
	mockInfra.CreateServerFunc = func(ctx context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		mu.Lock()
		capturedPrivateIPs = append(capturedPrivateIPs, opts.PrivateIP)
		mu.Unlock()
		return tracker.createServer(ctx, opts)
	}
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.0.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = defaultSnapshotFunc

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	_, err := p.reconcileNodePool(ctx, NodePoolSpec{
		Name:       "pool-a",
		Count:      2,
		ServerType: "cx31",
		Location:   "nbg1",
		Role:       "worker",
		PoolIndex:  0,
	})

	require.NoError(t, err)
	require.Len(t, capturedPrivateIPs, 2)

	// Worker pool 0: GetSubnetForRole("worker", 0) gives the 3rd /25 subnet
	// hostNum = (j-1) + 2 = 2, 3
	assert.Contains(t, capturedPrivateIPs, "10.0.65.2")
	assert.Contains(t, capturedPrivateIPs, "10.0.65.3")
}

func TestReconcileNodePool_WorkerPlacementGroupSharding(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	cfg.Workers = []config.WorkerNodePool{
		{Name: "pool-a", ServerType: "cx31", Count: 12, Location: "nbg1", PlacementGroup: true},
	}
	tracker := newPoolServerTracker(0)

	var pgNames []string
	var mu sync.Mutex

	mockInfra.EnsurePlacementGroupFunc = func(_ context.Context, name, _ string, _ map[string]string) (*hcloud.PlacementGroup, error) {
		mu.Lock()
		pgNames = append(pgNames, name)
		mu.Unlock()
		return &hcloud.PlacementGroup{ID: 1}, nil
	}
	mockInfra.GetServerIDFunc = tracker.getServerID
	mockInfra.CreateServerFunc = tracker.createServer
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.0.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = defaultSnapshotFunc

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	result, err := p.reconcileNodePool(ctx, NodePoolSpec{
		Name:       "pool-a",
		Count:      12,
		ServerType: "cx31",
		Location:   "nbg1",
		Role:       "worker",
		PoolIndex:  0,
	})

	require.NoError(t, err)
	assert.Len(t, result.IPs, 12)

	// Sharding: ceil((index+1)/10) groups
	// Servers 1-10 → shard 1, servers 11-12 → shard 2
	// PG names: test-cluster-w-pg-1, test-cluster-w-pg-2
	assert.Contains(t, pgNames, "test-cluster-w-pg-1")
	assert.Contains(t, pgNames, "test-cluster-w-pg-2")
}

func TestReconcileNodePool_WorkerPlacementGroupDisabled(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	cfg.Workers = []config.WorkerNodePool{
		{Name: "pool-a", ServerType: "cx31", Count: 2, Location: "nbg1", PlacementGroup: false},
	}
	tracker := newPoolServerTracker(0)

	pgCalled := false
	mockInfra.EnsurePlacementGroupFunc = func(_ context.Context, _, _ string, _ map[string]string) (*hcloud.PlacementGroup, error) {
		pgCalled = true
		return &hcloud.PlacementGroup{ID: 1}, nil
	}
	mockInfra.GetServerIDFunc = tracker.getServerID
	mockInfra.CreateServerFunc = tracker.createServer
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.0.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = defaultSnapshotFunc

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	_, err := p.reconcileNodePool(ctx, NodePoolSpec{
		Name:       "pool-a",
		Count:      2,
		ServerType: "cx31",
		Location:   "nbg1",
		Role:       "worker",
		PoolIndex:  0,
	})

	require.NoError(t, err)
	assert.False(t, pgCalled, "placement group should not be created when disabled")
}

func TestReconcileNodePool_CPPlacementGroupPassedThrough(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	tracker := newPoolServerTracker(0)

	var capturedPGIDs []*int64
	var mu sync.Mutex

	mockInfra.GetServerIDFunc = tracker.getServerID
	mockInfra.CreateServerFunc = func(ctx context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		mu.Lock()
		capturedPGIDs = append(capturedPGIDs, opts.PlacementGroupID)
		mu.Unlock()
		return tracker.createServer(ctx, opts)
	}
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.0.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = defaultSnapshotFunc

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	pgID := int64(42)
	_, err := p.reconcileNodePool(ctx, NodePoolSpec{
		Name:             "control-plane",
		Count:            2,
		ServerType:       "cx21",
		Location:         "nbg1",
		Role:             "control-plane",
		PlacementGroupID: &pgID,
		PoolIndex:        0,
	})

	require.NoError(t, err)
	require.Len(t, capturedPGIDs, 2)
	for _, id := range capturedPGIDs {
		require.NotNil(t, id)
		assert.Equal(t, int64(42), *id)
	}
}

func TestReconcileNodePool_CreateServerError(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	mockInfra.GetServerIDFunc = func(_ context.Context, _ string) (string, error) {
		return "", nil
	}
	mockInfra.CreateServerFunc = func(_ context.Context, _ hcloud_internal.ServerCreateOpts) (string, error) {
		return "", fmt.Errorf("out of capacity")
	}
	mockInfra.GetSnapshotByLabelsFunc = defaultSnapshotFunc

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	pgID := int64(1)
	_, err := p.reconcileNodePool(ctx, NodePoolSpec{
		Name:             "control-plane",
		Count:            1,
		ServerType:       "cx21",
		Location:         "nbg1",
		Role:             "control-plane",
		PlacementGroupID: &pgID,
		PoolIndex:        0,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to provision pool")
}

func TestReconcileNodePool_ZeroCount(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	result, err := p.reconcileNodePool(ctx, NodePoolSpec{
		Name:      "empty-pool",
		Count:     0,
		Role:      "worker",
		PoolIndex: 0,
	})

	require.NoError(t, err)
	assert.Empty(t, result.IPs)
	assert.Empty(t, result.ServerIDs)
}

func TestReconcileNodePool_MultiPoolCPIndexing(t *testing.T) {
	t.Parallel()
	mockInfra := &hcloud_internal.MockClient{}
	cfg := testConfigWithSubnets(t)
	tracker := newPoolServerTracker(0)

	var capturedPrivateIPs []string
	var mu sync.Mutex

	mockInfra.GetServerIDFunc = tracker.getServerID
	mockInfra.CreateServerFunc = func(ctx context.Context, opts hcloud_internal.ServerCreateOpts) (string, error) {
		mu.Lock()
		capturedPrivateIPs = append(capturedPrivateIPs, opts.PrivateIP)
		mu.Unlock()
		return tracker.createServer(ctx, opts)
	}
	mockInfra.GetServerIPFunc = func(_ context.Context, _ string) (string, error) {
		return "10.0.0.1", nil
	}
	mockInfra.GetSnapshotByLabelsFunc = defaultSnapshotFunc

	ctx := createTestContext(t, mockInfra, cfg)
	p := NewProvisioner()

	// Second CP pool (PoolIndex=1) with 2 nodes
	pgID := int64(1)
	_, err := p.reconcileNodePool(ctx, NodePoolSpec{
		Name:             "control-plane-2",
		Count:            2,
		ServerType:       "cx21",
		Location:         "nbg1",
		Role:             "control-plane",
		PlacementGroupID: &pgID,
		PoolIndex:        1,
	})

	require.NoError(t, err)
	require.Len(t, capturedPrivateIPs, 2)

	// PoolIndex=1, j=1..2
	// hostNum = 1*10 + (j-1) + 2 = 12, 13
	assert.Contains(t, capturedPrivateIPs, "10.0.64.12")
	assert.Contains(t, capturedPrivateIPs, "10.0.64.13")
}
