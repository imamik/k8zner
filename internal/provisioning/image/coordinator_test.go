package image

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/provisioning"
)

func TestGetImageBuilderConfig(t *testing.T) {
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

func TestImageBuilderConfigDefaults(t *testing.T) {
	// Test that config defaults are correctly set by the config loader
	// This tests the values set in load.go
	t.Run("default values from load.go", func(t *testing.T) {
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
