package config

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
		{"valid simple name", "my-cluster", false},
		{"valid with numbers", "cluster-123", false},
		{"valid lowercase only", "mycluster", false},
		{"valid numbers only", "cluster1", false},
		{"empty string", "", true},
		{"too long (64 chars)", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"max length (63 chars)", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"uppercase letters (auto-lowercased)", "MyCluster", false},
		{"starts with hyphen", "-cluster", true},
		{"ends with hyphen", "cluster-", true},
		{"contains underscore", "my_cluster", true},
		{"contains space", "my cluster", true},
		{"contains dot", "my.cluster", true},
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
		{"empty is optional", "", false},
		{"valid simple domain", "example.com", false},
		{"valid subdomain", "sub.example.com", false},
		{"valid nested subdomain", "deep.sub.example.com", false},
		{"valid org tld", "example.org", false},
		{"invalid no tld", "example", true},
		{"invalid single part", "localhost", true},
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

func TestWizardResult_ToSpec(t *testing.T) {
	t.Run("converts dev mode result", func(t *testing.T) {
		result := &WizardResult{
			Name:        "test-cluster",
			Region:      RegionFalkenstein,
			Mode:        ModeDev,
			WorkerCount: 2,
			WorkerSize:  SizeCX32,
		}

		cfg := result.ToSpec()

		require.NotNil(t, cfg)
		assert.Equal(t, "test-cluster", cfg.Name)
		assert.Equal(t, RegionFalkenstein, cfg.Region)
		assert.Equal(t, ModeDev, cfg.Mode)
		assert.Equal(t, 2, cfg.Workers.Count)
		assert.Equal(t, SizeCX32, cfg.Workers.Size)
		assert.Empty(t, cfg.Domain)
		assert.NotNil(t, cfg.ControlPlane)
		assert.Equal(t, SizeCX32, cfg.ControlPlane.Size)
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

		cfg := result.ToSpec()

		require.NotNil(t, cfg)
		assert.Equal(t, "production", cfg.Name)
		assert.Equal(t, RegionNuremberg, cfg.Region)
		assert.Equal(t, ModeHA, cfg.Mode)
		assert.Equal(t, 5, cfg.Workers.Count)
		assert.Equal(t, SizeCX52, cfg.Workers.Size)
		assert.Equal(t, "example.com", cfg.Domain)
		assert.NotNil(t, cfg.ControlPlane)
		assert.Equal(t, SizeCX52, cfg.ControlPlane.Size)
		// Domain-dependent defaults
		assert.Equal(t, "argo", cfg.ArgoSubdomain)
		assert.Equal(t, "admin@example.com", cfg.CertEmail)
		assert.Equal(t, "grafana", cfg.GrafanaSubdomain)
		assert.True(t, cfg.Monitoring)
	})

	t.Run("no domain skips domain-dependent features", func(t *testing.T) {
		result := &WizardResult{
			Name:        "minimal",
			Region:      RegionFalkenstein,
			Mode:        ModeDev,
			WorkerCount: 1,
			WorkerSize:  SizeCX23,
		}

		cfg := result.ToSpec()

		assert.Empty(t, cfg.ArgoSubdomain)
		assert.Empty(t, cfg.GrafanaSubdomain)
		assert.False(t, cfg.Monitoring)
	})

	t.Run("converted config reports correct counts", func(t *testing.T) {
		result := &WizardResult{
			Name:        "test",
			Region:      RegionHelsinki,
			Mode:        ModeHA,
			WorkerCount: 3,
			WorkerSize:  SizeCX42,
		}

		cfg := result.ToSpec()

		assert.Equal(t, 3, cfg.ControlPlaneCount())
		assert.Equal(t, 2, cfg.LoadBalancerCount())
	})
}

func TestWriteSpecYAML(t *testing.T) {
	t.Run("writes valid spec yaml", func(t *testing.T) {
		cfg := &Spec{
			Name:   "test-cluster",
			Region: RegionFalkenstein,
			Mode:   ModeDev,
			Workers: WorkerSpec{
				Count: 2,
				Size:  SizeCX32,
			},
		}

		tmpFile := t.TempDir() + "/test.yaml"

		err := WriteSpecYAML(cfg, tmpFile)
		require.NoError(t, err)

		// Should round-trip as Spec (not expanded Config)
		loaded, err := LoadSpec(tmpFile)
		require.NoError(t, err)

		assert.Equal(t, cfg.Name, loaded.Name)
		assert.Equal(t, cfg.Region, loaded.Region)
		assert.Equal(t, cfg.Mode, loaded.Mode)
		assert.Equal(t, cfg.Workers.Count, loaded.Workers.Count)
		assert.Equal(t, cfg.Workers.Size, loaded.Workers.Size)
	})

	t.Run("round-trips through LoadSpec and ExpandSpec", func(t *testing.T) {
		t.Setenv("CF_API_TOKEN", "test-token")

		cfg := &Spec{
			Name:   "roundtrip",
			Region: RegionNuremberg,
			Mode:   ModeHA,
			Workers: WorkerSpec{
				Count: 3,
				Size:  SizeCX43,
			},
			Domain:            "example.com",
			ArgoSubdomain:     "argo",
			GrafanaSubdomain:  "grafana",
			Monitoring:        true,
		}

		tmpFile := t.TempDir() + "/test.yaml"

		err := WriteSpecYAML(cfg, tmpFile)
		require.NoError(t, err)

		// Load as Spec
		loaded, err := LoadSpec(tmpFile)
		require.NoError(t, err)
		assert.Equal(t, "example.com", loaded.Domain)
		assert.Equal(t, "argo", loaded.ArgoSubdomain)
		assert.True(t, loaded.Monitoring)

		// Expand to full Config
		expanded, err := ExpandSpec(loaded)
		require.NoError(t, err)
		assert.Equal(t, "roundtrip", expanded.ClusterName)
		assert.Equal(t, "nbg1", expanded.Location)
		assert.Equal(t, 3, expanded.ControlPlane.NodePools[0].Count)
	})
}
