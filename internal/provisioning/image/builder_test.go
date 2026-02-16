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
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
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
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
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
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
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
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
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
	t.Parallel()
	// Test that empty location defaults to nbg1

	var capturedLocation string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, opts hcloud.ServerCreateOpts) (string, error) {
			capturedLocation = opts.Location
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
		{"amd64 default", "amd64", "cx23"},
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
				CreateServerFunc: func(_ context.Context, opts hcloud.ServerCreateOpts) (string, error) {
					capturedServerType = opts.ServerType
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
	t.Run("skips build when snapshot exists", func(t *testing.T) {
		t.Parallel()
		mockClient := &hcloud.MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
				return &hcloudgo.Image{ID: 123, Description: "existing-snapshot"}, nil
			},
		}
		ctx := createTestContext(t, mockClient, &config.Config{})

		err := ensureImageForArch(ctx, "amd64", "v1.8.0", "v1.30.0", "", "nbg1")
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

		err := ensureImageForArch(ctx, "amd64", "v1.8.0", "v1.30.0", "", "nbg1")
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

// TestCleanupServer tests the cleanupServer method.
func TestCleanupServer(t *testing.T) {
	t.Parallel()

	t.Run("success path", func(t *testing.T) {
		t.Parallel()
		var deletedServerName string
		mockClient := &hcloud.MockClient{
			DeleteServerFunc: func(_ context.Context, name string) error {
				deletedServerName = name
				return nil
			},
		}

		builder := NewBuilder(mockClient)
		builder.cleanupServer("test-server")
		assert.Equal(t, "test-server", deletedServerName)
	})

	t.Run("error path logs but does not panic", func(t *testing.T) {
		t.Parallel()
		mockClient := &hcloud.MockClient{
			DeleteServerFunc: func(_ context.Context, _ string) error {
				return errors.New("delete server failed")
			},
		}

		builder := NewBuilder(mockClient)
		// Should not panic; error is logged
		builder.cleanupServer("test-server")
	})
}

// TestCleanupSSHKey tests the cleanupSSHKey method.
func TestCleanupSSHKey(t *testing.T) {
	t.Parallel()

	t.Run("success path", func(t *testing.T) {
		t.Parallel()
		var deletedKeyName string
		mockClient := &hcloud.MockClient{
			DeleteSSHKeyFunc: func(_ context.Context, name string) error {
				deletedKeyName = name
				return nil
			},
		}

		builder := NewBuilder(mockClient)
		builder.cleanupSSHKey("test-key")
		assert.Equal(t, "test-key", deletedKeyName)
	})

	t.Run("error path logs but does not panic", func(t *testing.T) {
		t.Parallel()
		mockClient := &hcloud.MockClient{
			DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
				return errors.New("delete SSH key failed")
			},
		}

		builder := NewBuilder(mockClient)
		// Should not panic; error is logged
		builder.cleanupSSHKey("test-key")
	})
}

// TestBuild_DeferredCleanupsRunOnCreateServerError verifies that deferred
// cleanupServer and cleanupSSHKey are called even when CreateServer fails.
func TestBuild_DeferredCleanupsRunOnCreateServerError(t *testing.T) {
	t.Parallel()
	var serverDeleted, sshKeyDeleted bool
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "", errors.New("server creation failed")
		},
		DeleteServerFunc: func(_ context.Context, _ string) error {
			serverDeleted = true
			return nil
		},
		DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
			sshKeyDeleted = true
			return nil
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	// Both cleanup defers should have been called
	assert.True(t, serverDeleted, "server cleanup should have been called")
	assert.True(t, sshKeyDeleted, "SSH key cleanup should have been called")
}

// TestBuild_DeferredCleanupsRunOnEnableRescueError verifies cleanup on EnableRescue failure.
func TestBuild_DeferredCleanupsRunOnEnableRescueError(t *testing.T) {
	t.Parallel()
	var serverDeleted, sshKeyDeleted bool
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "server-123", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "1.2.3.4", nil
		},
		EnableRescueFunc: func(_ context.Context, _ string, _ []string) (string, error) {
			return "", errors.New("enable rescue failed")
		},
		DeleteServerFunc: func(_ context.Context, _ string) error {
			serverDeleted = true
			return nil
		},
		DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
			sshKeyDeleted = true
			return nil
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.True(t, serverDeleted, "server cleanup should have been called")
	assert.True(t, sshKeyDeleted, "SSH key cleanup should have been called")
}

// TestBuild_DeferredCleanupErrors verifies that cleanup errors do not mask the original error.
func TestBuild_DeferredCleanupErrors(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "", errors.New("server creation failed")
		},
		DeleteServerFunc: func(_ context.Context, _ string) error {
			return errors.New("cleanup server also failed")
		},
		DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
			return errors.New("cleanup SSH key also failed")
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	// The original error should still be returned, not the cleanup errors
	assert.Contains(t, err.Error(), "failed to create server")
}

// TestBuild_CustomServerType verifies that a custom server type is passed through.
func TestBuild_CustomServerType(t *testing.T) {
	t.Parallel()
	var capturedServerType string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, opts hcloud.ServerCreateOpts) (string, error) {
			capturedServerType = opts.ServerType
			return "", errors.New("stop here")
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "cx42", "nbg1", nil)
	assert.Equal(t, "cx42", capturedServerType)
}

// TestBuild_NilLabels verifies that nil labels are handled (SSH key labels get type added).
func TestBuild_NilLabels(t *testing.T) {
	t.Parallel()
	var capturedLabels map[string]string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, labels map[string]string) (string, error) {
			capturedLabels = labels
			return "", errors.New("stop here")
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)

	// Even with nil labels input, the SSH key label should have the type field
	require.NotNil(t, capturedLabels)
	assert.Equal(t, "build-ssh-key", capturedLabels["type"])
	assert.Len(t, capturedLabels, 1)
}

// TestBuild_ServerCreateOptsPublicIP verifies that public IPv4 and IPv6 are enabled.
func TestBuild_ServerCreateOptsPublicIP(t *testing.T) {
	t.Parallel()
	var capturedOpts hcloud.ServerCreateOpts
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, opts hcloud.ServerCreateOpts) (string, error) {
			capturedOpts = opts
			return "", errors.New("stop here")
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)

	assert.True(t, capturedOpts.EnablePublicIPv4, "should enable public IPv4")
	assert.True(t, capturedOpts.EnablePublicIPv6, "should enable public IPv6")
	assert.Equal(t, "debian-13", capturedOpts.ImageType, "should use debian-13")
}

// TestBuild_SSHKeyIDPassedToEnableRescue verifies that the SSH key ID from CreateSSHKey
// is passed to EnableRescue.
func TestBuild_SSHKeyIDPassedToEnableRescue(t *testing.T) {
	t.Parallel()
	var capturedSSHKeyIDs []string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "my-ssh-key-id-42", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "server-99", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "1.2.3.4", nil
		},
		EnableRescueFunc: func(_ context.Context, _ string, sshKeyIDs []string) (string, error) {
			capturedSSHKeyIDs = sshKeyIDs
			return "", errors.New("stop here")
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)

	require.Len(t, capturedSSHKeyIDs, 1)
	assert.Equal(t, "my-ssh-key-id-42", capturedSSHKeyIDs[0])
}

// TestBuild verifies the basic builder orchestration logic.
// Note: This test uses mocks and cannot test actual SSH connectivity.
// SSH functionality is tested separately in internal/platform/ssh/ssh_test.go.
// For full end-to-end testing, use the e2e tests with real infrastructure.
func TestBuild(t *testing.T) {
	t.Parallel()
	t.Skip("This test requires mocking SSH which conflicts with the new SSH client design. Use e2e tests for full validation.")

	mockClient := &hcloud.MockClient{
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
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

// TestCleanupServer_ErrorDoesNotPreventSSHKeyCleanup verifies that a server
// cleanup error does not prevent SSH key cleanup from running when Build
// encounters an error at a later stage.
func TestCleanupServer_ErrorDoesNotPreventSSHKeyCleanup(t *testing.T) {
	t.Parallel()
	var cleanupOrder []string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "server-123", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("get IP failed")
		},
		DeleteServerFunc: func(_ context.Context, _ string) error {
			cleanupOrder = append(cleanupOrder, "server")
			return errors.New("server cleanup error")
		},
		DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
			cleanupOrder = append(cleanupOrder, "sshkey")
			return nil
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get server IP")
	// Both cleanups should run even when server cleanup fails
	assert.Contains(t, cleanupOrder, "server")
	assert.Contains(t, cleanupOrder, "sshkey")
}

// TestCleanupSSHKey_ErrorDoesNotPreventServerCleanup verifies that an SSH key
// cleanup error does not prevent server cleanup from running.
func TestCleanupSSHKey_ErrorDoesNotPreventServerCleanup(t *testing.T) {
	t.Parallel()
	var cleanupOrder []string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "server-123", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "1.2.3.4", nil
		},
		EnableRescueFunc: func(_ context.Context, _ string, _ []string) (string, error) {
			return "", errors.New("rescue failed")
		},
		DeleteServerFunc: func(_ context.Context, _ string) error {
			cleanupOrder = append(cleanupOrder, "server")
			return nil
		},
		DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
			cleanupOrder = append(cleanupOrder, "sshkey")
			return errors.New("SSH key cleanup error")
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to enable rescue")
	// Both cleanups should be called despite SSH key cleanup error
	assert.Contains(t, cleanupOrder, "server")
	assert.Contains(t, cleanupOrder, "sshkey")
}

// TestCleanupServer_CorrectNamePassed verifies that the server name generated
// during Build is the same name passed to cleanupServer.
func TestCleanupServer_CorrectNamePassed(t *testing.T) {
	t.Parallel()
	var createdServerName, deletedServerName string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, opts hcloud.ServerCreateOpts) (string, error) {
			createdServerName = opts.Name
			return "", errors.New("stop after capture")
		},
		DeleteServerFunc: func(_ context.Context, name string) error {
			deletedServerName = name
			return nil
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)

	assert.NotEmpty(t, createdServerName)
	assert.Equal(t, createdServerName, deletedServerName, "cleanup should delete the same server that was created")
	assert.Contains(t, createdServerName, "build-talos-v1.8.0-k8s-v1.30.0-amd64-")
}

// TestCleanupSSHKey_CorrectNamePassed verifies that the SSH key name generated
// during Build is the same name passed to cleanupSSHKey.
func TestCleanupSSHKey_CorrectNamePassed(t *testing.T) {
	t.Parallel()
	var createdKeyName, deletedKeyName string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, name string, _ string, _ map[string]string) (string, error) {
			createdKeyName = name
			return "", errors.New("stop after capture")
		},
		DeleteSSHKeyFunc: func(_ context.Context, name string) error {
			deletedKeyName = name
			return nil
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)

	// CreateSSHKey failed, so cleanup should NOT have been deferred (defer is after CreateSSHKey)
	// Actually, looking at the code, the defer is AFTER CreateSSHKey succeeds.
	// So when CreateSSHKey fails, cleanupSSHKey should NOT be called.
	assert.NotEmpty(t, createdKeyName)
	assert.Empty(t, deletedKeyName, "SSH key cleanup should not run when CreateSSHKey fails")
	assert.Contains(t, createdKeyName, "key-build-talos-v1.8.0-k8s-v1.30.0-amd64-")
}

// TestBuild_ServerIDPassedToEnableRescue verifies that the server ID from
// CreateServer is correctly passed to EnableRescue.
func TestBuild_ServerIDPassedToEnableRescue(t *testing.T) {
	t.Parallel()
	var capturedServerID string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-abc", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "server-id-999", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "10.0.0.1", nil
		},
		EnableRescueFunc: func(_ context.Context, serverID string, _ []string) (string, error) {
			capturedServerID = serverID
			return "", errors.New("stop here")
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "arm64", "", "fsn1", nil)

	assert.Equal(t, "server-id-999", capturedServerID)
}

// TestBuild_ServerIDPassedToResetServer verifies that the server ID from
// CreateServer is correctly passed to ResetServer.
func TestBuild_ServerIDPassedToResetServer(t *testing.T) {
	t.Parallel()
	var capturedServerID string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-abc", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "server-id-777", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "10.0.0.1", nil
		},
		EnableRescueFunc: func(_ context.Context, _ string, _ []string) (string, error) {
			return "password", nil
		},
		ResetServerFunc: func(_ context.Context, serverID string) error {
			capturedServerID = serverID
			return errors.New("stop here")
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)

	assert.Equal(t, "server-id-777", capturedServerID)
}

// TestBuild_DeferredCleanupsRunOnGetServerIPError verifies cleanup when
// GetServerIP fails (after both SSH key and server have been created).
func TestBuild_DeferredCleanupsRunOnGetServerIPError(t *testing.T) {
	t.Parallel()
	var serverDeleted, sshKeyDeleted bool
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "server-123", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("IP lookup failed")
		},
		DeleteServerFunc: func(_ context.Context, _ string) error {
			serverDeleted = true
			return nil
		},
		DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
			sshKeyDeleted = true
			return nil
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get server IP")
	assert.True(t, serverDeleted, "server cleanup should have been called")
	assert.True(t, sshKeyDeleted, "SSH key cleanup should have been called")
}

// TestBuild_DeferredCleanupsRunOnResetServerError verifies cleanup when
// ResetServer fails.
func TestBuild_DeferredCleanupsRunOnResetServerError(t *testing.T) {
	t.Parallel()
	var serverDeleted, sshKeyDeleted bool
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "server-123", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "1.2.3.4", nil
		},
		EnableRescueFunc: func(_ context.Context, _ string, _ []string) (string, error) {
			return "password", nil
		},
		ResetServerFunc: func(_ context.Context, _ string) error {
			return errors.New("reset failed")
		},
		DeleteServerFunc: func(_ context.Context, _ string) error {
			serverDeleted = true
			return nil
		},
		DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
			sshKeyDeleted = true
			return nil
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to reset server")
	assert.True(t, serverDeleted, "server cleanup should have been called")
	assert.True(t, sshKeyDeleted, "SSH key cleanup should have been called")
}

// TestBuild_BothCleanupsFail verifies error behavior when both cleanup
// functions fail. The original error should be returned, not cleanup errors.
func TestBuild_BothCleanupsFail(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "server-123", nil
		},
		GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("original error: IP lookup failed")
		},
		DeleteServerFunc: func(_ context.Context, _ string) error {
			return errors.New("server cleanup failed")
		},
		DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
			return errors.New("SSH key cleanup failed")
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	// Original error must be preserved
	assert.Contains(t, err.Error(), "failed to get server IP")
	// Cleanup errors should NOT be in the returned error
	assert.NotContains(t, err.Error(), "server cleanup failed")
	assert.NotContains(t, err.Error(), "SSH key cleanup failed")
}

// TestBuild_SSHKeyNameMatchesServerName verifies the SSH key name is derived
// from the server name with a "key-" prefix.
func TestBuild_SSHKeyNameMatchesServerName(t *testing.T) {
	t.Parallel()
	var capturedSSHKeyName, capturedServerName string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, name string, _ string, _ map[string]string) (string, error) {
			capturedSSHKeyName = name
			return "key-id-1", nil
		},
		CreateServerFunc: func(_ context.Context, opts hcloud.ServerCreateOpts) (string, error) {
			capturedServerName = opts.Name
			return "", errors.New("stop here")
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)

	// SSH key name should be "key-" + serverName
	assert.Equal(t, "key-"+capturedServerName, capturedSSHKeyName)
}

// TestBuild_SSHKeyNamesInCreateServer verifies the SSH key name is included
// in the CreateServer call's SSHKeys field.
func TestBuild_SSHKeyNamesInCreateServer(t *testing.T) {
	t.Parallel()
	var capturedSSHKeyName string
	var capturedServerSSHKeys []string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, name string, _ string, _ map[string]string) (string, error) {
			capturedSSHKeyName = name
			return "key-id-1", nil
		},
		CreateServerFunc: func(_ context.Context, opts hcloud.ServerCreateOpts) (string, error) {
			capturedServerSSHKeys = opts.SSHKeys
			return "", errors.New("stop here")
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)

	require.Len(t, capturedServerSSHKeys, 1)
	assert.Equal(t, capturedSSHKeyName, capturedServerSSHKeys[0])
}

// TestBuild_GetServerIPReceivesServerName verifies that GetServerIP is called
// with the server name (not server ID).
func TestBuild_GetServerIPReceivesServerName(t *testing.T) {
	t.Parallel()
	var capturedServerName, capturedGetIPName string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, opts hcloud.ServerCreateOpts) (string, error) {
			capturedServerName = opts.Name
			return "server-id-123", nil
		},
		GetServerIPFunc: func(_ context.Context, name string) (string, error) {
			capturedGetIPName = name
			return "", errors.New("stop here")
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)

	assert.Equal(t, capturedServerName, capturedGetIPName, "GetServerIP should receive server name, not server ID")
}

// TestBuild_NoCleanupSSHKeyOnCreateSSHKeyError verifies that cleanupSSHKey is
// NOT called when CreateSSHKey itself fails, since the defer for SSH key cleanup
// is placed after a successful CreateSSHKey call.
func TestBuild_NoCleanupSSHKeyOnCreateSSHKeyError(t *testing.T) {
	t.Parallel()
	sshKeyDeleteCalled := false
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "", errors.New("SSH key creation failed")
		},
		DeleteSSHKeyFunc: func(_ context.Context, _ string) error {
			sshKeyDeleteCalled = true
			return nil
		},
	}

	builder := NewBuilder(mockClient)
	_, err := builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload ssh key")
	// cleanupSSHKey defer is registered AFTER CreateSSHKey succeeds, so it should NOT be called
	assert.False(t, sshKeyDeleteCalled, "SSH key cleanup should not be called when CreateSSHKey fails")
}

// TestBuild_CustomLocation verifies that a non-empty location is passed through
// to CreateServer without being overridden.
func TestBuild_CustomLocation(t *testing.T) {
	t.Parallel()
	var capturedLocation string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, opts hcloud.ServerCreateOpts) (string, error) {
			capturedLocation = opts.Location
			return "", errors.New("stop here")
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "fsn1", nil)

	assert.Equal(t, "fsn1", capturedLocation)
}

// TestBuild_LabelsNotMutated verifies that the original labels map passed to
// Build is not mutated by the SSH key label merging logic.
func TestBuild_LabelsNotMutated(t *testing.T) {
	t.Parallel()
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "", errors.New("stop here")
		},
	}

	originalLabels := map[string]string{
		"env": "test",
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", originalLabels)

	// The original labels should NOT have the "type" key added
	_, hasType := originalLabels["type"]
	assert.False(t, hasType, "original labels map should not be mutated by Build")
	assert.Len(t, originalLabels, 1, "original labels should still have exactly 1 entry")
}

// TestBuild_PublicKeyPassedToCreateSSHKey verifies that a non-empty public key
// is generated and passed to CreateSSHKey.
func TestBuild_PublicKeyPassedToCreateSSHKey(t *testing.T) {
	t.Parallel()
	var capturedPublicKey string
	mockClient := &hcloud.MockClient{
		CreateSSHKeyFunc: func(_ context.Context, _ string, publicKey string, _ map[string]string) (string, error) {
			capturedPublicKey = publicKey
			return "", errors.New("stop here")
		},
	}

	builder := NewBuilder(mockClient)
	_, _ = builder.Build(context.Background(), "v1.8.0", "v1.30.0", "amd64", "", "nbg1", nil)

	assert.NotEmpty(t, capturedPublicKey, "public key should be generated and passed to CreateSSHKey")
	assert.Contains(t, capturedPublicKey, "ssh-rsa", "public key should be in SSH RSA format")
}
