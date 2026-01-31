package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateClusterName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "valid simple name",
			input:     "my-cluster",
			wantError: false,
		},
		{
			name:      "valid with numbers",
			input:     "cluster-123",
			wantError: false,
		},
		{
			name:      "valid lowercase only",
			input:     "mycluster",
			wantError: false,
		},
		{
			name:      "valid numbers only",
			input:     "cluster1",
			wantError: false,
		},
		{
			name:      "empty string",
			input:     "",
			wantError: true,
		},
		{
			name:      "too long (64 chars)",
			input:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantError: true,
		},
		{
			name:      "max length (63 chars)",
			input:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantError: false,
		},
		{
			name:      "uppercase letters (auto-lowercased)",
			input:     "MyCluster",
			wantError: false, // validated after ToLower conversion
		},
		{
			name:      "starts with hyphen",
			input:     "-cluster",
			wantError: true,
		},
		{
			name:      "ends with hyphen",
			input:     "cluster-",
			wantError: true,
		},
		{
			name:      "contains underscore",
			input:     "my_cluster",
			wantError: true,
		},
		{
			name:      "contains space",
			input:     "my cluster",
			wantError: true,
		},
		{
			name:      "contains dot",
			input:     "my.cluster",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateClusterName(tt.input)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "empty is optional",
			input:     "",
			wantError: false,
		},
		{
			name:      "valid simple domain",
			input:     "example.com",
			wantError: false,
		},
		{
			name:      "valid subdomain",
			input:     "sub.example.com",
			wantError: false,
		},
		{
			name:      "valid nested subdomain",
			input:     "deep.sub.example.com",
			wantError: false,
		},
		{
			name:      "valid org tld",
			input:     "example.org",
			wantError: false,
		},
		{
			name:      "invalid no tld",
			input:     "example",
			wantError: true,
		},
		{
			name:      "invalid single part",
			input:     "localhost",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDomain(tt.input)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWizardResult_ToConfig(t *testing.T) {
	t.Run("converts dev mode result", func(t *testing.T) {
		result := &WizardResult{
			Name:        "test-cluster",
			Region:      RegionFalkenstein,
			Mode:        ModeDev,
			WorkerCount: 2,
			WorkerSize:  SizeCX32,
		}

		cfg := result.ToConfig()

		require.NotNil(t, cfg)
		assert.Equal(t, "test-cluster", cfg.Name)
		assert.Equal(t, RegionFalkenstein, cfg.Region)
		assert.Equal(t, ModeDev, cfg.Mode)
		assert.Equal(t, 2, cfg.Workers.Count)
		assert.Equal(t, SizeCX32, cfg.Workers.Size)
		assert.Empty(t, cfg.Domain)
	})

	t.Run("converts ha mode result with domain", func(t *testing.T) {
		result := &WizardResult{
			Name:        "production",
			Region:      RegionNuremberg,
			Mode:        ModeHA,
			WorkerCount: 5,
			WorkerSize:  SizeCX52,
			Domain:      "example.com",
		}

		cfg := result.ToConfig()

		require.NotNil(t, cfg)
		assert.Equal(t, "production", cfg.Name)
		assert.Equal(t, RegionNuremberg, cfg.Region)
		assert.Equal(t, ModeHA, cfg.Mode)
		assert.Equal(t, 5, cfg.Workers.Count)
		assert.Equal(t, SizeCX52, cfg.Workers.Size)
		assert.Equal(t, "example.com", cfg.Domain)
	})

	t.Run("converted config reports correct counts", func(t *testing.T) {
		result := &WizardResult{
			Name:        "test",
			Region:      RegionHelsinki,
			Mode:        ModeHA,
			WorkerCount: 3,
			WorkerSize:  SizeCX42,
		}

		cfg := result.ToConfig()

		// HA mode = 3 control planes
		assert.Equal(t, 3, cfg.ControlPlaneCount())
		// HA mode = 2 load balancers
		assert.Equal(t, 2, cfg.LoadBalancerCount())
	})
}

func TestWriteYAML(t *testing.T) {
	t.Run("writes valid yaml", func(t *testing.T) {
		cfg := &Config{
			Name:   "test-cluster",
			Region: RegionFalkenstein,
			Mode:   ModeDev,
			Workers: Worker{
				Count: 2,
				Size:  SizeCX32,
			},
		}

		// Create temp file
		tmpFile := t.TempDir() + "/test.yaml"

		err := WriteYAML(cfg, tmpFile)
		require.NoError(t, err)

		// Read it back
		loadedCfg, err := LoadWithoutValidation(tmpFile)
		require.NoError(t, err)

		assert.Equal(t, cfg.Name, loadedCfg.Name)
		assert.Equal(t, cfg.Region, loadedCfg.Region)
		assert.Equal(t, cfg.Mode, loadedCfg.Mode)
		assert.Equal(t, cfg.Workers.Count, loadedCfg.Workers.Count)
		assert.Equal(t, cfg.Workers.Size, loadedCfg.Workers.Size)
	})
}
