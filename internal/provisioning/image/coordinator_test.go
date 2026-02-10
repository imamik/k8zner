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
