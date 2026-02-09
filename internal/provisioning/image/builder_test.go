package image

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
)

func TestNewBuilder(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{}
	builder := NewBuilder(mockClient)
	require.NotNil(t, builder)
	assert.Equal(t, mockClient, builder.infra)
}

func TestNewBuilder_NilClient(t *testing.T) {
	t.Parallel()
	builder := NewBuilder(nil)
	require.NotNil(t, builder)
	assert.Nil(t, builder.infra)
}

func TestBuild_NilInfraClient(t *testing.T) {
	t.Parallel()
	builder := NewBuilder(nil)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InfrastructureManager is required")
}

func TestBuild_CreateSSHKeyError(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("ssh key creation failed")
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "", expectedErr
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload ssh key")
}

func TestBuild_CreateServerError(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("server creation failed")
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ string, _ string, _ string, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string, _, _ bool) (string, error) {
			return "", expectedErr
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create server")
}

func TestBuild_GetServerIPError(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("get IP failed")
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ string, _ string, _ string, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string, _, _ bool) (string, error) {
			return "server-123", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "", expectedErr
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get server IP")
}

func TestBuild_EnableRescueError(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("enable rescue failed")
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ string, _ string, _ string, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string, _, _ bool) (string, error) {
			return "server-123", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "1.2.3.4", nil
		},
		EnableRescueFunc: func(_ context.Context, _ string, _ []string) (string, error) {
			return "", expectedErr
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to enable rescue")
}

func TestBuild_ResetServerError(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("reset server failed")
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ string, _ string, _ string, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string, _, _ bool) (string, error) {
			return "server-123", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "1.2.3.4", nil
		},
		EnableRescueFunc: func(_ context.Context, _ string, _ []string) (string, error) {
			return "password", nil
		},
		ResetServerFunc: func(_ context.Context, _ string) error {
			return expectedErr
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to reset server")
}

func TestBuild_DefaultLocation(t *testing.T) {
	t.Parallel(
	// Test that empty location defaults to nbg1
	)

	var capturedLocation string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _, _, _, location string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string, _, _ bool) (string, error) {
			capturedLocation = location
			return "", errors.New("stop here") // Stop after capturing location
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "", nil)

	assert.Equal(t, "nbg1", capturedLocation)
}

func TestBuild_DefaultServerType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		arch               string
		expectedServerType string
	}{
		{"amd64 default", "amd64", "cpx22"},
		{"arm64 default", "arm64", "cax11"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var capturedServerType string
			mockClient := &hcloud.MockClient{
				CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
					return "key-123", nil
				},
				CreateServerFunc: func(_ context.Context, _, _, serverType, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string, _, _ bool) (string, error) {
					capturedServerType = serverType
					return "", errors.New("stop here")
				},
			}

			builder := NewBuilder(mockClient)
			_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", tt.arch, "", "nbg1", nil)

			assert.Equal(t, tt.expectedServerType, capturedServerType)
		})
	}
}

func TestBuild_CustomLabels(t *testing.T) {
	t.Parallel()
	var capturedLabels map[string]string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, labels map[string]string) (string, error) {
			capturedLabels = labels
			return "", errors.New("stop here")
		},
	}

	customLabels := map[string]string{
		"env":  "test",
		"team": "platform",
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", customLabels)

	// Check that custom labels are passed through
	assert.Equal(t, "test", capturedLabels["env"])
	assert.Equal(t, "platform", capturedLabels["team"])
	// And type label is added
	assert.Equal(t, "build-ssh-key", capturedLabels["type"])
}

func TestGetKeys(t *testing.T) {
	t.Parallel()
	t.Run("empty map", func(t *testing.T) {
		t.Parallel()
		result := getKeys(map[string]bool{})
		assert.Empty(t, result)
	})

	t.Run("single key", func(t *testing.T) {
		t.Parallel()
		result := getKeys(map[string]bool{"amd64": true})
		assert.Len(t, result, 1)
		assert.Contains(t, result, "amd64")
	})

	t.Run("multiple keys", func(t *testing.T) {
		t.Parallel()
		result := getKeys(map[string]bool{"amd64": true, "arm64": true})
		assert.Len(t, result, 2)
		assert.Contains(t, result, "amd64")
		assert.Contains(t, result, "arm64")
	})
}

func TestEnsureImageForArch(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	t.Run("skips build when snapshot exists", func(t *testing.T) {
		t.Parallel()
		mockClient := &hcloud.MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
				return &hcloudgo.Image{ID: 123, Description: "existing-snapshot"}, nil
			},
		}
		ctx := createTestContext(t, mockClient, &config.Config{})

		err := p.ensureImageForArch(ctx, "amd64", "v1.8.0", "v1.30.0", "", "nbg1")
		assert.NoError(t, err)
	})

	t.Run("returns error when snapshot check fails", func(t *testing.T) {
		t.Parallel()
		mockClient := &hcloud.MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
				return nil, errors.New("API error")
			},
		}
		ctx := createTestContext(t, mockClient, &config.Config{})

		err := p.ensureImageForArch(ctx, "amd64", "v1.8.0", "v1.30.0", "", "nbg1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check for existing snapshot")
	})
}

func createTestContext(t *testing.T, mockInfra *hcloud.MockClient, cfg *config.Config) *provisioning.Context {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{}
	}
	return provisioning.NewContext(
		context.Background(),
		cfg,
		mockInfra,
		nil,
	)
}

// TestBuild verifies the basic builder orchestration logic.
// Note: This test uses mocks and cannot test actual SSH connectivity.
// SSH functionality is tested separately in internal/platform/ssh/ssh_test.go.
// For full end-to-end testing, use the e2e tests with real infrastructure.
func TestBuild(t *testing.T) {
	t.Parallel()
	t.Skip("This test requires mocking SSH which conflicts with the new SSH client design. Use e2e tests for full validation.")

	mockClient := &hcloud.MockClient{
		CreateServerFunc: func(_ context.Context, _ string, _ string, _ string, _ string, _ []string, _ map[string]string, _ string, _ *int64, _ int64, _ string, _, _ bool) (string, error) {
			return "123", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "1.2.3.4", nil
		},
		EnableRescueFunc: func(_ context.Context, _ string, _ []string) (string, error) {
			return "rescue-password", nil
		},
		ResetServerFunc: func(_ context.Context, _ string) error {
			return nil
		},
		PoweroffServerFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateSnapshotFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "snap-123", nil
		},
		DeleteServerFunc: func(_ context.Context, _ string) error {
			return nil
		},
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}

	builder := NewBuilder(mockClient)
	// serverType is empty to use auto-detection based on architecture
	snapshotID, err := builder.Build(context.Background(), "test-image", "v1.8.0", "amd64", "", "nbg1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snapshotID != "snap-123" {
		t.Errorf("expected snapshot ID 'snap-123', got '%s'", snapshotID)
	}
}
