package upgrade

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"

	hcloudAPI "github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTalosClient implements TalosConfigProducer for testing.
type mockTalosClient struct {
	GetNodeVersionFunc    func(ctx context.Context, endpoint string) (string, error)
	UpgradeNodeFunc       func(ctx context.Context, endpoint, imageURL string, opts provisioning.UpgradeOptions) error
	UpgradeKubernetesFunc func(ctx context.Context, endpoint, targetVersion string) error
	WaitForNodeReadyFunc  func(ctx context.Context, endpoint string, timeout time.Duration) error
	HealthCheckFunc       func(ctx context.Context, endpoint string) error
}

func (m *mockTalosClient) GetNodeVersion(ctx context.Context, endpoint string) (string, error) {
	if m.GetNodeVersionFunc != nil {
		return m.GetNodeVersionFunc(ctx, endpoint)
	}
	return "v1.8.3", nil
}

func (m *mockTalosClient) UpgradeNode(ctx context.Context, endpoint, imageURL string, opts provisioning.UpgradeOptions) error {
	if m.UpgradeNodeFunc != nil {
		return m.UpgradeNodeFunc(ctx, endpoint, imageURL, opts)
	}
	return nil
}

func (m *mockTalosClient) UpgradeKubernetes(ctx context.Context, endpoint, targetVersion string) error {
	if m.UpgradeKubernetesFunc != nil {
		return m.UpgradeKubernetesFunc(ctx, endpoint, targetVersion)
	}
	return nil
}

func (m *mockTalosClient) WaitForNodeReady(ctx context.Context, endpoint string, timeout time.Duration) error {
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
func (m *mockTalosClient) GenerateControlPlaneConfig(_ []string, _ string, _ int64) ([]byte, error) {
	return nil, nil
}

func (m *mockTalosClient) GenerateWorkerConfig(_ string, _ int64) ([]byte, error) {
	return nil, nil
}

func (m *mockTalosClient) GetClientConfig() ([]byte, error) {
	return nil, nil
}

func (m *mockTalosClient) SetEndpoint(_ string) {
}

func (m *mockTalosClient) GenerateAutoscalerConfig(_ string, _ map[string]string, _ []string) ([]byte, error) {
	return nil, nil
}

func (m *mockTalosClient) SetMachineConfigOptions(_ interface{}) {
}

func TestProvisionerName(t *testing.T) {
	t.Parallel()
	p := NewProvisioner(ProvisionerOptions{})
	assert.Equal(t, "Upgrade", p.Name())
}

func TestGetControlPlaneIPs(t *testing.T) {
	t.Parallel()
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
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
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
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
					},
				},
				{
					Name: "cp-2",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.11")},
					},
				},
				{
					Name: "cp-3",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.12")},
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
			t.Parallel()
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
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			assert.Equal(t, "test-cluster", labels["cluster"])
			assert.Equal(t, "worker", labels["role"])
			return []*hcloudAPI.Server{
				{
					Name: "worker-1",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.20")},
					},
				},
				{
					Name: "worker-2",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.21")},
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
	assert.Contains(t, ips, "1.2.3.20")
	assert.Contains(t, ips, "1.2.3.21")
}

func TestUpgradeControlPlane_SkipsNodesAlreadyAtTargetVersion(t *testing.T) {
	t.Parallel()
	nodeVersions := map[string]string{
		"1.2.3.10": "v1.8.3", // Already at target
		"1.2.3.11": "v1.8.2", // Needs upgrade
	}

	upgradeCallCount := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" {
				return []*hcloudAPI.Server{
					{
						Name: "cp-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
						},
					},
					{
						Name: "cp-2",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.11")},
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
		UpgradeNodeFunc: func(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
			upgradeCallCount++
			return nil
		},
		HealthCheckFunc: func(_ context.Context, _ string) error {
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
	t.Parallel()
	upgradeCallCount := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" {
				return []*hcloudAPI.Server{
					{
						Name: "cp-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
						},
					},
				}, nil
			}
			return []*hcloudAPI.Server{}, nil
		},
	}

	mockTalos := &mockTalosClient{
		UpgradeNodeFunc: func(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
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
	t.Parallel()
	nodeVersions := map[string]string{
		"1.2.3.20": "v1.8.3", // Already at target
		"1.2.3.21": "v1.8.2", // Needs upgrade
	}

	upgradeCallCount := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "worker" {
				return []*hcloudAPI.Server{
					{
						Name: "worker-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.20")},
						},
					},
					{
						Name: "worker-2",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.21")},
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
		UpgradeNodeFunc: func(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
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
	t.Parallel()
	upgradeCallCount := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		UpgradeKubernetesFunc: func(_ context.Context, _, _ string) error {
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
	t.Parallel()
	upgradeCallCount := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		UpgradeKubernetesFunc: func(_ context.Context, _, _ string) error {
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
	t.Parallel()
	healthCheckCalls := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		HealthCheckFunc: func(_ context.Context, _ string) error {
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
	t.Parallel()
	healthCheckCalls := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		HealthCheckFunc: func(_ context.Context, _ string) error {
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
	t.Parallel()
	var capturedImageURL string

	mockTalos := &mockTalosClient{
		UpgradeNodeFunc: func(_ context.Context, _, imageURL string, _ provisioning.UpgradeOptions) error {
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
	t.Parallel()
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

// --- Provision orchestrator tests ---

func TestProvision_ValidationError(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return nil, assert.AnError
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.Provision(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestProvision_ControlPlaneUpgradeError(t *testing.T) {
	t.Parallel()
	callCount := 0
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			callCount++
			// First call: getClusterServers (no role filter)
			// Second call: getControlPlaneIPs
			if labels["role"] == "control-plane" {
				return []*hcloudAPI.Server{
					{
						Name: "cp-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
						},
					},
				}, nil
			}
			// getClusterServers call (no role filter)
			return []*hcloudAPI.Server{{Name: "cp-1"}}, nil
		},
	}

	mockTalos := &mockTalosClient{
		GetNodeVersionFunc: func(_ context.Context, _ string) (string, error) {
			return "v1.8.2", nil // Needs upgrade
		},
		UpgradeNodeFunc: func(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
			return assert.AnError
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos:       config.TalosConfig{Version: "v1.8.3"},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{SkipHealthCheck: true})

	err := p.Provision(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "control plane upgrade failed")
}

func TestProvision_WorkerUpgradeError(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" {
				return []*hcloudAPI.Server{}, nil // No CP nodes to upgrade
			}
			if labels["role"] == "worker" {
				return []*hcloudAPI.Server{
					{
						Name: "worker-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.20")},
						},
					},
				}, nil
			}
			return []*hcloudAPI.Server{{Name: "worker-1"}}, nil
		},
	}

	mockTalos := &mockTalosClient{
		GetNodeVersionFunc: func(_ context.Context, _ string) (string, error) {
			return "v1.8.2", nil
		},
		UpgradeNodeFunc: func(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
			return assert.AnError
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos:       config.TalosConfig{Version: "v1.8.3"},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{SkipHealthCheck: true})

	err := p.Provision(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker upgrade failed")
}

func TestProvision_KubernetesUpgradeError(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" {
				return []*hcloudAPI.Server{
					{
						Name: "cp-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
						},
					},
				}, nil
			}
			if labels["role"] == "worker" {
				return []*hcloudAPI.Server{}, nil
			}
			return []*hcloudAPI.Server{{Name: "cp-1"}}, nil
		},
	}

	mockTalos := &mockTalosClient{
		GetNodeVersionFunc: func(_ context.Context, _ string) (string, error) {
			return "v1.8.3", nil // Already at target, skip node upgrade
		},
		UpgradeKubernetesFunc: func(_ context.Context, _, _ string) error {
			return assert.AnError
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos:       config.TalosConfig{Version: "v1.8.3"},
		Kubernetes:  config.KubernetesConfig{Version: "v1.32.1"},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{SkipHealthCheck: true})

	err := p.Provision(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kubernetes upgrade failed")
}

func TestProvision_HealthCheckError(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" {
				return []*hcloudAPI.Server{
					{
						Name: "cp-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
						},
					},
				}, nil
			}
			if labels["role"] == "worker" {
				return []*hcloudAPI.Server{}, nil
			}
			return []*hcloudAPI.Server{{Name: "cp-1"}}, nil
		},
	}

	mockTalos := &mockTalosClient{
		GetNodeVersionFunc: func(_ context.Context, _ string) (string, error) {
			return "v1.8.3", nil // Already at target
		},
		HealthCheckFunc: func(_ context.Context, _ string) error {
			return assert.AnError
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos:       config.TalosConfig{Version: "v1.8.3"},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{}) // SkipHealthCheck=false

	err := p.Provision(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "final health check failed")
}

func TestProvision_SkipsHealthCheckOnDryRun(t *testing.T) {
	t.Parallel()
	healthCheckCalls := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" || labels["role"] == "worker" {
				return []*hcloudAPI.Server{}, nil
			}
			return []*hcloudAPI.Server{{Name: "cp-1"}}, nil
		},
	}

	mockTalos := &mockTalosClient{
		HealthCheckFunc: func(_ context.Context, _ string) error {
			healthCheckCalls++
			return nil
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{DryRun: true})

	err := p.Provision(pCtx)
	require.NoError(t, err)
	assert.Equal(t, 0, healthCheckCalls)
}

func TestProvision_SkipsHealthCheckWhenFlagSet(t *testing.T) {
	t.Parallel()
	healthCheckCalls := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" || labels["role"] == "worker" {
				return []*hcloudAPI.Server{}, nil
			}
			return []*hcloudAPI.Server{{Name: "cp-1"}}, nil
		},
	}

	mockTalos := &mockTalosClient{
		HealthCheckFunc: func(_ context.Context, _ string) error {
			healthCheckCalls++
			return nil
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{SkipHealthCheck: true})

	err := p.Provision(pCtx)
	require.NoError(t, err)
	assert.Equal(t, 0, healthCheckCalls)
}

func TestProvision_SuccessfulFullRun(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" {
				return []*hcloudAPI.Server{
					{
						Name: "cp-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
						},
					},
				}, nil
			}
			if labels["role"] == "worker" {
				return []*hcloudAPI.Server{}, nil
			}
			return []*hcloudAPI.Server{{Name: "cp-1"}}, nil
		},
	}

	mockTalos := &mockTalosClient{
		GetNodeVersionFunc: func(_ context.Context, _ string) (string, error) {
			return "v1.8.3", nil // Already at target
		},
		HealthCheckFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos:       config.TalosConfig{Version: "v1.8.3"},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.Provision(pCtx)
	require.NoError(t, err)
}

// --- validateAndPrepare tests ---

func TestValidateAndPrepare_EmptyCluster(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{}, nil
		},
	}

	cfg := &config.Config{ClusterName: "empty-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.validateAndPrepare(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no servers found")
}

func TestValidateAndPrepare_DryRunCallsDryRunReport(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{{Name: "cp-1"}}, nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos:       config.TalosConfig{Version: "v1.8.3"},
		Kubernetes:  config.KubernetesConfig{Version: "v1.32.1"},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{DryRun: true})

	// dryRunReport always returns nil
	err := p.validateAndPrepare(pCtx)
	require.NoError(t, err)
}

func TestValidateAndPrepare_GetServersError(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return nil, assert.AnError
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.validateAndPrepare(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get cluster servers")
}

// --- getClusterServers tests ---

func TestGetClusterServers_Success(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			assert.Equal(t, "test-cluster", labels["cluster"])
			return []*hcloudAPI.Server{
				{Name: "cp-1"},
				{Name: "worker-1"},
				{Name: "worker-2"},
			}, nil
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	names, err := p.getClusterServers(pCtx)
	require.NoError(t, err)
	assert.Len(t, names, 3)
	assert.Contains(t, names, "cp-1")
	assert.Contains(t, names, "worker-1")
	assert.Contains(t, names, "worker-2")
}

func TestGetClusterServers_Error(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return nil, assert.AnError
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	names, err := p.getClusterServers(pCtx)
	require.Error(t, err)
	assert.Nil(t, names)
	assert.Contains(t, err.Error(), "failed to query servers")
}

// --- Error path tests ---

func TestGetControlPlaneIPs_Error(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return nil, assert.AnError
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	ips, err := p.getControlPlaneIPs(pCtx)
	require.Error(t, err)
	assert.Nil(t, ips)
	assert.Contains(t, err.Error(), "failed to query control plane servers")
}

func TestGetWorkerIPs_Error(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return nil, assert.AnError
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	ips, err := p.getWorkerIPs(pCtx)
	require.Error(t, err)
	assert.Nil(t, ips)
	assert.Contains(t, err.Error(), "failed to query worker servers")
}

func TestUpgradeControlPlane_GetIPsError(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" {
				return nil, assert.AnError
			}
			return []*hcloudAPI.Server{}, nil
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster", Talos: config.TalosConfig{Version: "v1.8.3"}}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeControlPlane(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get control plane IPs")
}

func TestUpgradeControlPlane_NoNodes(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{}, nil
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster", Talos: config.TalosConfig{Version: "v1.8.3"}}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeControlPlane(pCtx)
	require.NoError(t, err) // Skips gracefully
}

func TestUpgradeControlPlane_GetVersionError(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" {
				return []*hcloudAPI.Server{
					{
						Name: "cp-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
						},
					},
				}, nil
			}
			return []*hcloudAPI.Server{}, nil
		},
	}

	mockTalos := &mockTalosClient{
		GetNodeVersionFunc: func(_ context.Context, _ string) (string, error) {
			return "", assert.AnError
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster", Talos: config.TalosConfig{Version: "v1.8.3"}}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeControlPlane(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get version for node")
}

func TestUpgradeControlPlane_UpgradeNodeError(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "control-plane" {
				return []*hcloudAPI.Server{
					{
						Name: "cp-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
						},
					},
				}, nil
			}
			return []*hcloudAPI.Server{}, nil
		},
	}

	mockTalos := &mockTalosClient{
		GetNodeVersionFunc: func(_ context.Context, _ string) (string, error) {
			return "v1.8.2", nil // Needs upgrade
		},
		UpgradeNodeFunc: func(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
			return assert.AnError
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster", Talos: config.TalosConfig{Version: "v1.8.3"}}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{SkipHealthCheck: true})

	err := p.upgradeControlPlane(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upgrade node")
}

func TestUpgradeWorkers_GetIPsError(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "worker" {
				return nil, assert.AnError
			}
			return []*hcloudAPI.Server{}, nil
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster", Talos: config.TalosConfig{Version: "v1.8.3"}}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeWorkers(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get worker IPs")
}

func TestUpgradeWorkers_NoNodes(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{}, nil
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster", Talos: config.TalosConfig{Version: "v1.8.3"}}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeWorkers(pCtx)
	require.NoError(t, err) // Skips gracefully
}

func TestUpgradeWorkers_DryRun(t *testing.T) {
	t.Parallel()
	upgradeCallCount := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "worker" {
				return []*hcloudAPI.Server{
					{
						Name: "worker-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.20")},
						},
					},
				}, nil
			}
			return []*hcloudAPI.Server{}, nil
		},
	}

	mockTalos := &mockTalosClient{
		UpgradeNodeFunc: func(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
			upgradeCallCount++
			return nil
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster", Talos: config.TalosConfig{Version: "v1.8.3"}}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{DryRun: true})

	err := p.upgradeWorkers(pCtx)
	require.NoError(t, err)
	assert.Equal(t, 0, upgradeCallCount)
}

func TestUpgradeWorkers_GetVersionError(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "worker" {
				return []*hcloudAPI.Server{
					{
						Name: "worker-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.20")},
						},
					},
				}, nil
			}
			return []*hcloudAPI.Server{}, nil
		},
	}

	mockTalos := &mockTalosClient{
		GetNodeVersionFunc: func(_ context.Context, _ string) (string, error) {
			return "", assert.AnError
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster", Talos: config.TalosConfig{Version: "v1.8.3"}}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeWorkers(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get version for node")
}

func TestUpgradeWorkers_UpgradeNodeError(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, labels map[string]string) ([]*hcloudAPI.Server, error) {
			if labels["role"] == "worker" {
				return []*hcloudAPI.Server{
					{
						Name: "worker-1",
						PublicNet: hcloudAPI.ServerPublicNet{
							IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.20")},
						},
					},
				}, nil
			}
			return []*hcloudAPI.Server{}, nil
		},
	}

	mockTalos := &mockTalosClient{
		GetNodeVersionFunc: func(_ context.Context, _ string) (string, error) {
			return "v1.8.2", nil
		},
		UpgradeNodeFunc: func(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
			return assert.AnError
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster", Talos: config.TalosConfig{Version: "v1.8.3"}}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeWorkers(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upgrade node")
}

// --- upgradeKubernetes additional tests ---

func TestUpgradeKubernetes_Success(t *testing.T) {
	t.Parallel()
	var capturedEndpoint, capturedVersion string

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		UpgradeKubernetesFunc: func(_ context.Context, endpoint, targetVersion string) error {
			capturedEndpoint = endpoint
			capturedVersion = targetVersion
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Kubernetes:  config.KubernetesConfig{Version: "v1.32.1"},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeKubernetes(pCtx)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3.10", capturedEndpoint)
	assert.Equal(t, "v1.32.1", capturedVersion)
}

func TestUpgradeKubernetes_Error(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		UpgradeKubernetesFunc: func(_ context.Context, _, _ string) error {
			return assert.AnError
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Kubernetes:  config.KubernetesConfig{Version: "v1.32.1"},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeKubernetes(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upgrade Kubernetes")
}

func TestUpgradeKubernetes_NoCPNodes(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return nil, assert.AnError // Error getting CP nodes
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Kubernetes:  config.KubernetesConfig{Version: "v1.32.1"},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeKubernetes(pCtx)
	require.NoError(t, err) // Skips gracefully
}

// --- upgradeNode additional tests ---

func TestUpgradeNode_WithoutSchematic(t *testing.T) {
	t.Parallel()
	var capturedImageURL string

	mockTalos := &mockTalosClient{
		UpgradeNodeFunc: func(_ context.Context, _, imageURL string, _ provisioning.UpgradeOptions) error {
			capturedImageURL = imageURL
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos: config.TalosConfig{
			Version:     "v1.8.3",
			SchematicID: "", // No schematic
		},
	}

	pCtx := provisioning.NewContext(context.Background(), cfg, nil, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeNode(pCtx, "10.0.0.10", "worker")
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/siderolabs/installer:v1.8.3", capturedImageURL)
}

func TestUpgradeNode_UpgradeError(t *testing.T) {
	t.Parallel()
	mockTalos := &mockTalosClient{
		UpgradeNodeFunc: func(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
			return assert.AnError
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos:       config.TalosConfig{Version: "v1.8.3"},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, nil, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeNode(pCtx, "10.0.0.10", "worker")
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestUpgradeNode_WaitForNodeReadyError(t *testing.T) {
	t.Parallel()
	mockTalos := &mockTalosClient{
		UpgradeNodeFunc: func(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
			return nil
		},
		WaitForNodeReadyFunc: func(_ context.Context, _ string, _ time.Duration) error {
			return assert.AnError
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos:       config.TalosConfig{Version: "v1.8.3"},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, nil, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeNode(pCtx, "10.0.0.10", "control-plane")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "node failed to become ready")
}

func TestUpgradeNode_PassesUpgradeOptions(t *testing.T) {
	t.Parallel()
	var capturedOpts provisioning.UpgradeOptions

	mockTalos := &mockTalosClient{
		UpgradeNodeFunc: func(_ context.Context, _, _ string, opts provisioning.UpgradeOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Talos: config.TalosConfig{
			Version: "v1.8.3",
			Upgrade: config.UpgradeConfig{
				Stage: true,
				Force: true,
			},
		},
	}
	pCtx := provisioning.NewContext(context.Background(), cfg, nil, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.upgradeNode(pCtx, "10.0.0.10", "worker")
	require.NoError(t, err)
	assert.True(t, capturedOpts.Stage)
	assert.True(t, capturedOpts.Force)
}

// --- healthCheck additional tests ---

func TestHealthCheck_NoCPNodes(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{}, nil
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, nil)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.healthCheck(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no control plane nodes available")
}

func TestHealthCheck_Error(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		HealthCheckFunc: func(_ context.Context, _ string) error {
			return assert.AnError
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	err := p.healthCheck(pCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestHealthCheckWithRetry_AllFail(t *testing.T) {
	t.Parallel()
	healthCheckCalls := 0

	mockClient := &hcloud.MockClient{
		GetServersByLabelFunc: func(_ context.Context, _ map[string]string) ([]*hcloudAPI.Server, error) {
			return []*hcloudAPI.Server{
				{
					Name: "cp-1",
					PublicNet: hcloudAPI.ServerPublicNet{
						IPv4: hcloudAPI.ServerPublicNetIPv4{IP: net.ParseIP("1.2.3.10")},
					},
				},
			}, nil
		},
	}

	mockTalos := &mockTalosClient{
		HealthCheckFunc: func(_ context.Context, _ string) error {
			healthCheckCalls++
			return assert.AnError
		},
	}

	cfg := &config.Config{ClusterName: "test-cluster"}
	pCtx := provisioning.NewContext(context.Background(), cfg, mockClient, mockTalos)
	p := NewProvisioner(ProvisionerOptions{})

	// Use maxRetries=1 to avoid sleep delays
	err := p.healthCheckWithRetry(pCtx, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "health check failed after 1 attempts")
	assert.Equal(t, 1, healthCheckCalls)
}
