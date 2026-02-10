package cluster

import (
	"context"
	"fmt"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	hcloud_internal "github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- configureAndWaitForNewCPs / configureAndWaitForNewWorkers tests ---

func TestConfigureAndWaitForNewCPs_Empty(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Observer: observer,
		Logger:   observer,
		Timeouts: config.TestTimeouts(),
	}

	err := p.configureAndWaitForNewCPs(pCtx, map[string]string{})
	require.NoError(t, err)
}

func TestConfigureAndWaitForNewWorkers_Empty(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Observer: observer,
		Logger:   observer,
		Timeouts: config.TestTimeouts(),
	}

	err := p.configureAndWaitForNewWorkers(pCtx, map[string]string{})
	require.NoError(t, err)
}

func TestConfigureAndWaitForNewCPs_GenerateConfigError(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	mockTalos := &mockTalosConfigProducer{
		generateControlPlaneConfigFn: func(_ []string, hostname string, _ int64) ([]byte, error) {
			return nil, fmt.Errorf("config generation failed for %s", hostname)
		},
	}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Talos:    mockTalos,
		Observer: observer,
		Logger:   observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.ControlPlaneServerIDs = map[string]int64{"cp-new": 123}

	newCPs := map[string]string{"cp-new": "10.0.0.5"}
	err := p.configureAndWaitForNewCPs(pCtx, newCPs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate machine config for new CP node")
}

func TestConfigureAndWaitForNewWorkers_GenerateConfigError(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	mockTalos := &mockTalosConfigProducer{
		generateWorkerConfigFn: func(hostname string, _ int64) ([]byte, error) {
			return nil, fmt.Errorf("worker config generation failed for %s", hostname)
		},
	}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Talos:    mockTalos,
		Observer: observer,
		Logger:   observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.WorkerServerIDs = map[string]int64{"w-new": 456}

	newWorkers := map[string]string{"w-new": "10.0.0.10"}
	err := p.configureAndWaitForNewWorkers(pCtx, newWorkers)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate worker config for new node")
}

// --- configureNewNodes tests ---

func TestConfigureNewNodes_NoMaintenanceNodes(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Observer: observer,
		Logger:   observer,
		Timeouts: config.TestTimeouts(),
	}
	// No nodes at all
	pCtx.State.ControlPlaneIPs = map[string]string{}
	pCtx.State.WorkerIPs = map[string]string{}

	err := p.configureNewNodes(pCtx)
	require.NoError(t, err)
}

// --- detectMaintenanceModeNodes tests ---

func TestDetectMaintenanceModeNodes_EmptyNodeList(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Observer: observer,
		Logger:   observer,
		Timeouts: config.TestTimeouts(),
	}

	result := p.detectMaintenanceModeNodes(pCtx, map[string]string{}, "control plane")
	assert.Empty(t, result)
}

// --- BootstrapCluster integration-style tests ---

func TestBootstrapCluster_ApplyControlPlaneConfigsError(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	mockTalos := &mockTalosConfigProducer{
		getClientConfigFunc: func() ([]byte, error) {
			return []byte("test-talos-config"), nil
		},
		generateControlPlaneConfigFn: func(_ []string, _ string, _ int64) ([]byte, error) {
			return nil, fmt.Errorf("config generation error")
		},
	}

	mockInfra := &hcloud_internal.MockClient{}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Talos:    mockTalos,
		Infra:    mockInfra,
		Observer: observer,
		Logger:   observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.ControlPlaneIPs = map[string]string{"cp-1": "10.0.0.1"}
	pCtx.State.ControlPlaneServerIDs = map[string]int64{"cp-1": 100}

	err := p.BootstrapCluster(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate machine config")
}

func TestApplyControlPlaneConfigs_PrivateFirstViaLB_NoLB(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	mockInfra := &hcloud_internal.MockClient{}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster", ClusterAccess: "private"},
		State:    provisioning.NewState(),
		Infra:    mockInfra,
		Observer: observer,
		Logger:   observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.ControlPlaneIPs = map[string]string{"cp-1": "10.0.0.1"}

	err := p.applyControlPlaneConfigs(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private-first mode requires Load Balancer")
}

func TestWaitForControlPlaneReady_DirectPath_Empty(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Observer: observer,
		Logger:   observer,
		Timeouts: config.TestTimeouts(),
	}
	// No CP nodes → should succeed immediately
	pCtx.State.ControlPlaneIPs = map[string]string{}

	err := p.waitForControlPlaneReady(pCtx)
	require.NoError(t, err)
}

func TestRetrieveAndStoreKubeconfig_DirectPath_InvalidConfig(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Observer: observer,
		Logger:   observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.ControlPlaneIPs = map[string]string{"cp-1": "10.0.0.1"}
	pCtx.State.TalosConfig = []byte("bad-config")

	err := p.retrieveAndStoreKubeconfig(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to retrieve kubeconfig")
}

func TestBootstrapEtcd_DirectPath_InvalidConfig(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  context.Background(),
		Config:   &config.Config{ClusterName: "test-cluster"},
		State:    provisioning.NewState(),
		Observer: observer,
		Logger:   observer,
		Timeouts: config.TestTimeouts(),
	}
	pCtx.State.ControlPlaneIPs = map[string]string{"cp-1": "10.0.0.1"}
	pCtx.State.TalosConfig = []byte("invalid-yaml")

	err := p.bootstrapEtcd(pCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse talos config")
}

