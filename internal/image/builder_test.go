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
		CreateServerFunc: func(_ context.Context, _ string, _ string, _ string, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string) (string, error) {
			return "123", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "1.2.3.4", nil
		},
		CreateSnapshotFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "snap-123", nil
		},
		DeleteServerFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string) (string, error) {
			return "key-123", nil
		},
		DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}

	mockSSHFactory := func(_ string) ssh.Communicator {
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

func (m *MockSSH) Execute(_ context.Context, _ string) (string, error) {
	return "ok", nil
}

func (m *MockSSH) UploadFile(_ context.Context, _, _ string) error {
	return nil
}
