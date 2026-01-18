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
			} else {
				if len(matches) > 0 {
					t.Errorf("expected no match, got %v", matches)
				}
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
}
