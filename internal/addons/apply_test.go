package addons

import (
	"context"
	"testing"

	"github.com/imamik/k8zner/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestApply_EmptyKubeconfig(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons:      config.AddonsConfig{CCM: config.CCMConfig{Enabled: true}},
	}
	err := Apply(context.Background(), cfg, nil, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig is required")
}

func TestApply_NoAddonsConfigured(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{Enabled: false},
			CSI: config.CSIConfig{Enabled: false},
		},
	}
	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters: []
contexts: []
current-context: ""
users: []`)

	err := Apply(context.Background(), cfg, kubeconfig, 1)
	assert.NoError(t, err)
}

func TestHasEnabledAddons(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected bool
	}{
		{
			name:     "no addons enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{}},
			expected: false,
		},
		{
			name:     "traefik enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{Traefik: config.TraefikConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "cilium enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{Cilium: config.CiliumConfig{Enabled: true}}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, hasEnabledAddons(tt.cfg))
		})
	}
}

func TestGetControlPlaneCount(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected int
	}{
		{
			name: "single node",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{{Name: "cp", Count: 1}},
				},
			},
			expected: 1,
		},
		{
			name: "ha cluster",
			cfg: &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{{Name: "cp", Count: 3}},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, getControlPlaneCount(tt.cfg))
		})
	}
}

func TestValidateAddonConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr string
	}{
		{
			name:    "no addons enabled",
			cfg:     &config.Config{},
			wantErr: "",
		},
		{
			name: "CCM enabled without token",
			cfg: &config.Config{
				Addons: config.AddonsConfig{
					CCM: config.CCMConfig{Enabled: true},
				},
			},
			wantErr: "ccm/csi/operator addons require hcloud_token",
		},
		{
			name: "CCM enabled with token",
			cfg: &config.Config{
				HCloudToken: "test-token",
				Addons: config.AddonsConfig{
					CCM: config.CCMConfig{Enabled: true},
				},
			},
			wantErr: "",
		},
		{
			name: "Cloudflare enabled without API token",
			cfg: &config.Config{
				Addons: config.AddonsConfig{
					Cloudflare: config.CloudflareConfig{Enabled: true},
				},
			},
			wantErr: "cloudflare addon requires api_token",
		},
		{
			name: "ExternalDNS enabled without Cloudflare",
			cfg: &config.Config{
				Addons: config.AddonsConfig{
					ExternalDNS: config.ExternalDNSConfig{Enabled: true},
				},
			},
			wantErr: "external-dns addon requires cloudflare",
		},
		{
			name: "TalosBackup without S3 bucket",
			cfg: &config.Config{
				Addons: config.AddonsConfig{
					TalosBackup: config.TalosBackupConfig{Enabled: true},
				},
			},
			wantErr: "talos-backup addon requires s3_bucket",
		},
		{
			name: "TalosBackup with full config",
			cfg: &config.Config{
				Addons: config.AddonsConfig{
					TalosBackup: config.TalosBackupConfig{
						Enabled:     true,
						S3Bucket:    "test-bucket",
						S3AccessKey: "access-key",
						S3SecretKey: "secret-key",
						S3Endpoint:  "https://s3.example.com",
					},
				},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAddonConfig(tt.cfg)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}
