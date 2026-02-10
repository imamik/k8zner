package controller

import (
	"context"
	"sync"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"

	"github.com/imamik/k8zner/internal/platform/hcloud"
)

// MockHCloudClient is a mock implementation of HCloudClient for testing.
type MockHCloudClient struct {
	mu sync.Mutex

	// Configurable responses
	CreateServerFunc        func(ctx context.Context, opts hcloud.ServerCreateOpts) (string, error)
	DeleteServerFunc        func(ctx context.Context, name string) error
	GetServerIPFunc         func(ctx context.Context, name string) (string, error)
	GetServerIDFunc         func(ctx context.Context, name string) (string, error)
	GetServerByNameFunc     func(ctx context.Context, name string) (*hcloudgo.Server, error)
	GetServersByLabelFunc   func(ctx context.Context, labels map[string]string) ([]*hcloudgo.Server, error)
	CreateSSHKeyFunc        func(ctx context.Context, name, publicKey string, labels map[string]string) (string, error)
	DeleteSSHKeyFunc        func(ctx context.Context, name string) error
	GetNetworkFunc          func(ctx context.Context, name string) (*hcloudgo.Network, error)
	GetSnapshotByLabelsFunc func(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error)

	// Call tracking
	CreateServerCalls        []hcloud.ServerCreateOpts
	DeleteServerCalls        []string
	GetServerIPCalls         []string
	GetServerIDCalls         []string
	GetServerByNameCalls     []string
	CreateSSHKeyCalls        []CreateSSHKeyCall
	DeleteSSHKeyCalls        []string
	GetNetworkCalls          []string
	GetSnapshotByLabelsCalls []map[string]string
}

// CreateSSHKeyCall tracks arguments to CreateSSHKey.
type CreateSSHKeyCall struct {
	Name      string
	PublicKey string
	Labels    map[string]string
}

func (m *MockHCloudClient) CreateServer(ctx context.Context, opts hcloud.ServerCreateOpts) (string, error) {
	m.mu.Lock()
	m.CreateServerCalls = append(m.CreateServerCalls, opts)
	m.mu.Unlock()

	if m.CreateServerFunc != nil {
		return m.CreateServerFunc(ctx, opts)
	}
	return "12345", nil
}

func (m *MockHCloudClient) DeleteServer(ctx context.Context, name string) error {
	m.mu.Lock()
	m.DeleteServerCalls = append(m.DeleteServerCalls, name)
	m.mu.Unlock()

	if m.DeleteServerFunc != nil {
		return m.DeleteServerFunc(ctx, name)
	}
	return nil
}

func (m *MockHCloudClient) GetServerIP(ctx context.Context, name string) (string, error) {
	m.mu.Lock()
	m.GetServerIPCalls = append(m.GetServerIPCalls, name)
	m.mu.Unlock()

	if m.GetServerIPFunc != nil {
		return m.GetServerIPFunc(ctx, name)
	}
	return "10.0.0.1", nil
}

func (m *MockHCloudClient) GetServerID(ctx context.Context, name string) (string, error) {
	m.mu.Lock()
	m.GetServerIDCalls = append(m.GetServerIDCalls, name)
	m.mu.Unlock()

	if m.GetServerIDFunc != nil {
		return m.GetServerIDFunc(ctx, name)
	}
	return "12345", nil
}

func (m *MockHCloudClient) GetServerByName(ctx context.Context, name string) (*hcloudgo.Server, error) {
	m.mu.Lock()
	m.GetServerByNameCalls = append(m.GetServerByNameCalls, name)
	m.mu.Unlock()

	if m.GetServerByNameFunc != nil {
		return m.GetServerByNameFunc(ctx, name)
	}
	return nil, nil // Default: server not found
}

func (m *MockHCloudClient) GetServersByLabel(ctx context.Context, labels map[string]string) ([]*hcloudgo.Server, error) {
	if m.GetServersByLabelFunc != nil {
		return m.GetServersByLabelFunc(ctx, labels)
	}
	return nil, nil
}

func (m *MockHCloudClient) CreateSSHKey(ctx context.Context, name, publicKey string, labels map[string]string) (string, error) {
	m.mu.Lock()
	labelsCopy := make(map[string]string, len(labels))
	for k, v := range labels {
		labelsCopy[k] = v
	}
	m.CreateSSHKeyCalls = append(m.CreateSSHKeyCalls, CreateSSHKeyCall{
		Name:      name,
		PublicKey: publicKey,
		Labels:    labelsCopy,
	})
	m.mu.Unlock()

	if m.CreateSSHKeyFunc != nil {
		return m.CreateSSHKeyFunc(ctx, name, publicKey, labels)
	}
	return "12345", nil
}

func (m *MockHCloudClient) DeleteSSHKey(ctx context.Context, name string) error {
	m.mu.Lock()
	m.DeleteSSHKeyCalls = append(m.DeleteSSHKeyCalls, name)
	m.mu.Unlock()

	if m.DeleteSSHKeyFunc != nil {
		return m.DeleteSSHKeyFunc(ctx, name)
	}
	return nil
}

func (m *MockHCloudClient) GetNetwork(ctx context.Context, name string) (*hcloudgo.Network, error) {
	m.mu.Lock()
	m.GetNetworkCalls = append(m.GetNetworkCalls, name)
	m.mu.Unlock()

	if m.GetNetworkFunc != nil {
		return m.GetNetworkFunc(ctx, name)
	}
	return &hcloudgo.Network{ID: 1, Name: name}, nil
}

func (m *MockHCloudClient) GetSnapshotByLabels(ctx context.Context, labels map[string]string) (*hcloudgo.Image, error) {
	m.mu.Lock()
	// Make a copy of labels for tracking
	labelsCopy := make(map[string]string, len(labels))
	for k, v := range labels {
		labelsCopy[k] = v
	}
	m.GetSnapshotByLabelsCalls = append(m.GetSnapshotByLabelsCalls, labelsCopy)
	m.mu.Unlock()

	if m.GetSnapshotByLabelsFunc != nil {
		return m.GetSnapshotByLabelsFunc(ctx, labels)
	}
	return &hcloudgo.Image{ID: 1, Name: "talos-snapshot"}, nil
}

// MockTalosClient is a mock implementation of TalosClient for testing.
type MockTalosClient struct {
	mu sync.Mutex

	// Configurable responses
	ApplyConfigFunc             func(ctx context.Context, nodeIP string, config []byte) error
	IsNodeInMaintenanceModeFunc func(ctx context.Context, nodeIP string) (bool, error)
	GetEtcdMembersFunc          func(ctx context.Context, nodeIP string) ([]etcdMember, error)
	RemoveEtcdMemberFunc        func(ctx context.Context, nodeIP string, memberID string) error
	WaitForNodeReadyFunc        func(ctx context.Context, nodeIP string, timeout int) error

	// Call tracking
	ApplyConfigCalls      []ApplyConfigCall
	GetEtcdMembersCalls   []string
	RemoveEtcdMemberCalls []RemoveEtcdMemberCall
	WaitForNodeReadyCalls []WaitForNodeReadyCall
}

// WaitForNodeReadyCall tracks arguments to WaitForNodeReady.
type WaitForNodeReadyCall struct {
	NodeIP  string
	Timeout int
}

// ApplyConfigCall tracks arguments to ApplyConfig.
type ApplyConfigCall struct {
	NodeIP string
	Config []byte
}

// RemoveEtcdMemberCall tracks arguments to RemoveEtcdMember.
type RemoveEtcdMemberCall struct {
	NodeIP   string
	MemberID string
}

func (m *MockTalosClient) ApplyConfig(ctx context.Context, nodeIP string, config []byte) error {
	m.mu.Lock()
	m.ApplyConfigCalls = append(m.ApplyConfigCalls, ApplyConfigCall{
		NodeIP: nodeIP,
		Config: config,
	})
	m.mu.Unlock()

	if m.ApplyConfigFunc != nil {
		return m.ApplyConfigFunc(ctx, nodeIP, config)
	}
	return nil
}

func (m *MockTalosClient) IsNodeInMaintenanceMode(ctx context.Context, nodeIP string) (bool, error) {
	if m.IsNodeInMaintenanceModeFunc != nil {
		return m.IsNodeInMaintenanceModeFunc(ctx, nodeIP)
	}
	return true, nil
}

func (m *MockTalosClient) GetEtcdMembers(ctx context.Context, nodeIP string) ([]etcdMember, error) {
	m.mu.Lock()
	m.GetEtcdMembersCalls = append(m.GetEtcdMembersCalls, nodeIP)
	m.mu.Unlock()

	if m.GetEtcdMembersFunc != nil {
		return m.GetEtcdMembersFunc(ctx, nodeIP)
	}
	return []etcdMember{
		{ID: "1", Name: "cp-1", Endpoint: "10.0.0.1:2379", IsLeader: true},
		{ID: "2", Name: "cp-2", Endpoint: "10.0.0.2:2379", IsLeader: false},
		{ID: "3", Name: "cp-3", Endpoint: "10.0.0.3:2379", IsLeader: false},
	}, nil
}

func (m *MockTalosClient) RemoveEtcdMember(ctx context.Context, nodeIP string, memberID string) error {
	m.mu.Lock()
	m.RemoveEtcdMemberCalls = append(m.RemoveEtcdMemberCalls, RemoveEtcdMemberCall{
		NodeIP:   nodeIP,
		MemberID: memberID,
	})
	m.mu.Unlock()

	if m.RemoveEtcdMemberFunc != nil {
		return m.RemoveEtcdMemberFunc(ctx, nodeIP, memberID)
	}
	return nil
}

func (m *MockTalosClient) WaitForNodeReady(ctx context.Context, nodeIP string, timeout int) error {
	m.mu.Lock()
	m.WaitForNodeReadyCalls = append(m.WaitForNodeReadyCalls, WaitForNodeReadyCall{
		NodeIP:  nodeIP,
		Timeout: timeout,
	})
	m.mu.Unlock()

	if m.WaitForNodeReadyFunc != nil {
		return m.WaitForNodeReadyFunc(ctx, nodeIP, timeout)
	}
	return nil
}

// MockTalosConfigGenerator is a mock implementation of TalosConfigGenerator for testing.
type MockTalosConfigGenerator struct {
	mu sync.Mutex

	// Configurable responses
	GenerateControlPlaneConfigFunc func(sans []string, hostname string, serverID int64) ([]byte, error)
	GenerateWorkerConfigFunc       func(hostname string, serverID int64) ([]byte, error)
	SetEndpointFunc                func(endpoint string)
	GetClientConfigFunc            func() ([]byte, error)

	// Call tracking
	GenerateControlPlaneConfigCalls []GenerateControlPlaneConfigCall
	GenerateWorkerConfigCalls       []GenerateWorkerConfigCall
	SetEndpointCalls                []string
}

// GenerateControlPlaneConfigCall tracks arguments to GenerateControlPlaneConfig.
type GenerateControlPlaneConfigCall struct {
	SANs     []string
	Hostname string
	ServerID int64
}

// GenerateWorkerConfigCall tracks arguments to GenerateWorkerConfig.
type GenerateWorkerConfigCall struct {
	Hostname string
	ServerID int64
}

func (m *MockTalosConfigGenerator) GenerateControlPlaneConfig(sans []string, hostname string, serverID int64) ([]byte, error) {
	m.mu.Lock()
	m.GenerateControlPlaneConfigCalls = append(m.GenerateControlPlaneConfigCalls, GenerateControlPlaneConfigCall{
		SANs:     sans,
		Hostname: hostname,
		ServerID: serverID,
	})
	m.mu.Unlock()

	if m.GenerateControlPlaneConfigFunc != nil {
		return m.GenerateControlPlaneConfigFunc(sans, hostname, serverID)
	}
	return []byte("mock-control-plane-config"), nil
}

func (m *MockTalosConfigGenerator) GenerateWorkerConfig(hostname string, serverID int64) ([]byte, error) {
	m.mu.Lock()
	m.GenerateWorkerConfigCalls = append(m.GenerateWorkerConfigCalls, GenerateWorkerConfigCall{
		Hostname: hostname,
		ServerID: serverID,
	})
	m.mu.Unlock()

	if m.GenerateWorkerConfigFunc != nil {
		return m.GenerateWorkerConfigFunc(hostname, serverID)
	}
	return []byte("mock-worker-config"), nil
}

func (m *MockTalosConfigGenerator) SetEndpoint(endpoint string) {
	m.mu.Lock()
	m.SetEndpointCalls = append(m.SetEndpointCalls, endpoint)
	m.mu.Unlock()

	if m.SetEndpointFunc != nil {
		m.SetEndpointFunc(endpoint)
	}
}

func (m *MockTalosConfigGenerator) GetClientConfig() ([]byte, error) {
	if m.GetClientConfigFunc != nil {
		return m.GetClientConfigFunc()
	}
	return []byte("mock-talosconfig"), nil
}
