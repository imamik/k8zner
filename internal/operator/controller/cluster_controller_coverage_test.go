package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

// --- ensureHCloudClient tests ---

func TestEnsureHCloudClient_AlreadyInitialized(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	mockHCloud := &MockHCloudClient{}

	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	err := r.ensureHCloudClient()
	require.NoError(t, err)
	assert.Equal(t, mockHCloud, r.hcloudClient, "should keep existing client")
}

func TestEnsureHCloudClient_NoTokenReturnsError(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(client, scheme, recorder)
	// No HCloud client and no token

	err := r.ensureHCloudClient()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HCloud token not configured")
}

func TestEnsureHCloudClient_CreatesClientWithToken(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudToken("test-token-123"),
	)

	assert.Nil(t, r.hcloudClient, "client should be nil before ensureHCloudClient")

	err := r.ensureHCloudClient()
	require.NoError(t, err)
	assert.NotNil(t, r.hcloudClient, "client should be created with token")
}

// --- findUnhealthyNodes tests ---

func TestFindUnhealthyNodes_Empty(t *testing.T) {
	t.Parallel()
	result := findUnhealthyNodes(nil, 3*time.Minute)
	assert.Empty(t, result)
}

func TestFindUnhealthyNodes_AllHealthy(t *testing.T) {
	t.Parallel()
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: true},
		{Name: "w-2", Healthy: true},
	}
	result := findUnhealthyNodes(nodes, 3*time.Minute)
	assert.Empty(t, result)
}

func TestFindUnhealthyNodes_UnhealthyBelowThreshold(t *testing.T) {
	t.Parallel()
	recentTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: false, UnhealthySince: &recentTime},
	}
	// Threshold is 3 minutes, node has been unhealthy for only 1 minute
	result := findUnhealthyNodes(nodes, 3*time.Minute)
	assert.Empty(t, result, "node unhealthy for less than threshold should not be returned")
}

func TestFindUnhealthyNodes_UnhealthyAboveThreshold(t *testing.T) {
	t.Parallel()
	pastTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: false, UnhealthySince: &pastTime},
		{Name: "w-2", Healthy: true},
		{Name: "w-3", Healthy: false, UnhealthySince: &pastTime},
	}
	// Threshold is 3 minutes, nodes have been unhealthy for 5 minutes
	result := findUnhealthyNodes(nodes, 3*time.Minute)
	require.Len(t, result, 2)
	assert.Equal(t, "w-1", result[0].Name)
	assert.Equal(t, "w-3", result[1].Name)
}

func TestFindUnhealthyNodes_NilUnhealthySince(t *testing.T) {
	t.Parallel()
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: false, UnhealthySince: nil},
	}
	// Unhealthy but no UnhealthySince timestamp - should be skipped
	result := findUnhealthyNodes(nodes, 3*time.Minute)
	assert.Empty(t, result, "nodes with nil UnhealthySince should be skipped")
}

func TestFindUnhealthyNodes_MixedStates(t *testing.T) {
	t.Parallel()
	oldTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	recentTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
	nodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", Healthy: true},                                   // healthy
		{Name: "w-2", Healthy: false, UnhealthySince: &oldTime},        // past threshold
		{Name: "w-3", Healthy: false, UnhealthySince: nil},             // no timestamp
		{Name: "w-4", Healthy: false, UnhealthySince: &recentTime},     // below threshold
		{Name: "w-5", Healthy: false, UnhealthySince: &oldTime},        // past threshold
	}
	result := findUnhealthyNodes(nodes, 3*time.Minute)
	require.Len(t, result, 2)
	assert.Equal(t, "w-2", result[0].Name)
	assert.Equal(t, "w-5", result[1].Name)
}

// --- normalizeServerSize (controller version) tests ---

func TestNormalizeServerSize_Controller(t *testing.T) {
	t.Parallel()
	// The controller version calls configv2.ServerSize().Normalize()
	tests := []struct {
		input    string
		expected string
	}{
		{"cx22", "cx23"},
		{"cx32", "cx33"},
		{"cx42", "cx43"},
		{"cx52", "cx53"},
		{"cx23", "cx23"},
		{"cpx31", "cpx31"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, normalizeServerSize(tt.input))
		})
	}
}

// --- waitForServerIP additional tests ---

func TestWaitForServerIP_ErrorOnInitialCallThenRetrySucceeds(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	calls := 0
	mockHCloud := &MockHCloudClient{
		GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
			calls++
			if calls == 1 {
				return "", fmt.Errorf("temporary error")
			}
			return "10.0.0.5", nil
		},
	}
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	ip, err := r.waitForServerIP(context.Background(), "test-server", 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.5", ip)
}

func TestWaitForServerIP_ErrorOnInitialCallEmptyOnRetryThenSuccess(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	calls := 0
	mockHCloud := &MockHCloudClient{
		GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
			calls++
			if calls == 1 {
				return "", fmt.Errorf("initial error")
			}
			if calls == 2 {
				return "", nil // empty, not yet assigned
			}
			if calls == 3 {
				return "", fmt.Errorf("transient error in ticker")
			}
			return "10.0.0.9", nil
		},
	}
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(mockHCloud),
	)

	ip, err := r.waitForServerIP(context.Background(), "test-server", 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.9", ip)
}

// --- WithNodeReadyWaiter option test ---

func TestWithNodeReadyWaiter(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	waiterCalled := false
	customWaiter := func(ctx context.Context, nodeName string, timeout time.Duration) error {
		waiterCalled = true
		return nil
	}

	r := NewClusterReconciler(client, scheme, recorder,
		WithNodeReadyWaiter(customWaiter),
	)

	assert.NotNil(t, r.nodeReadyWaiter)
	err := r.nodeReadyWaiter(context.Background(), "test-node", 10*time.Second)
	require.NoError(t, err)
	assert.True(t, waiterCalled)
}

// --- WithTalosConfigGenerator option test ---

func TestWithTalosConfigGenerator(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	mockGen := &MockTalosConfigGenerator{}

	r := NewClusterReconciler(client, scheme, recorder,
		WithTalosConfigGenerator(mockGen),
	)

	assert.Equal(t, mockGen, r.talosConfigGen)
}

// --- updateStatusWithRetry tests ---

func TestUpdateStatusWithRetry_SuccessFirstTry(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)

	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()
	recorder := record.NewFakeRecorder(10)

	r := NewClusterReconciler(client, scheme, recorder)

	cluster.Status.Phase = k8znerv1alpha1.ClusterPhaseRunning
	err := r.updateStatusWithRetry(context.Background(), cluster)
	require.NoError(t, err)
}

// --- checkStuckNodes additional test: node in phase without timeout entry ---

func TestCheckStuckNodes_PhaseWithoutTimeoutIsSkipped(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	pastTime := metav1.NewTime(time.Now().Add(-30 * time.Minute))
	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						// Use a phase that is NOT in phaseTimeouts map
						Name:                "w-1",
						Phase:               k8znerv1alpha1.NodePhase("SomeCustomPhase"),
						PhaseTransitionTime: &pastTime,
					},
				},
			},
		},
	}

	stuck := r.checkStuckNodes(t.Context(), cluster)
	assert.Empty(t, stuck, "node in a phase without timeout config should be skipped")
}

// --- updateNodePhase: test Ready phase sets Healthy=true ---

func TestUpdateNodePhase_ReadyPhaseMarkHealthy(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseWaitingForK8s, Healthy: false},
				},
			},
		},
	}

	r.updateNodePhase(t.Context(), cluster, "worker", nodeStatusUpdate{
		Name:  "w-1",
		Phase: k8znerv1alpha1.NodePhaseReady,
	})

	assert.True(t, cluster.Status.Workers.Nodes[0].Healthy, "Ready phase should mark node as healthy")
}

func TestUpdateNodePhase_NonReadyPhaseMarkUnhealthy(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			Workers: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{Name: "w-1", Phase: k8znerv1alpha1.NodePhaseReady, Healthy: true},
				},
			},
		},
	}

	r.updateNodePhase(t.Context(), cluster, "worker", nodeStatusUpdate{
		Name:  "w-1",
		Phase: k8znerv1alpha1.NodePhaseFailed,
	})

	assert.False(t, cluster.Status.Workers.Nodes[0].Healthy, "Failed phase should mark node as unhealthy")
}

// --- updateNodePhase: existing node update skips zero ServerID/PublicIP/PrivateIP ---

func TestUpdateNodePhase_PreservesExistingFieldsOnZeroUpdate(t *testing.T) {
	t.Parallel()
	scheme := setupTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	r := NewClusterReconciler(client, scheme, recorder,
		WithHCloudClient(&MockHCloudClient{}),
	)

	cluster := &k8znerv1alpha1.K8znerCluster{
		Status: k8znerv1alpha1.K8znerClusterStatus{
			ControlPlanes: k8znerv1alpha1.NodeGroupStatus{
				Nodes: []k8znerv1alpha1.NodeStatus{
					{
						Name:      "cp-1",
						ServerID:  12345,
						PublicIP:  "1.2.3.4",
						PrivateIP: "10.0.0.1",
						Phase:     k8znerv1alpha1.NodePhaseWaitingForK8s,
					},
				},
			},
		},
	}

	// Update with zero ServerID, empty IPs - should preserve existing values
	r.updateNodePhase(t.Context(), cluster, "control-plane", nodeStatusUpdate{
		Name:   "cp-1",
		Phase:  k8znerv1alpha1.NodePhaseReady,
		Reason: "Node ready",
		// ServerID=0, PublicIP="", PrivateIP="" - all zero/empty
	})

	node := cluster.Status.ControlPlanes.Nodes[0]
	assert.Equal(t, int64(12345), node.ServerID, "existing ServerID preserved when update has 0")
	assert.Equal(t, "1.2.3.4", node.PublicIP, "existing PublicIP preserved when update is empty")
	assert.Equal(t, "10.0.0.1", node.PrivateIP, "existing PrivateIP preserved when update is empty")
}
