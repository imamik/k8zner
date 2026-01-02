package hcloud

import (
	"context"
)

// MockClient is a mock implementation of ServerProvisioner, SnapshotManager, and SSHKeyManager.
type MockClient struct {
	CreateServerFunc   func(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string) (string, error)
	DeleteServerFunc   func(ctx context.Context, name string) error
	GetServerIPFunc    func(ctx context.Context, name string) (string, error)
	GetServerIDFunc    func(ctx context.Context, name string) (string, error)
	EnableRescueFunc   func(ctx context.Context, serverID string, sshKeyIDs []string) (string, error)
	ResetServerFunc    func(ctx context.Context, serverID string) error
	PoweroffServerFunc func(ctx context.Context, serverID string) error

	CreateSnapshotFunc func(ctx context.Context, serverID, snapshotDescription string) (string, error)
	DeleteImageFunc    func(ctx context.Context, imageID string) error

	CreateSSHKeyFunc func(ctx context.Context, name, publicKey string) (string, error)
	DeleteSSHKeyFunc func(ctx context.Context, name string) error
}

// CreateServer mocks server creation.
func (m *MockClient) CreateServer(ctx context.Context, name, imageType, serverType, location string, sshKeys []string, labels map[string]string, userData string) (string, error) {
	if m.CreateServerFunc != nil {
		return m.CreateServerFunc(ctx, name, imageType, serverType, location, sshKeys, labels, userData)
	}
	return "mock-id", nil
}

// DeleteServer mocks server deletion.
func (m *MockClient) DeleteServer(ctx context.Context, name string) error {
	if m.DeleteServerFunc != nil {
		return m.DeleteServerFunc(ctx, name)
	}
	return nil
}

// GetServerIP mocks getting server IP.
func (m *MockClient) GetServerIP(ctx context.Context, name string) (string, error) {
	if m.GetServerIPFunc != nil {
		return m.GetServerIPFunc(ctx, name)
	}
	return "127.0.0.1", nil
}

// GetServerID mocks getting server ID.
func (m *MockClient) GetServerID(ctx context.Context, name string) (string, error) {
	if m.GetServerIDFunc != nil {
		return m.GetServerIDFunc(ctx, name)
	}
	return "123", nil
}

// EnableRescue mocks enabling rescue mode.
func (m *MockClient) EnableRescue(ctx context.Context, serverID string, sshKeyIDs []string) (string, error) {
	if m.EnableRescueFunc != nil {
		return m.EnableRescueFunc(ctx, serverID, sshKeyIDs)
	}
	return "mock-password", nil
}

// ResetServer mocks resetting server.
func (m *MockClient) ResetServer(ctx context.Context, serverID string) error {
	if m.ResetServerFunc != nil {
		return m.ResetServerFunc(ctx, serverID)
	}
	return nil
}

// PoweroffServer mocks powering off server.
func (m *MockClient) PoweroffServer(ctx context.Context, serverID string) error {
	if m.PoweroffServerFunc != nil {
		return m.PoweroffServerFunc(ctx, serverID)
	}
	return nil
}

// CreateSnapshot mocks snapshot creation.
func (m *MockClient) CreateSnapshot(ctx context.Context, serverID, snapshotDescription string) (string, error) {
	if m.CreateSnapshotFunc != nil {
		return m.CreateSnapshotFunc(ctx, serverID, snapshotDescription)
	}
	return "mock-snapshot-id", nil
}

// DeleteImage mocks image deletion.
func (m *MockClient) DeleteImage(ctx context.Context, imageID string) error {
	if m.DeleteImageFunc != nil {
		return m.DeleteImageFunc(ctx, imageID)
	}
	return nil
}

// CreateSSHKey mocks ssh key creation.
func (m *MockClient) CreateSSHKey(ctx context.Context, name, publicKey string) (string, error) {
	if m.CreateSSHKeyFunc != nil {
		return m.CreateSSHKeyFunc(ctx, name, publicKey)
	}
	return "mock-key-id", nil
}

// DeleteSSHKey mocks ssh key deletion.
func (m *MockClient) DeleteSSHKey(ctx context.Context, name string) error {
	if m.DeleteSSHKeyFunc != nil {
		return m.DeleteSSHKeyFunc(ctx, name)
	}
	return nil
}
