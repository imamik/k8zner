package testing

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
)

// MockTalosProducer is a mock implementation of the TalosConfigProducer interface.
// It can be used across all tests that need to mock Talos configuration generation.
type MockTalosProducer struct {
	mock.Mock
}

// SetMachineConfigOptions sets the machine configuration options.
func (m *MockTalosProducer) SetMachineConfigOptions(opts any) {
	m.Called(opts)
}

// GenerateControlPlaneConfig generates a mock control plane configuration.
func (m *MockTalosProducer) GenerateControlPlaneConfig(san []string, hostname string) ([]byte, error) {
	args := m.Called(san, hostname)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

// GenerateWorkerConfig generates a mock worker configuration.
func (m *MockTalosProducer) GenerateWorkerConfig(hostname string) ([]byte, error) {
	args := m.Called(hostname)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

// GenerateAutoscalerConfig generates a mock autoscaler configuration.
func (m *MockTalosProducer) GenerateAutoscalerConfig(poolName string, labels map[string]string, taints []string) ([]byte, error) {
	args := m.Called(poolName, labels, taints)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

// GetClientConfig returns a mock client configuration.
func (m *MockTalosProducer) GetClientConfig() ([]byte, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

// SetEndpoint sets the mock endpoint.
func (m *MockTalosProducer) SetEndpoint(endpoint string) {
	m.Called(endpoint)
}

// GetNodeVersion retrieves the current Talos version from a node.
func (m *MockTalosProducer) GetNodeVersion(ctx context.Context, endpoint string) (string, error) {
	args := m.Called(ctx, endpoint)
	return args.String(0), args.Error(1)
}

// UpgradeNode upgrades a single node to the specified image.
func (m *MockTalosProducer) UpgradeNode(ctx context.Context, endpoint, imageURL string) error {
	args := m.Called(ctx, endpoint, imageURL)
	return args.Error(0)
}

// UpgradeKubernetes upgrades the Kubernetes control plane to the target version.
func (m *MockTalosProducer) UpgradeKubernetes(ctx context.Context, endpoint, targetVersion string) error {
	args := m.Called(ctx, endpoint, targetVersion)
	return args.Error(0)
}

// WaitForNodeReady waits for a node to become ready after reboot.
func (m *MockTalosProducer) WaitForNodeReady(ctx context.Context, endpoint string, timeout time.Duration) error {
	args := m.Called(ctx, endpoint, timeout)
	return args.Error(0)
}

// HealthCheck performs a cluster health check.
func (m *MockTalosProducer) HealthCheck(ctx context.Context, endpoint string) error {
	args := m.Called(ctx, endpoint)
	return args.Error(0)
}

// NewMockTalosProducer creates a new MockTalosProducer with default successful behavior.
func NewMockTalosProducer() *MockTalosProducer {
	m := &MockTalosProducer{}
	m.On("GetClientConfig").Return([]byte("client-config"), nil)
	m.On("SetMachineConfigOptions", mock.Anything).Return()
	return m
}

// WithMachineConfigOptions configures the mock to expect machine config options to be set.
func (m *MockTalosProducer) WithMachineConfigOptions() *MockTalosProducer {
	m.On("SetMachineConfigOptions", mock.Anything).Return()
	return m
}

// WithEndpoint configures the mock to expect a specific endpoint to be set.
func (m *MockTalosProducer) WithEndpoint(endpoint string) *MockTalosProducer {
	m.On("SetEndpoint", endpoint).Return()
	return m
}

// WithControlPlaneConfig configures the mock to return a specific control plane config.
func (m *MockTalosProducer) WithControlPlaneConfig(config []byte) *MockTalosProducer {
	m.On("GenerateControlPlaneConfig", mock.Anything, mock.Anything).Return(config, nil)
	return m
}

// WithWorkerConfig configures the mock to return a specific worker config.
func (m *MockTalosProducer) WithWorkerConfig(config []byte) *MockTalosProducer {
	m.On("GenerateWorkerConfig", mock.Anything).Return(config, nil)
	return m
}

// MockK8sClient is a mock implementation of the k8sclient.Client interface.
// It tracks method calls and allows configurable behavior via function fields.
type MockK8sClient struct {
	mock.Mock
	// ApplyManifestsFunc allows custom implementation for ApplyManifests
	ApplyManifestsFunc func(ctx context.Context, manifests []byte, fieldManager string) error
	// CreateSecretFunc allows custom implementation for CreateSecret
	CreateSecretFunc func(ctx context.Context, secret any) error
	// DeleteSecretFunc allows custom implementation for DeleteSecret
	DeleteSecretFunc func(ctx context.Context, namespace, name string) error
}

// ApplyManifests applies manifests using SSA.
func (m *MockK8sClient) ApplyManifests(ctx context.Context, manifests []byte, fieldManager string) error {
	if m.ApplyManifestsFunc != nil {
		return m.ApplyManifestsFunc(ctx, manifests, fieldManager)
	}
	args := m.Called(ctx, manifests, fieldManager)
	return args.Error(0)
}

// CreateSecret creates or replaces a secret.
func (m *MockK8sClient) CreateSecret(ctx context.Context, secret any) error {
	if m.CreateSecretFunc != nil {
		return m.CreateSecretFunc(ctx, secret)
	}
	args := m.Called(ctx, secret)
	return args.Error(0)
}

// DeleteSecret deletes a secret.
func (m *MockK8sClient) DeleteSecret(ctx context.Context, namespace, name string) error {
	if m.DeleteSecretFunc != nil {
		return m.DeleteSecretFunc(ctx, namespace, name)
	}
	args := m.Called(ctx, namespace, name)
	return args.Error(0)
}

// NewMockK8sClient creates a new MockK8sClient with default successful behavior.
func NewMockK8sClient() *MockK8sClient {
	m := &MockK8sClient{}
	m.On("ApplyManifests", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	m.On("CreateSecret", mock.Anything, mock.Anything).Return(nil)
	m.On("DeleteSecret", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	return m
}

// WithApplyManifestsError configures the mock to return an error on ApplyManifests.
func (m *MockK8sClient) WithApplyManifestsError(err error) *MockK8sClient {
	m.On("ApplyManifests", mock.Anything, mock.Anything, mock.Anything).Return(err)
	return m
}

// WithCreateSecretError configures the mock to return an error on CreateSecret.
func (m *MockK8sClient) WithCreateSecretError(err error) *MockK8sClient {
	m.On("CreateSecret", mock.Anything, mock.Anything).Return(err)
	return m
}
