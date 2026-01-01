package image_test

import (
	"context"
	"testing"

	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/image"
	"github.com/sak-d/hcloud-k8s/internal/ssh"
)

func TestBuild(t *testing.T) {
	mockClient := &hcloud.MockClient{
		CreateServerFunc: func(ctx context.Context, name, imageType, serverType string, sshKeys []string, labels map[string]string) (string, error) {
			return "123", nil
		},
		GetServerIPFunc: func(ctx context.Context, name string) (string, error) {
			return "1.2.3.4", nil
		},
		CreateSnapshotFunc: func(ctx context.Context, serverName, snapshotDescription string) (string, error) {
			return "snap-123", nil
		},
		DeleteServerFunc: func(ctx context.Context, name string) error {
			return nil
		},
		CreateSSHKeyFunc: func(ctx context.Context, name, publicKey string) (string, error) {
			return "key-123", nil
		},
		DeleteSSHKeyFunc: func(ctx context.Context, name string) error {
			return nil
		},
	}

	mockSSHFactory := func(host string) ssh.Communicator {
		return &MockSSH{}
	}

	builder := image.NewBuilder(mockClient, mockSSHFactory)
	snapshotID, err := builder.Build(context.Background(), "test-image", "v1.8.0", "amd64", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snapshotID != "snap-123" {
		t.Errorf("expected snapshot ID 'snap-123', got '%s'", snapshotID)
	}
}

type MockSSH struct{}

func (m *MockSSH) Execute(ctx context.Context, command string) (string, error) {
	return "ok", nil
}

func (m *MockSSH) UploadFile(ctx context.Context, localPath, remotePath string) error {
	return nil
}
