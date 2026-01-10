package testing

import "github.com/stretchr/testify/mock"

// MockTalosProducer is a mock implementation of the TalosConfigProducer interface.
// It can be used across all tests that need to mock Talos configuration generation.
type MockTalosProducer struct {
	mock.Mock
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

// NewMockTalosProducer creates a new MockTalosProducer with default successful behavior.
func NewMockTalosProducer() *MockTalosProducer {
	m := &MockTalosProducer{}
	m.On("GetClientConfig").Return([]byte("client-config"), nil)
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
