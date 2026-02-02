package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/config"
)

func TestIsNodeHealthy(t *testing.T) {
	tests := []struct {
		name     string
		node     *corev1.Node
		expected bool
	}{
		{
			name: "node is ready",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
			expected: true,
		},
		{
			name: "node is not ready",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
					},
				},
			},
			expected: false,
		},
		{
			name: "node ready unknown",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionUnknown},
					},
				},
			},
			expected: false,
		},
		{
			name: "no ready condition",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
					},
				},
			},
			expected: false,
		},
		{
			name: "empty conditions",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNodeHealthy(tt.node)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCountHealthy(t *testing.T) {
	tests := []struct {
		name     string
		nodes    []k8znerv1alpha1.NodeStatus
		expected int
	}{
		{
			name:     "empty list",
			nodes:    []k8znerv1alpha1.NodeStatus{},
			expected: 0,
		},
		{
			name: "all healthy",
			nodes: []k8znerv1alpha1.NodeStatus{
				{Name: "node1", Healthy: true},
				{Name: "node2", Healthy: true},
				{Name: "node3", Healthy: true},
			},
			expected: 3,
		},
		{
			name: "all unhealthy",
			nodes: []k8znerv1alpha1.NodeStatus{
				{Name: "node1", Healthy: false},
				{Name: "node2", Healthy: false},
			},
			expected: 0,
		},
		{
			name: "mixed",
			nodes: []k8znerv1alpha1.NodeStatus{
				{Name: "node1", Healthy: true},
				{Name: "node2", Healthy: false},
				{Name: "node3", Healthy: true},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countHealthy(tt.nodes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAlreadyExistsError(t *testing.T) {
	// The current implementation checks string prefix, test it directly
	assert.False(t, isAlreadyExistsError(nil))
	assert.False(t, isAlreadyExistsError(assert.AnError))
}

func TestBuildMigrationCRD(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "nbg1",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Count: 3, ServerType: "cx23"},
			},
		},
		Workers: []config.WorkerNodePool{
			{Count: 5, ServerType: "cx33"},
		},
		Network: config.NetworkConfig{
			IPv4CIDR:        "10.0.0.0/16",
			PodIPv4CIDR:     "10.0.128.0/17",
			ServiceIPv4CIDR: "10.96.0.0/12",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "1.32.0",
		},
		Talos: config.TalosConfig{
			Version:     "v1.9.0",
			SchematicID: "abc123",
			Extensions:  []string{"siderolabs/qemu-guest-agent"},
		},
		Addons: config.AddonsConfig{
			Traefik:     config.TraefikConfig{Enabled: true},
			CertManager: config.CertManagerConfig{Enabled: true},
		},
	}

	infra := &InfrastructureInfo{
		NetworkID:      123,
		NetworkName:    "test-cluster-net",
		FirewallID:     456,
		FirewallName:   "test-cluster-fw",
		LoadBalancerID: 789,
		LoadBalancerIP: "1.2.3.4",
		SSHKeyID:       111,
	}

	cpNodes := []k8znerv1alpha1.NodeStatus{
		{Name: "cp-1", ServerID: 1, PublicIP: "1.1.1.1", Healthy: true},
		{Name: "cp-2", ServerID: 2, PublicIP: "1.1.1.2", Healthy: true},
		{Name: "cp-3", ServerID: 3, PublicIP: "1.1.1.3", Healthy: false},
	}

	workerNodes := []k8znerv1alpha1.NodeStatus{
		{Name: "w-1", ServerID: 10, Healthy: true},
		{Name: "w-2", ServerID: 11, Healthy: true},
	}

	cluster := buildMigrationCRD(cfg, infra, cpNodes, workerNodes)

	require.NotNil(t, cluster)

	// Verify metadata
	assert.Equal(t, "test-cluster", cluster.Name)
	assert.Equal(t, k8znerNamespace, cluster.Namespace)
	assert.Equal(t, "test-cluster", cluster.Labels["cluster"])

	// Verify spec
	assert.Equal(t, "nbg1", cluster.Spec.Region)
	assert.Equal(t, 3, cluster.Spec.ControlPlanes.Count)
	assert.Equal(t, "cx23", cluster.Spec.ControlPlanes.Size)
	assert.Equal(t, 2, cluster.Spec.Workers.Count)
	assert.Equal(t, "cx33", cluster.Spec.Workers.Size)

	// Verify bootstrap state
	require.NotNil(t, cluster.Spec.Bootstrap)
	assert.True(t, cluster.Spec.Bootstrap.Completed)
	assert.Equal(t, "cp-1", cluster.Spec.Bootstrap.BootstrapNode)
	assert.Equal(t, int64(1), cluster.Spec.Bootstrap.BootstrapNodeID)
	assert.Equal(t, "1.1.1.1", cluster.Spec.Bootstrap.PublicIP)

	// Verify status
	assert.Equal(t, k8znerv1alpha1.ClusterPhaseRunning, cluster.Status.Phase)
	assert.Equal(t, k8znerv1alpha1.PhaseComplete, cluster.Status.ProvisioningPhase)
	assert.Equal(t, 3, cluster.Status.ControlPlanes.Desired)
	assert.Equal(t, 2, cluster.Status.ControlPlanes.Ready) // 2 healthy
	assert.Equal(t, 2, cluster.Status.Workers.Desired)
	assert.Equal(t, 2, cluster.Status.Workers.Ready) // 2 healthy

	// Verify infrastructure
	assert.Equal(t, int64(123), cluster.Status.Infrastructure.NetworkID)
	assert.Equal(t, int64(456), cluster.Status.Infrastructure.FirewallID)
	assert.Equal(t, int64(789), cluster.Status.Infrastructure.LoadBalancerID)
	assert.Equal(t, "1.2.3.4", cluster.Status.Infrastructure.LoadBalancerIP)
}

func TestBuildMigrationCRD_EmptyNodes(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "empty-cluster",
		Location:    "fsn1",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Count: 1, ServerType: "cx23"},
			},
		},
		Network:    config.NetworkConfig{},
		Kubernetes: config.KubernetesConfig{},
		Talos:      config.TalosConfig{},
	}

	infra := &InfrastructureInfo{}

	cluster := buildMigrationCRD(cfg, infra, nil, nil)

	require.NotNil(t, cluster)
	assert.Equal(t, 0, cluster.Spec.ControlPlanes.Count)
	assert.Equal(t, 0, cluster.Spec.Workers.Count)

	// Bootstrap should have empty values
	require.NotNil(t, cluster.Spec.Bootstrap)
	assert.Empty(t, cluster.Spec.Bootstrap.BootstrapNode)
	assert.Empty(t, cluster.Spec.Bootstrap.PublicIP)
}

func TestPrintMigrationPlan(t *testing.T) {
	cfg := &config.Config{ClusterName: "test"}
	infra := &InfrastructureInfo{
		NetworkID:      1,
		FirewallID:     2,
		LoadBalancerID: 3,
	}
	cpNodes := []k8znerv1alpha1.NodeStatus{{Name: "cp", Healthy: true}}
	workerNodes := []k8znerv1alpha1.NodeStatus{{Name: "w", Healthy: false}}

	// Should not panic
	printMigrationPlan(cfg, infra, cpNodes, workerNodes)
}

func TestPrintMigrationSuccess(t *testing.T) {
	// Should not panic
	printMigrationSuccess("test-cluster")
}
