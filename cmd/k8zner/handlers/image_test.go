package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockImageBuilder implements ImageBuilder for testing.
type mockImageBuilder struct {
	snapshotID string
	err        error
}

func (m *mockImageBuilder) Build(_ context.Context, _, _, _, _, _ string, _ map[string]string) (string, error) {
	return m.snapshotID, m.err
}

// saveAndRestoreImageFactories saves and restores image factory functions.
func saveAndRestoreImageFactories(t *testing.T) {
	origNewImageBuilder := newImageBuilder
	origNewInfraClient := newInfraClient

	t.Cleanup(func() {
		newImageBuilder = origNewImageBuilder
		newInfraClient = origNewInfraClient
	})
}

func TestBuild_MissingToken(t *testing.T) {
	// t.Setenv clears the token and automatically restores it after the test
	t.Setenv("HCLOUD_TOKEN", "")

	// Build should fail due to missing token
	ctx := context.Background()
	err := Build(ctx, "test-image", "v1.8.3", "amd64", "nbg1")

	// The error will be from the hcloud client validation
	assert.Error(t, err)
}

func TestBuild_InvalidParameters(t *testing.T) {
	saveAndRestoreImageFactories(t)

	tests := []struct {
		name         string
		imageName    string
		talosVersion string
		arch         string
		location     string
		expectError  bool
	}{
		{
			name:         "empty image name - builds with auto-generated name",
			imageName:    "",
			talosVersion: "v1.8.3",
			arch:         "amd64",
			location:     "nbg1",
			expectError:  false, // Empty image name is allowed, auto-generates one
		},
		{
			name:         "empty talos version - uses default",
			imageName:    "test-image",
			talosVersion: "",
			arch:         "amd64",
			location:     "nbg1",
			expectError:  false, // Empty version is allowed, uses default
		},
		{
			name:         "invalid arch",
			imageName:    "test-image",
			talosVersion: "v1.8.3",
			arch:         "invalid-arch",
			location:     "nbg1",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use mock to avoid hitting real API
			newInfraClient = func(_ string) hcloud.InfrastructureManager {
				return &hcloud.MockClient{}
			}

			if tt.expectError {
				newImageBuilder = func(_ hcloud.InfrastructureManager) ImageBuilder {
					return &mockImageBuilder{err: errors.New("invalid arch")}
				}
			} else {
				newImageBuilder = func(_ hcloud.InfrastructureManager) ImageBuilder {
					return &mockImageBuilder{snapshotID: "snap-test"}
				}
			}

			ctx := context.Background()
			err := Build(ctx, tt.imageName, tt.talosVersion, tt.arch, tt.location)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBuild_WithInjection(t *testing.T) {
	saveAndRestoreImageFactories(t)

	t.Run("success flow", func(t *testing.T) {
		newInfraClient = func(_ string) hcloud.InfrastructureManager {
			return &hcloud.MockClient{}
		}

		newImageBuilder = func(_ hcloud.InfrastructureManager) ImageBuilder {
			return &mockImageBuilder{snapshotID: "snap-12345"}
		}

		err := Build(context.Background(), "test-image", "v1.9.0", "amd64", "nbg1")
		require.NoError(t, err)
	})

	t.Run("builder error", func(t *testing.T) {
		newInfraClient = func(_ string) hcloud.InfrastructureManager {
			return &hcloud.MockClient{}
		}

		newImageBuilder = func(_ hcloud.InfrastructureManager) ImageBuilder {
			return &mockImageBuilder{err: errors.New("server creation failed")}
		}

		err := Build(context.Background(), "test-image", "v1.9.0", "amd64", "nbg1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "build failed")
	})

	t.Run("different architectures", func(t *testing.T) {
		archs := []string{"amd64", "arm64"}

		for _, arch := range archs {
			t.Run(arch, func(t *testing.T) {
				var capturedArch string

				newInfraClient = func(_ string) hcloud.InfrastructureManager {
					return &hcloud.MockClient{}
				}

				newImageBuilder = func(_ hcloud.InfrastructureManager) ImageBuilder {
					return &mockImageBuilder{
						snapshotID: "snap-" + arch,
					}
				}

				// Create a custom builder that captures the arch
				origBuilder := newImageBuilder
				newImageBuilder = func(client hcloud.InfrastructureManager) ImageBuilder {
					builder := origBuilder(client)
					return &archCapturingBuilder{
						inner:        builder,
						capturedArch: &capturedArch,
					}
				}

				err := Build(context.Background(), "test-image", "v1.9.0", arch, "nbg1")
				require.NoError(t, err)
				assert.Equal(t, arch, capturedArch)
			})
		}
	})

	t.Run("different locations", func(t *testing.T) {
		locations := []string{"nbg1", "fsn1", "hel1"}

		for _, loc := range locations {
			t.Run(loc, func(t *testing.T) {
				newInfraClient = func(_ string) hcloud.InfrastructureManager {
					return &hcloud.MockClient{}
				}

				newImageBuilder = func(_ hcloud.InfrastructureManager) ImageBuilder {
					return &mockImageBuilder{snapshotID: "snap-" + loc}
				}

				err := Build(context.Background(), "test-image", "v1.9.0", "amd64", loc)
				require.NoError(t, err)
			})
		}
	})
}

// archCapturingBuilder wraps ImageBuilder to capture architecture parameter.
type archCapturingBuilder struct {
	inner        ImageBuilder
	capturedArch *string
}

func (b *archCapturingBuilder) Build(ctx context.Context, talosVersion, k8sVersion, architecture, serverType, location string, labels map[string]string) (string, error) {
	*b.capturedArch = architecture
	return b.inner.Build(ctx, talosVersion, k8sVersion, architecture, serverType, location, labels)
}
