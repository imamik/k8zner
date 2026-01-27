package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyTalosBackupS3Defaults(t *testing.T) {
	tests := []struct {
		name           string
		inputURL       string
		expectedBucket string
		expectedRegion string
		expectedEndpt  string
	}{
		{
			name:           "valid URL with HTTPS",
			inputURL:       "https://mybucket.fsn1.your-objectstorage.com",
			expectedBucket: "mybucket",
			expectedRegion: "fsn1",
			expectedEndpt:  "https://fsn1.your-objectstorage.com",
		},
		{
			name:           "valid URL without protocol",
			inputURL:       "mybucket.nbg1.your-objectstorage.com",
			expectedBucket: "mybucket",
			expectedRegion: "nbg1",
			expectedEndpt:  "https://nbg1.your-objectstorage.com",
		},
		{
			name:           "valid URL with trailing dot",
			inputURL:       "mybucket.hel1.your-objectstorage.com.",
			expectedBucket: "mybucket",
			expectedRegion: "hel1",
			expectedEndpt:  "https://hel1.your-objectstorage.com",
		},
		{
			name:           "invalid URL - not hcloud format",
			inputURL:       "s3.amazonaws.com/mybucket",
			expectedBucket: "",
			expectedRegion: "",
			expectedEndpt:  "",
		},
		{
			name:           "empty URL",
			inputURL:       "",
			expectedBucket: "",
			expectedRegion: "",
			expectedEndpt:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Addons: AddonsConfig{
					TalosBackup: TalosBackupConfig{
						S3HcloudURL: tt.inputURL,
					},
				},
			}

			applyTalosBackupS3Defaults(cfg)

			// For valid URLs, check derived values
			if tt.expectedBucket != "" {
				assert.Equal(t, tt.expectedBucket, cfg.Addons.TalosBackup.S3Bucket)
				assert.Equal(t, tt.expectedRegion, cfg.Addons.TalosBackup.S3Region)
				assert.Equal(t, tt.expectedEndpt, cfg.Addons.TalosBackup.S3Endpoint)
			}
		})
	}
}

func TestApplyTalosBackupS3DefaultsPreservesExisting(t *testing.T) {
	// If values are already set, they should not be overwritten
	cfg := &Config{
		Addons: AddonsConfig{
			TalosBackup: TalosBackupConfig{
				S3HcloudURL: "https://mybucket.fsn1.your-objectstorage.com",
				S3Bucket:    "existing-bucket",
				S3Region:    "existing-region",
				S3Endpoint:  "existing-endpoint",
			},
		},
	}

	applyTalosBackupS3Defaults(cfg)

	// Existing values should be preserved
	assert.Equal(t, "existing-bucket", cfg.Addons.TalosBackup.S3Bucket)
	assert.Equal(t, "existing-region", cfg.Addons.TalosBackup.S3Region)
	assert.Equal(t, "existing-endpoint", cfg.Addons.TalosBackup.S3Endpoint)
}

func TestHcloudS3URLRegex(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		matches bool
		bucket  string
		region  string
	}{
		{
			name:    "basic URL",
			url:     "mybucket.fsn1.your-objectstorage.com",
			matches: true,
			bucket:  "mybucket",
			region:  "fsn1",
		},
		{
			name:    "URL with https",
			url:     "https://mybucket.nbg1.your-objectstorage.com",
			matches: true,
			bucket:  "mybucket",
			region:  "nbg1",
		},
		{
			name:    "URL with http",
			url:     "http://mybucket.hel1.your-objectstorage.com",
			matches: true,
			bucket:  "mybucket",
			region:  "hel1",
		},
		{
			name:    "URL with trailing dot",
			url:     "mybucket.fsn1.your-objectstorage.com.",
			matches: true,
			bucket:  "mybucket",
			region:  "fsn1",
		},
		{
			name:    "AWS S3 URL - should not match",
			url:     "s3.amazonaws.com",
			matches: false,
		},
		{
			name:    "random URL - should not match",
			url:     "example.com",
			matches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := hcloudS3URLRegex.FindStringSubmatch(tt.url)
			if tt.matches {
				if len(matches) != 3 {
					t.Errorf("expected match with 3 groups, got %d", len(matches))
					return
				}
				assert.Equal(t, tt.bucket, matches[1], "bucket mismatch")
				assert.Equal(t, tt.region, matches[2], "region mismatch")
			} else if len(matches) > 0 {
				t.Errorf("expected no match, got %v", matches)
			}
		})
	}
}

func TestShouldEnableAddonByDefault(t *testing.T) {
	// When rawConfig is nil, addons should be enabled by default
	result := shouldEnableAddonByDefault(nil, "some_addon")
	assert.True(t, result, "addon should be enabled by default when rawConfig is nil")

	// When rawConfig has the addon key, it should not be enabled by default
	rawConfig := map[string]interface{}{
		"addons": map[string]interface{}{
			"some_addon": map[string]interface{}{
				"enabled": false,
			},
		},
	}
	result = shouldEnableAddonByDefault(rawConfig, "some_addon")
	assert.False(t, result, "addon should not be enabled when explicitly set in rawConfig")

	// When rawConfig has addons but not this specific addon
	rawConfig2 := map[string]interface{}{
		"addons": map[string]interface{}{
			"other_addon": map[string]interface{}{},
		},
	}
	result = shouldEnableAddonByDefault(rawConfig2, "some_addon")
	assert.True(t, result, "addon should be enabled when not in rawConfig")

	// When addons key exists but is not a map
	rawConfig3 := map[string]interface{}{
		"addons": "not a map",
	}
	result = shouldEnableAddonByDefault(rawConfig3, "some_addon")
	assert.True(t, result, "addon should be enabled when addons is not a map")
}

func TestApplyAutoCalculatedDefaults_MetricsServerReplicas(t *testing.T) {
	t.Run("1 replica with 0 workers", func(t *testing.T) {
		cfg := &Config{
			Addons: AddonsConfig{
				MetricsServer: MetricsServerConfig{
					Enabled: true,
				},
			},
			Workers: []WorkerNodePool{},
		}
		applyAutoCalculatedDefaults(cfg)
		assert.NotNil(t, cfg.Addons.MetricsServer.Replicas)
		assert.Equal(t, 1, *cfg.Addons.MetricsServer.Replicas)
	})

	t.Run("1 replica with 1 worker", func(t *testing.T) {
		cfg := &Config{
			Addons: AddonsConfig{
				MetricsServer: MetricsServerConfig{
					Enabled: true,
				},
			},
			Workers: []WorkerNodePool{
				{Count: 1},
			},
		}
		applyAutoCalculatedDefaults(cfg)
		assert.NotNil(t, cfg.Addons.MetricsServer.Replicas)
		assert.Equal(t, 1, *cfg.Addons.MetricsServer.Replicas)
	})

	t.Run("2 replicas with 2+ workers", func(t *testing.T) {
		cfg := &Config{
			Addons: AddonsConfig{
				MetricsServer: MetricsServerConfig{
					Enabled: true,
				},
			},
			Workers: []WorkerNodePool{
				{Count: 2},
			},
		}
		applyAutoCalculatedDefaults(cfg)
		assert.NotNil(t, cfg.Addons.MetricsServer.Replicas)
		assert.Equal(t, 2, *cfg.Addons.MetricsServer.Replicas)
	})

	t.Run("preserves existing replicas", func(t *testing.T) {
		replicas := 5
		cfg := &Config{
			Addons: AddonsConfig{
				MetricsServer: MetricsServerConfig{
					Enabled:  true,
					Replicas: &replicas,
				},
			},
		}
		applyAutoCalculatedDefaults(cfg)
		assert.Equal(t, 5, *cfg.Addons.MetricsServer.Replicas)
	})

	t.Run("skips when metrics server disabled", func(t *testing.T) {
		cfg := &Config{
			Addons: AddonsConfig{
				MetricsServer: MetricsServerConfig{
					Enabled: false,
				},
			},
		}
		applyAutoCalculatedDefaults(cfg)
		assert.Nil(t, cfg.Addons.MetricsServer.Replicas)
	})
}

func TestApplyAutoCalculatedDefaults_MetricsServerScheduleOnCP(t *testing.T) {
	t.Run("schedule on CP when no workers", func(t *testing.T) {
		cfg := &Config{
			Addons: AddonsConfig{
				MetricsServer: MetricsServerConfig{
					Enabled: true,
				},
			},
			Workers: []WorkerNodePool{},
		}
		applyAutoCalculatedDefaults(cfg)
		assert.NotNil(t, cfg.Addons.MetricsServer.ScheduleOnControlPlane)
		assert.True(t, *cfg.Addons.MetricsServer.ScheduleOnControlPlane)
	})

	t.Run("don't schedule on CP when workers exist", func(t *testing.T) {
		cfg := &Config{
			Addons: AddonsConfig{
				MetricsServer: MetricsServerConfig{
					Enabled: true,
				},
			},
			Workers: []WorkerNodePool{
				{Count: 1},
			},
		}
		applyAutoCalculatedDefaults(cfg)
		assert.NotNil(t, cfg.Addons.MetricsServer.ScheduleOnControlPlane)
		assert.False(t, *cfg.Addons.MetricsServer.ScheduleOnControlPlane)
	})
}

func TestApplyAutoCalculatedDefaults_IngressNginxReplicas(t *testing.T) {
	t.Run("2 replicas with <3 workers", func(t *testing.T) {
		cfg := &Config{
			Addons: AddonsConfig{
				IngressNginx: IngressNginxConfig{
					Enabled: true,
					Kind:    "Deployment",
				},
			},
			Workers: []WorkerNodePool{
				{Count: 2},
			},
		}
		applyAutoCalculatedDefaults(cfg)
		assert.NotNil(t, cfg.Addons.IngressNginx.Replicas)
		assert.Equal(t, 2, *cfg.Addons.IngressNginx.Replicas)
	})

	t.Run("3 replicas with 3+ workers", func(t *testing.T) {
		cfg := &Config{
			Addons: AddonsConfig{
				IngressNginx: IngressNginxConfig{
					Enabled: true,
					Kind:    "Deployment",
				},
			},
			Workers: []WorkerNodePool{
				{Count: 3},
			},
		}
		applyAutoCalculatedDefaults(cfg)
		assert.NotNil(t, cfg.Addons.IngressNginx.Replicas)
		assert.Equal(t, 3, *cfg.Addons.IngressNginx.Replicas)
	})

	t.Run("skips DaemonSet kind", func(t *testing.T) {
		cfg := &Config{
			Addons: AddonsConfig{
				IngressNginx: IngressNginxConfig{
					Enabled: true,
					Kind:    "DaemonSet",
				},
			},
		}
		applyAutoCalculatedDefaults(cfg)
		assert.Nil(t, cfg.Addons.IngressNginx.Replicas)
	})
}

func TestApplyAutoCalculatedDefaults_FirewallDefaults(t *testing.T) {
	t.Run("sets firewall IP defaults for public cluster", func(t *testing.T) {
		cfg := &Config{
			ClusterAccess: "public",
		}
		applyAutoCalculatedDefaults(cfg)
		assert.NotNil(t, cfg.Firewall.UseCurrentIPv4)
		assert.True(t, *cfg.Firewall.UseCurrentIPv4)
		assert.NotNil(t, cfg.Firewall.UseCurrentIPv6)
		assert.True(t, *cfg.Firewall.UseCurrentIPv6)
	})

	t.Run("skips for non-public cluster", func(t *testing.T) {
		cfg := &Config{
			ClusterAccess: "private",
		}
		applyAutoCalculatedDefaults(cfg)
		assert.Nil(t, cfg.Firewall.UseCurrentIPv4)
		assert.Nil(t, cfg.Firewall.UseCurrentIPv6)
	})

	t.Run("preserves existing values", func(t *testing.T) {
		falseVal := false
		cfg := &Config{
			ClusterAccess: "public",
			Firewall: FirewallConfig{
				UseCurrentIPv4: &falseVal,
				UseCurrentIPv6: &falseVal,
			},
		}
		applyAutoCalculatedDefaults(cfg)
		assert.False(t, *cfg.Firewall.UseCurrentIPv4)
		assert.False(t, *cfg.Firewall.UseCurrentIPv6)
	})
}
