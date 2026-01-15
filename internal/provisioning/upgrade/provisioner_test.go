package upgrade

import (
	"context"
	"net"
	"testing"

	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning"

	hcloudAPI "github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTalosClient implements TalosConfigProducer for testing.
type mockTalosClient struct {
	GetNodeVersionFunc     func(ctx context.Context, endpoint string) (string, error)
	GetSchematicIDFunc     func(ctx context.Context, endpoint string) (string, error)
	UpgradeNodeFunc        func(ctx context.Context, endpoint, imageURL string) error
	UpgradeKubernetesFunc  func(ctx context.Context, endpoint, targetVersion string) error
	WaitForNodeReadyFunc   func(ctx context.Context, endpoint string, timeout interface{}) error
	HealthCheckFunc        func(ctx context.Context, endpoint string) error
}

func (m *mockTalosClient) GetNodeVersion(ctx context.Context, endpoint string) (string, error) {
	if m.GetNodeVersionFunc != nil {
		return m.GetNodeVersionFunc(ctx, endpoint)
	}
	return "v1.8.3", nil
}

func (m *mockTalosClient) GetSchematicID(ctx context.Context, endpoint string) (string, error) {
	if m.GetSchematicIDFunc != nil {
		return m.GetSchematicIDFunc(ctx, endpoint)
	}
	return "abc123", nil
}

func (m *mockTalosClient) UpgradeNode(ctx context.Context, endpoint, imageURL string) error {
	if m.UpgradeNodeFunc != nil {
		return m.UpgradeNodeFunc(ctx, endpoint, imageURL)
	}
	return nil
}

func (m *mockTalosClient) UpgradeKubernetes(ctx context.Context, endpoint, targetVersion string) error {
	if m.UpgradeKubernetesFunc != nil {
		return m.UpgradeKubernetesFunc(ctx, endpoint, targetVersion)
	}
	return nil
}

func (m *mockTalosClient) WaitForNodeReady(ctx context.Context, endpoint string, timeout interface{}) error {
	if m.WaitForNodeReadyFunc != nil {
		return m.WaitForNodeReadyFunc(ctx, endpoint, timeout)
	}
	return nil
}

func (m *mockTalosClient) HealthCheck(ctx context.Context, endpoint string) error {
	if m.HealthCheckFunc != nil {
		return m.HealthCheckFunc(ctx, endpoint)
	}
	return nil
}

// Implement other required TalosConfigProducer methods as stubs.
func (m *mockTalosClient) GenerateControlPlaneConfig(_ int) ([]byte, error) {
	return nil, nil
}

func (m *mockTalosClient) GenerateWorkerConfig() ([]byte, error) {
	return nil, nil
}

func (m *mockTalosClient) GetClientConfig() (interface{}, error) {
	return nil, nil
}

func (m *mockTalosClient) GetKubeconfig() ([]byte, error) {
	return nil, nil
}

func (m *mockTalosClient) GenerateAutoscalerConfig(_ config.AutoscalerNodePool) ([]byte, error) {
	return nil, nil
}

func TestProvisionerName(t *testing.T) {
	p := NewProvisioner(ProvisionerOptions{})
	assert.Equal(t, "Upgrade", p.Name())
}

func TestGetControlPlaneIPs(t *testing.T) {
	tests := []struct {
		name          string
		servers       []*hcloudAPI.Server
		expectError   bool
		expectedCount int
	}{
		{
			name: "single control plane node",
			servers: []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PrivateNet: []hcloudAPI.ServerPrivateNet{
						{IP: net.ParseIP("10.0.0.10")},
					},
				},
			},
			expectError:   false,
			expectedCount: 1,
		},
		{
			name: "multiple control plane nodes",
			servers: []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PrivateNet: []hcloudAPI.ServerPrivateNet{
						{IP: net.ParseIP("10.0.0.10")},
					},
				},
				{
					Name: "cp-2",
					PrivateNet: []hcloudAPI.ServerPrivateNet{
						{IP: net.ParseIP("10.0.0.11")},
					},
				},
				{
					Name: "cp-3",
					PrivateNet: []hcloudAPI.ServerPrivateNet{
						{IP: net.ParseIP("10.0.0.12")},
					},
				},
			},
			expectError:   false,
			expectedCount: 3,
		},
		{
			name:          "no control plane nodes",
			servers:       []*hcloudAPI.Server{},
			expectError:   false,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &hcloud.MockClient{
				GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
					assert.Equal(t, "test-cluster", labels["cluster"])
					assert.Equal(t, "control-plane", labels["role"])
					return tt.servers, nil
				},
			}

			cfg := &config.Config{
				ClusterName: "test-cluster",
			}

			pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
			provisioner := NewProvisioner(ProvisionerOptions{})

			ips, err := provisioner.getControlPlaneIPs(pCtx)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, ips, tt.expectedCount)
			}
		})
	}
}

func TestGetWorkerIPs(t *testing.T) {
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			assert.Equal(t, "test-cluster", labels["cluster"])
			assert.Equal(t, "worker", labels["role"])
			return []*hcloudAPI.Server{
				{
					Name: "worker-1",
					PrivateNet: []hcloudAPI.ServerPrivateNet{
						{IP: net.ParseIP("10.0.1.10")},
					},
				},
				{
					Name: "worker-2",
					PrivateNet: []hcloudAPI.ServerPrivateNet{
						{IP: net.ParseIP("10.0.1.11")},
					},
				},
			}, nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	provisioner := NewProvisioner(ProvisionerOptions{})

	ips, err := provisioner.getWorkerIPs(pCtx)

	require.NoError(t, err)
	assert.Len(t, ips, 2)
	assert.Contains(t, ips, "10.0.1.10")
	assert.Contains(t, ips, "10.0.1.11")
}

func TestUpgradeControlPlane_SkipsNodesAlreadyAtTargetVersion(t *testing.T) {
	nodeVersions := map[string]string{
		"10.0.0.10": "v1.8.3", // Already at target
		"10.0.0.11": "v1.8.2", // Needs upgrade
	}

	upgradeCallCount := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" {
				return []*hcloudAPI.Server{
					{
						Name: "cp-1",
						PrivateNet: []hcloudAPI.ServerPrivateNet{
							{IP: net.ParseIP("10.0.0.10")},
						},
					},
					{
						Name: "cp-2",
						PrivateNet: []hcloudAPI.ServerPrivateNet{
							{IP: net.ParseIP("10.0.0.11")},
						},
					},
				}, nil
			}
			return []*hcloudAPI.Server{}, nil
		},
	}

	mockTalos := &mockTalosClient{
		GetNodeVersionFunc: func(_ context.Context, endpoint string) (string, error) {
			return nodeVersions[endpoint], nil
		},
		UpgradeNodeFunc: func(_ context.Context, endpoint, imageURL string) error {
			upgradeCallCount++
			return nil
		},
		HealthCheckFunc: func(_ context.Context, endpoint string) error {
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos: config.TalosConfig{
			Version:     "v1.8.3",
			SchematicID: "abc123",
		},
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	provisioner := NewProvisioner(ProvisionerOptions{})

	err := provisioner.upgradeControlPlane(pCtx)

	require.NoError(t, err)
	// Only one node should be upgraded (the one at v1.8.2)
	assert.Equal(t, 1, upgradeCallCount)
}

func TestUpgradeControlPlane_DryRun(t *testing.T) {
	upgradeCallCount := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" {
				return []*hcloudAPI.Server{
					{
						Name: "cp-1",
						PrivateNet: []hcloudAPI.ServerPrivateNet{
							{IP: net.ParseIP("10.0.0.10")},
						},
					},
				}, nil
			}
			return []*hcloudAPI.Server{}, nil
		},
	}

	mockTalos := &mockTalosClient{
		UpgradeNodeFunc: func(_ context.Context, endpoint, imageURL string) error {
			upgradeCallCount++
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos: config.TalosConfig{
			Version:     "v1.8.3",
			SchematicID: "abc123",
		},
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	provisioner := NewProvisioner(ProvisionerOptions{
		DryRun: true,
	})

	err := provisioner.upgradeControlPlane(pCtx)

	require.NoError(t, err)
	// No upgrades should be performed in dry run
	assert.Equal(t, 0, upgradeCallCount)
}

func TestUpgradeWorkers_SkipsNodesAlreadyAtTargetVersion(t *testing.T) {
	nodeVersions := map[string]string{
		"10.0.1.10": "v1.8.3", // Already at target
		"10.0.1.11": "v1.8.2", // Needs upgrade
	}

	upgradeCallCount := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "worker" {
				return []*hcloudAPI.Server{
					{
						Name: "worker-1",
						PrivateNet: []hcloudAPI.ServerPrivateNet{
							{IP: net.ParseIP("10.0.1.10")},
						},
					},
					{
						Name: "worker-2",
						PrivateNet: []hcloudAPI.ServerPrivateNet{
							{IP: net.ParseIP("10.0.1.11")},
						},
					},
				}, nil
			}
			return []*hcloudAPI.Server{}, nil
		},
	}

	mockTalos := &mockTalosClient{
		GetNodeVersionFunc: func(_ context.Context, endpoint string) (string, error) {
			return nodeVersions[endpoint], nil
		},
		UpgradeNodeFunc: func(_ context.Context, endpoint, imageURL string) error {
			upgradeCallCount++
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos: config.TalosConfig{
			Version:     "v1.8.3",
			SchematicID: "abc123",
		},
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	provisioner := NewProvisioner(ProvisionerOptions{})

	err := provisioner.upgradeWorkers(pCtx)

	require.NoError(t, err)
	// Only one worker should be upgraded
	assert.Equal(t, 1, upgradeCallCount)
}

func TestUpgradeKubernetes_SkipsIfNoVersionSpecified(t *testing.T) {
	upgradeCallCount := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PrivateNet: []hcloudAPI.ServerPrivateNet{
						{IP: net.ParseIP("10.0.0.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		UpgradeKubernetesFunc: func(_ context.Context, endpoint, targetVersion string) error {
			upgradeCallCount++
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Kubernetes: config.KubernetesConfig{
			Version: "", // No version specified
		},
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	provisioner := NewProvisioner(ProvisionerOptions{})

	err := provisioner.upgradeKubernetes(pCtx)

	require.NoError(t, err)
	// No upgrade should be performed
	assert.Equal(t, 0, upgradeCallCount)
}

func TestUpgradeKubernetes_DryRun(t *testing.T) {
	upgradeCallCount := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PrivateNet: []hcloudAPI.ServerPrivateNet{
						{IP: net.ParseIP("10.0.0.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		UpgradeKubernetesFunc: func(_ context.Context, endpoint, targetVersion string) error {
			upgradeCallCount++
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.1",
		},
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	provisioner := NewProvisioner(ProvisionerOptions{
		DryRun: true,
	})

	err := provisioner.upgradeKubernetes(pCtx)

	require.NoError(t, err)
	// No upgrade should be performed in dry run
	assert.Equal(t, 0, upgradeCallCount)
}

func TestHealthCheckWithRetry_SucceedsOnFirstAttempt(t *testing.T) {
	healthCheckCalls := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PrivateNet: []hcloudAPI.ServerPrivateNet{
						{IP: net.ParseIP("10.0.0.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		HealthCheckFunc: func(_ context.Context, endpoint string) error {
			healthCheckCalls++
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	provisioner := NewProvisioner(ProvisionerOptions{})

	err := provisioner.healthCheckWithRetry(pCtx, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, healthCheckCalls)
}

func TestHealthCheckWithRetry_SucceedsOnSecondAttempt(t *testing.T) {
	healthCheckCalls := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PrivateNet: []hcloudAPI.ServerPrivateNet{
						{IP: net.ParseIP("10.0.0.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		HealthCheckFunc: func(_ context.Context, endpoint string) error {
			healthCheckCalls++
			if healthCheckCalls == 1 {
				return assert.AnError
			}
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	provisioner := NewProvisioner(ProvisionerOptions{})

	err := provisioner.healthCheckWithRetry(pCtx, 3)

	require.NoError(t, err)
	assert.Equal(t, 2, healthCheckCalls)
}

func TestUpgradeNode_BuildsCorrectImageURL(t *testing.T) {
	var capturedImageURL string

	mockTalos := &mockTalosClient{
		UpgradeNodeFunc: func(_ context.Context, endpoint, imageURL string) error {
			capturedImageURL = imageURL
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos: config.TalosConfig{
			Version:     "v1.8.3",
			SchematicID: "abc123def456",
		},
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, nil, mockTalos)
	provisioner := NewProvisioner(ProvisionerOptions{})

	err := provisioner.upgradeNode(pCtx, "10.0.0.10", "control-plane")

	require.NoError(t, err)
	assert.Equal(t, "factory.talos.dev/installer/abc123def456:v1.8.3", capturedImageURL)
}

func TestDryRunReport(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos: config.TalosConfig{
			Version: "v1.8.3",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.32.1",
		},
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, nil, nil)
	provisioner := NewProvisioner(ProvisionerOptions{})

	servers := []string{"cp-1", "cp-2", "worker-1"}

	err := provisioner.dryRunReport(pCtx, servers)

	require.NoError(t, err)
}
