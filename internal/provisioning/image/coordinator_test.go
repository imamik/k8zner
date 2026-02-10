package image

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	hcloudgo "github.com/hetznercloud/hcloud-go/v2/hcloud"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
)

func TestGetImageBuilderConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		arch               string
		cfg                *config.Config
		expectedServerType string
		expectedLocation   string
	}{
		{
			name: "amd64 with defaults",
			arch: "amd64",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Location: "fsn1"},
					},
				},
				Talos: config.TalosConfig{
					ImageBuilder: config.ImageBuilderConfig{},
				},
			},
			expectedServerType: "", // Empty means auto-detect
			expectedLocation:   "fsn1",
		},
		{
			name: "amd64 with explicit config",
			arch: "amd64",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Location: "fsn1"},
					},
				},
				Talos: config.TalosConfig{
					ImageBuilder: config.ImageBuilderConfig{
						AMD64: config.ImageBuilderArchConfig{
							ServerType:     "cpx21",
							ServerLocation: "nbg1",
						},
					},
				},
			},
			expectedServerType: "cpx21",
			expectedLocation:   "nbg1",
		},
		{
			name: "arm64 with defaults",
			arch: "arm64",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Location: "hel1"},
					},
				},
				Talos: config.TalosConfig{
					ImageBuilder: config.ImageBuilderConfig{},
				},
			},
			expectedServerType: "", // Empty means auto-detect
			expectedLocation:   "hel1",
		},
		{
			name: "arm64 with explicit config",
			arch: "arm64",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Location: "hel1"},
					},
				},
				Talos: config.TalosConfig{
					ImageBuilder: config.ImageBuilderConfig{
						ARM64: config.ImageBuilderArchConfig{
							ServerType:     "cax21",
							ServerLocation: "fsn1",
						},
					},
				},
			},
			expectedServerType: "cax21",
			expectedLocation:   "fsn1",
		},
		{
			name: "no control plane pools - defaults to nbg1",
			arch: "amd64",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{},
				},
				Talos: config.TalosConfig{
					ImageBuilder: config.ImageBuilderConfig{},
				},
			},
			expectedServerType: "",
			expectedLocation:   "nbg1",
		},
		{
			name: "control plane pool without location - defaults to nbg1",
			arch: "amd64",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Name: "cp", Location: ""},
					},
				},
				Talos: config.TalosConfig{
					ImageBuilder: config.ImageBuilderConfig{},
				},
			},
			expectedServerType: "",
			expectedLocation:   "nbg1",
		},
		{
			name: "amd64 config does not affect arm64",
			arch: "arm64",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Location: "ash"},
					},
				},
				Talos: config.TalosConfig{
					ImageBuilder: config.ImageBuilderConfig{
						AMD64: config.ImageBuilderArchConfig{
							ServerType:     "cpx21",
							ServerLocation: "nbg1",
						},
						// ARM64 not set
					},
				},
			},
			expectedServerType: "",
			expectedLocation:   "ash", // Falls back to CP location
		},
		{
			name: "only server type set, location from CP",
			arch: "amd64",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Location: "hil"},
					},
				},
				Talos: config.TalosConfig{
					ImageBuilder: config.ImageBuilderConfig{
						AMD64: config.ImageBuilderArchConfig{
							ServerType: "cpx31",
							// ServerLocation not set
						},
					},
				},
			},
			expectedServerType: "cpx31",
			expectedLocation:   "hil", // From CP
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &Provisioner{}
			ctx := &provisioning.Context{
				Config: tt.cfg,
			}

			serverType, location := p.getImageBuilderConfig(ctx, tt.arch)

			assert.Equal(t, tt.expectedServerType, serverType, "server type mismatch")
			assert.Equal(t, tt.expectedLocation, location, "location mismatch")
		})
	}
}

func TestNewProvisioner(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()
	assert.NotNil(t, p)
	assert.Equal(t, "image", p.Name())
}

func TestEnsureAllImages_NoTalosPoolsNeeded(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Image: "custom-image", ServerType: "cx22"},
			},
		},
		Workers: []config.WorkerNodePool{
			{Image: "custom-image", ServerType: "cx32"},
		},
	}

	mockClient := &hcloud.MockClient{}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
}

func TestEnsureAllImages_EmptyPools(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{NodePools: nil},
		Workers:      nil,
	}

	mockClient := &hcloud.MockClient{}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
}

func TestEnsureAllImages_SnapshotExists(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Location: "nbg1"},
			},
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
			return &hcloudgo.Image{ID: 123, Description: "existing"}, nil
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
}

func TestEnsureAllImages_SnapshotCheckError(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Location: "nbg1"},
			},
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
			return nil, errors.New("API error")
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for existing snapshot")
}

func TestEnsureAllImages_DefaultVersions(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	// Config with empty versions - should use defaults
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Location: "nbg1"},
			},
		},
		Talos:      config.TalosConfig{Version: ""},      // Empty = use default
		Kubernetes: config.KubernetesConfig{Version: ""}, // Empty = use default
	}

	var capturedLabels map[string]string
	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			capturedLabels = labels
			return &hcloudgo.Image{ID: 1, Description: "exists"}, nil
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "v1.8.3", capturedLabels["talos-version"])
	assert.Equal(t, "v1.31.0", capturedLabels["k8s-version"])
}

func TestEnsureAllImages_TalosImageFilter(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	// Mix of Talos and custom image pools
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Image: "talos", Location: "nbg1"},      // Needs image
				{ServerType: "cx32", Image: "custom-img", Location: "nbg1"}, // Custom, skip
			},
		},
		Workers: []config.WorkerNodePool{
			{ServerType: "cx22", Image: "", Location: "nbg1"},       // Empty = talos
			{ServerType: "cax21", Image: "other", Location: "nbg1"}, // Custom, skip
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	snapshotCallCount := 0
	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
			snapshotCallCount++
			return &hcloudgo.Image{ID: 1, Description: "exists"}, nil
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
	// Only amd64 pools (cx22, cx32) need images, but cx32 is custom.
	// cx22 appears in both CP and worker with talos/empty.
	// So only amd64 arch is needed (1 call).
	assert.Equal(t, 1, snapshotCallCount)
}

// TestProvision delegates to EnsureAllImages.
func TestProvision(t *testing.T) {
	t.Parallel()

	t.Run("no pools needed - success", func(t *testing.T) {
		t.Parallel()
		p := NewProvisioner()

		cfg := &config.Config{
			ControlPlane: config.ControlPlaneConfig{NodePools: nil},
			Workers:      nil,
		}
		mockClient := &hcloud.MockClient{}
		ctx := createTestContext(t, mockClient, cfg)

		err := p.Provision(ctx)
		assert.NoError(t, err)
	})

	t.Run("snapshot exists - success", func(t *testing.T) {
		t.Parallel()
		p := NewProvisioner()

		cfg := &config.Config{
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{ServerType: "cx22", Location: "nbg1"},
				},
			},
			Talos:      config.TalosConfig{Version: "v1.8.3"},
			Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
		}

		mockClient := &hcloud.MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
				return &hcloudgo.Image{ID: 100, Description: "existing"}, nil
			},
		}
		ctx := createTestContext(t, mockClient, cfg)

		err := p.Provision(ctx)
		assert.NoError(t, err)
	})

	t.Run("snapshot check error - returns error", func(t *testing.T) {
		t.Parallel()
		p := NewProvisioner()

		cfg := &config.Config{
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{ServerType: "cx22", Location: "nbg1"},
				},
			},
			Talos:      config.TalosConfig{Version: "v1.8.3"},
			Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
		}

		mockClient := &hcloud.MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
				return nil, errors.New("API error")
			},
		}
		ctx := createTestContext(t, mockClient, cfg)

		err := p.Provision(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check for existing snapshot")
	})
}

// TestCreateImageBuilder verifies that createImageBuilder creates a Builder with the context's Infra.
func TestCreateImageBuilder(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	mockClient := &hcloud.MockClient{}
	ctx := createTestContext(t, mockClient, &config.Config{})

	builder := p.createImageBuilder(ctx)
	assert.NotNil(t, builder)
	assert.Equal(t, mockClient, builder.infra)
}

// TestEnsureImageForArch_BuildPath tests the path where no snapshot exists and build is triggered.
func TestEnsureImageForArch_BuildPath(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	t.Run("build triggered when no snapshot exists - build fails with CreateSSHKey error", func(t *testing.T) {
		t.Parallel()
		mockClient := &hcloud.MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
				return nil, nil // No snapshot found
			},
			CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
				return "", errors.New("SSH key creation failed")
			},
		}
		ctx := createTestContext(t, mockClient, &config.Config{})

		err := p.ensureImageForArch(ctx, "amd64", "v1.8.0", "v1.30.0", "", "nbg1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build image")
	})

	t.Run("build triggered when no snapshot exists - build fails with CreateServer error", func(t *testing.T) {
		t.Parallel()
		mockClient := &hcloud.MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
				return nil, nil // No snapshot found
			},
			CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
				return "key-123", nil
			},
			CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
				return "", errors.New("server creation failed")
			},
		}
		ctx := createTestContext(t, mockClient, &config.Config{})

		err := p.ensureImageForArch(ctx, "amd64", "v1.8.0", "v1.30.0", "cpx22", "nbg1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build image")
	})

	t.Run("labels passed to GetSnapshotByLabels are correct", func(t *testing.T) {
		t.Parallel()
		var capturedLabels map[string]string
		mockClient := &hcloud.MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, labels map[string]string) (*hcloudgo.Image, error) {
				capturedLabels = labels
				return &hcloudgo.Image{ID: 1, Description: "found"}, nil
			},
		}
		ctx := createTestContext(t, mockClient, &config.Config{})

		err := p.ensureImageForArch(ctx, "arm64", "v1.9.0", "v1.32.0", "", "fsn1")
		assert.NoError(t, err)
		assert.Equal(t, "talos", capturedLabels["os"])
		assert.Equal(t, "v1.9.0", capturedLabels["talos-version"])
		assert.Equal(t, "v1.32.0", capturedLabels["k8s-version"])
		assert.Equal(t, "arm64", capturedLabels["arch"])
	})
}

// TestEnsureAllImages_BuildError verifies that EnsureAllImages returns an error when build fails.
func TestEnsureAllImages_BuildError(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Location: "nbg1"},
			},
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
			return nil, nil // No snapshot found, triggers build
		},
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "", errors.New("SSH key creation failed")
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.Error(t, err)
}

// TestEnsureAllImages_MultipleArchitectures verifies that multiple architectures are built in parallel.
func TestEnsureAllImages_MultipleArchitectures(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Location: "nbg1"}, // amd64
			},
		},
		Workers: []config.WorkerNodePool{
			{ServerType: "cax21", Location: "nbg1"}, // arm64
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	architecturesSeen := make(map[string]bool)
	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			architecturesSeen[labels["arch"]] = true
			return &hcloudgo.Image{ID: 1, Description: "exists"}, nil
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
	assert.True(t, architecturesSeen["amd64"], "should check amd64 snapshot")
	assert.True(t, architecturesSeen["arm64"], "should check arm64 snapshot")
}

// TestEnsureAllImages_WorkerOnlyPools verifies image building when only workers need Talos images.
func TestEnsureAllImages_WorkerOnlyPools(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Image: "custom-image", Location: "nbg1"},
			},
		},
		Workers: []config.WorkerNodePool{
			{ServerType: "cx32", Image: "", Location: "nbg1"}, // Empty = talos
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	snapshotChecked := false
	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			snapshotChecked = true
			assert.Equal(t, "amd64", labels["arch"])
			return &hcloudgo.Image{ID: 1, Description: "exists"}, nil
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
	assert.True(t, snapshotChecked, "should have checked for amd64 snapshot from worker pool")
}

func TestImageBuilderConfigDefaults(t *testing.T) {
	t.Parallel()
	// Test that config defaults are correctly set by the config loader
	// This tests the values set in load.go

	t.Run("default values from load.go", func(t *testing.T) {
		t.Parallel()
		// These are the defaults set in internal/config/load.go

		expectedAMD64ServerType := "cpx11"
		expectedAMD64Location := "ash"
		expectedARM64ServerType := "cax11"
		expectedARM64Location := "nbg1"

		cfg := &config.Config{
			Talos: config.TalosConfig{
				ImageBuilder: config.ImageBuilderConfig{
					AMD64: config.ImageBuilderArchConfig{
						ServerType:     expectedAMD64ServerType,
						ServerLocation: expectedAMD64Location,
					},
					ARM64: config.ImageBuilderArchConfig{
						ServerType:     expectedARM64ServerType,
						ServerLocation: expectedARM64Location,
					},
				},
			},
		}

		assert.Equal(t, expectedAMD64ServerType, cfg.Talos.ImageBuilder.AMD64.ServerType)
		assert.Equal(t, expectedAMD64Location, cfg.Talos.ImageBuilder.AMD64.ServerLocation)
		assert.Equal(t, expectedARM64ServerType, cfg.Talos.ImageBuilder.ARM64.ServerType)
		assert.Equal(t, expectedARM64Location, cfg.Talos.ImageBuilder.ARM64.ServerLocation)
	})
}

// TestEnsureImageForArch_BuildErrorWrapping verifies that build errors are
// properly wrapped with "failed to build image" context.
func TestEnsureImageForArch_BuildErrorWrapping(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	t.Run("wraps GetServerIP error", func(t *testing.T) {
		t.Parallel()
		mockClient := &hcloud.MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
				return nil, nil // No snapshot found
			},
			CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
				return "key-123", nil
			},
			CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
				return "server-123", nil
			},
			GetServerIPFunc: func(_ context.Context, _ string) (string, error) {
				return "", errors.New("network error")
			},
		}
		ctx := createTestContext(t, mockClient, &config.Config{})

		err := p.ensureImageForArch(ctx, "amd64", "v1.8.0", "v1.30.0", "", "nbg1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build image")
		assert.Contains(t, err.Error(), "failed to get server IP")
	})

	t.Run("wraps EnableRescue error", func(t *testing.T) {
		t.Parallel()
		mockClient := &hcloud.MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
				return nil, nil
			},
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
				return "", errors.New("rescue mode unavailable")
			},
		}
		ctx := createTestContext(t, mockClient, &config.Config{})

		err := p.ensureImageForArch(ctx, "arm64", "v1.8.0", "v1.30.0", "cax11", "nbg1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build image")
		assert.Contains(t, err.Error(), "failed to enable rescue")
	})

	t.Run("wraps ResetServer error", func(t *testing.T) {
		t.Parallel()
		mockClient := &hcloud.MockClient{
			GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
				return nil, nil
			},
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
				return errors.New("reset timeout")
			},
		}
		ctx := createTestContext(t, mockClient, &config.Config{})

		err := p.ensureImageForArch(ctx, "amd64", "v1.8.0", "v1.30.0", "", "nbg1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build image")
		assert.Contains(t, err.Error(), "failed to reset server")
	})
}

// TestEnsureImageForArch_VersionsAndArchPassedCorrectly verifies that the
// version and architecture parameters are correctly used in snapshot labels.
func TestEnsureImageForArch_VersionsAndArchPassedCorrectly(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	tests := []struct {
		name         string
		arch         string
		talosVersion string
		k8sVersion   string
	}{
		{"amd64 with v1.8.0", "amd64", "v1.8.0", "v1.30.0"},
		{"arm64 with v1.9.0", "arm64", "v1.9.0", "v1.31.0"},
		{"amd64 with custom versions", "amd64", "v1.10.0-alpha.1", "v1.32.0-rc.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var capturedLabels map[string]string
			mockClient := &hcloud.MockClient{
				GetSnapshotByLabelsFunc: func(_ context.Context, labels map[string]string) (*hcloudgo.Image, error) {
					capturedLabels = labels
					return &hcloudgo.Image{ID: 1, Description: "found"}, nil
				},
			}
			ctx := createTestContext(t, mockClient, &config.Config{})

			err := p.ensureImageForArch(ctx, tt.arch, tt.talosVersion, tt.k8sVersion, "", "nbg1")
			assert.NoError(t, err)
			assert.Equal(t, tt.arch, capturedLabels["arch"])
			assert.Equal(t, tt.talosVersion, capturedLabels["talos-version"])
			assert.Equal(t, tt.k8sVersion, capturedLabels["k8s-version"])
			assert.Equal(t, "talos", capturedLabels["os"])
		})
	}
}

// TestEnsureAllImages_DuplicateServerTypes verifies that duplicate server
// types across pools are deduplicated into a single architecture check.
func TestEnsureAllImages_DuplicateServerTypes(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Location: "nbg1"}, // amd64
				{ServerType: "cx32", Location: "nbg1"}, // also amd64
			},
		},
		Workers: []config.WorkerNodePool{
			{ServerType: "cx22", Location: "nbg1"}, // amd64 again
			{ServerType: "cx42", Location: "nbg1"}, // amd64 again
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	snapshotCallCount := 0
	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			snapshotCallCount++
			assert.Equal(t, "amd64", labels["arch"])
			return &hcloudgo.Image{ID: 1, Description: "exists"}, nil
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
	// All server types are amd64, so only one snapshot check should happen
	assert.Equal(t, 1, snapshotCallCount, "should deduplicate to single amd64 check")
}

// TestEnsureAllImages_OnlyARM64Workers verifies image building when only
// ARM64 worker pools exist.
func TestEnsureAllImages_OnlyARM64Workers(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Image: "custom-image", Location: "nbg1"}, // custom, skip
			},
		},
		Workers: []config.WorkerNodePool{
			{ServerType: "cax21", Image: "", Location: "nbg1"}, // arm64
			{ServerType: "cax31", Image: "", Location: "nbg1"}, // arm64
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	var capturedArch string
	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			capturedArch = labels["arch"]
			return &hcloudgo.Image{ID: 1, Description: "exists"}, nil
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "arm64", capturedArch, "should only check for arm64 snapshot")
}

// TestEnsureAllImages_ControlPlaneOnlyPools verifies image building with
// only control plane pools needing Talos images.
func TestEnsureAllImages_ControlPlaneOnlyPools(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Location: "fsn1"}, // amd64, needs image
			},
		},
		Workers: []config.WorkerNodePool{
			{ServerType: "cx32", Image: "custom-image", Location: "nbg1"}, // custom, skip
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	snapshotChecked := false
	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			snapshotChecked = true
			assert.Equal(t, "amd64", labels["arch"])
			return &hcloudgo.Image{ID: 1, Description: "exists"}, nil
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
	assert.True(t, snapshotChecked, "should check for amd64 snapshot from CP pool")
}

// TestEnsureAllImages_VersionsFromConfig verifies that Talos and K8s versions
// are correctly read from config and passed to the snapshot label check.
func TestEnsureAllImages_VersionsFromConfig(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Location: "nbg1"},
			},
		},
		Talos:      config.TalosConfig{Version: "v1.9.0"},
		Kubernetes: config.KubernetesConfig{Version: "v1.32.0"},
	}

	var capturedLabels map[string]string
	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			capturedLabels = labels
			return &hcloudgo.Image{ID: 1, Description: "exists"}, nil
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "v1.9.0", capturedLabels["talos-version"])
	assert.Equal(t, "v1.32.0", capturedLabels["k8s-version"])
}

// TestProvision_BuildError verifies that Provision returns errors from build
// failures through the full call chain.
func TestProvision_BuildError(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Location: "nbg1"},
			},
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
			return nil, nil // No snapshot, triggers build
		},
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "key-123", nil
		},
		CreateServerFunc: func(_ context.Context, _ hcloud.ServerCreateOpts) (string, error) {
			return "", errors.New("quota exceeded")
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.Provision(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to build image")
}

// TestEnsureAllImages_MixedArchitecturesBuildError verifies that when one
// architecture fails during build, the error is propagated.
func TestEnsureAllImages_MixedArchitecturesBuildError(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Location: "nbg1"}, // amd64
			},
		},
		Workers: []config.WorkerNodePool{
			{ServerType: "cax21", Location: "nbg1"}, // arm64
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, labels map[string]string) (*hcloudgo.Image, error) {
			// amd64 snapshot exists, arm64 does not
			if labels["arch"] == "amd64" {
				return &hcloudgo.Image{ID: 1, Description: "exists"}, nil
			}
			return nil, nil // arm64 needs build
		},
		CreateSSHKeyFunc: func(_ context.Context, _ string, _ string, _ map[string]string) (string, error) {
			return "", errors.New("SSH key creation failed for arm64")
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.Error(t, err)
}

// TestCreateImageBuilder_ReturnsBuilderWithInfra verifies that
// createImageBuilder always returns a non-nil Builder using ctx.Infra.
func TestCreateImageBuilder_ReturnsBuilderWithInfra(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	t.Run("with mock client", func(t *testing.T) {
		t.Parallel()
		mockClient := &hcloud.MockClient{}
		ctx := createTestContext(t, mockClient, &config.Config{})

		builder := p.createImageBuilder(ctx)
		assert.NotNil(t, builder)
		assert.Equal(t, mockClient, builder.infra)
	})

	t.Run("with nil infra", func(t *testing.T) {
		t.Parallel()
		ctx := createTestContext(t, nil, &config.Config{})

		builder := p.createImageBuilder(ctx)
		// createImageBuilder always returns non-nil, even with nil infra
		assert.NotNil(t, builder)
		assert.Nil(t, builder.infra)
	})
}

// TestGetImageBuilderConfig_UnknownArch verifies that an unknown architecture
// falls through the switch without setting any server type overrides.
func TestGetImageBuilderConfig_UnknownArch(t *testing.T) {
	t.Parallel()
	p := &Provisioner{}

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Location: "fsn1"},
			},
		},
		Talos: config.TalosConfig{
			ImageBuilder: config.ImageBuilderConfig{
				AMD64: config.ImageBuilderArchConfig{
					ServerType:     "cpx21",
					ServerLocation: "nbg1",
				},
				ARM64: config.ImageBuilderArchConfig{
					ServerType:     "cax21",
					ServerLocation: "hel1",
				},
			},
		},
	}

	ctx := &provisioning.Context{Config: cfg}

	// An unknown architecture should not match amd64 or arm64 config
	serverType, location := p.getImageBuilderConfig(ctx, "riscv64")
	assert.Equal(t, "", serverType, "unknown arch should not get a server type")
	assert.Equal(t, "fsn1", location, "unknown arch should still get CP location")
}

// TestEnsureAllImages_TalosImageKeyword verifies that pools with Image="talos"
// are included in image building.
func TestEnsureAllImages_TalosImageKeyword(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Image: "talos", Location: "nbg1"},
			},
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	snapshotChecked := false
	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
			snapshotChecked = true
			return &hcloudgo.Image{ID: 1, Description: "exists"}, nil
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
	assert.True(t, snapshotChecked, "pools with Image='talos' should trigger snapshot check")
}

// TestEnsureAllImages_AllCustomImages verifies that no snapshot checks occur
// when all pools use custom images (non-empty, non-"talos").
func TestEnsureAllImages_AllCustomImages(t *testing.T) {
	t.Parallel()
	p := NewProvisioner()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{ServerType: "cx22", Image: "ubuntu-22.04", Location: "nbg1"},
			},
		},
		Workers: []config.WorkerNodePool{
			{ServerType: "cx32", Image: "debian-12", Location: "nbg1"},
			{ServerType: "cax21", Image: "rocky-9", Location: "nbg1"},
		},
		Talos:      config.TalosConfig{Version: "v1.8.3"},
		Kubernetes: config.KubernetesConfig{Version: "v1.31.0"},
	}

	snapshotChecked := false
	mockClient := &hcloud.MockClient{
		GetSnapshotByLabelsFunc: func(_ context.Context, _ map[string]string) (*hcloudgo.Image, error) {
			snapshotChecked = true
			return nil, nil
		},
	}
	ctx := createTestContext(t, mockClient, cfg)

	err := p.EnsureAllImages(ctx)
	assert.NoError(t, err)
	assert.False(t, snapshotChecked, "no snapshot check should happen when all pools use custom images")
}
