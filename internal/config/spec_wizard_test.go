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
}

// TestInitToApplyPipeline verifies the full init → apply/cost/doctor pipeline.
// This is the critical integration test: init writes a file, and the same
// LoadSpec → ExpandSpec chain that apply/cost/doctor use must work on it.
func TestInitToApplyPipeline(t *testing.T) {
	t.Run("minimal dev cluster without domain", func(t *testing.T) {
		// Simulate wizard result
		result := &WizardResult{
			Name:        "my-dev",
			Region:      RegionFalkenstein,
			Mode:        ModeDev,
			WorkerCount: 1,
			WorkerSize:  SizeCX23,
		}

		spec := result.ToSpec()
		tmpFile := t.TempDir() + "/k8zner.yaml"

		// init writes the file
		err := WriteSpecYAML(spec, tmpFile)
		require.NoError(t, err)

		// apply/cost/doctor load it: LoadSpec → ExpandSpec
		loaded, err := LoadSpec(tmpFile)
		require.NoError(t, err)
		assert.Equal(t, "my-dev", loaded.Name)
		assert.Equal(t, RegionFalkenstein, loaded.Region)
		assert.Equal(t, ModeDev, loaded.Mode)
		assert.Equal(t, 1, loaded.Workers.Count)
		assert.Equal(t, SizeCX23, loaded.Workers.Size)
		assert.Empty(t, loaded.Domain)
		assert.False(t, loaded.Monitoring)

		expanded, err := ExpandSpec(loaded)
		require.NoError(t, err)

		// Verify expanded config has correct values
		assert.Equal(t, "my-dev", expanded.ClusterName)
		assert.Equal(t, "fsn1", expanded.Location)

		// Control plane: dev = 1 CP node
		require.Len(t, expanded.ControlPlane.NodePools, 1)
		assert.Equal(t, 1, expanded.ControlPlane.NodePools[0].Count)
		assert.Equal(t, "cx23", expanded.ControlPlane.NodePools[0].ServerType)

		// Workers
		require.Len(t, expanded.Workers, 1)
		assert.Equal(t, 1, expanded.Workers[0].Count)
		assert.Equal(t, "cx23", expanded.Workers[0].ServerType)

		// Core addons always enabled
		assert.True(t, expanded.Addons.CCM.Enabled)
		assert.True(t, expanded.Addons.CSI.Enabled)
		assert.True(t, expanded.Addons.Cilium.Enabled)
		assert.True(t, expanded.Addons.Traefik.Enabled)
		assert.True(t, expanded.Addons.ArgoCD.Enabled)
		assert.True(t, expanded.Addons.MetricsServer.Enabled)

		// cert-manager always enabled, but Cloudflare DNS01 only with domain
		assert.True(t, expanded.Addons.CertManager.Enabled)
		assert.False(t, expanded.Addons.CertManager.Cloudflare.Enabled)
		assert.False(t, expanded.Addons.ExternalDNS.Enabled)
	})

	t.Run("full-featured HA cluster with domain", func(t *testing.T) {
		t.Setenv("CF_API_TOKEN", "test-token")

		result := &WizardResult{
			Name:        "production",
			Region:      RegionNuremberg,
			Mode:        ModeHA,
			WorkerCount: 3,
			WorkerSize:  SizeCX43,
			Domain:      "example.com",
		}

		spec := result.ToSpec()
		tmpFile := t.TempDir() + "/k8zner.yaml"

		// Verify ToSpec populated domain-dependent fields
		assert.Equal(t, "argo", spec.ArgoSubdomain)
		assert.Equal(t, "grafana", spec.GrafanaSubdomain)
		assert.Equal(t, "admin@example.com", spec.CertEmail)
		assert.True(t, spec.Monitoring)

		// init writes the file
		err := WriteSpecYAML(spec, tmpFile)
		require.NoError(t, err)

		// apply/cost/doctor load it: LoadSpec → ExpandSpec
		loaded, err := LoadSpec(tmpFile)
		require.NoError(t, err)

		// All fields must survive the round-trip
		assert.Equal(t, "production", loaded.Name)
		assert.Equal(t, RegionNuremberg, loaded.Region)
		assert.Equal(t, ModeHA, loaded.Mode)
		assert.Equal(t, 3, loaded.Workers.Count)
		assert.Equal(t, SizeCX43, loaded.Workers.Size)
		assert.Equal(t, "example.com", loaded.Domain)
		assert.Equal(t, "argo", loaded.ArgoSubdomain)
		assert.Equal(t, "grafana", loaded.GrafanaSubdomain)
		assert.Equal(t, "admin@example.com", loaded.CertEmail)
		assert.True(t, loaded.Monitoring)
		require.NotNil(t, loaded.ControlPlane)
		assert.Equal(t, SizeCX43, loaded.ControlPlane.Size)

		expanded, err := ExpandSpec(loaded)
		require.NoError(t, err)

		// Verify expanded config
		assert.Equal(t, "production", expanded.ClusterName)
		assert.Equal(t, "nbg1", expanded.Location)

		// HA = 3 CP nodes
		require.Len(t, expanded.ControlPlane.NodePools, 1)
		assert.Equal(t, 3, expanded.ControlPlane.NodePools[0].Count)
		assert.Equal(t, "cx43", expanded.ControlPlane.NodePools[0].ServerType)

		// Workers
		require.Len(t, expanded.Workers, 1)
		assert.Equal(t, 3, expanded.Workers[0].Count)
		assert.Equal(t, "cx43", expanded.Workers[0].ServerType)

		// Domain-dependent addons
		assert.True(t, expanded.Addons.CertManager.Enabled)
		assert.Equal(t, "admin@example.com", expanded.Addons.CertManager.Cloudflare.Email)
		assert.True(t, expanded.Addons.ExternalDNS.Enabled)

		// ArgoCD with domain
		assert.True(t, expanded.Addons.ArgoCD.Enabled)
		assert.Equal(t, "argo.example.com", expanded.Addons.ArgoCD.IngressHost)

		// Monitoring
		assert.True(t, expanded.Addons.KubePrometheusStack.Enabled)
		assert.Equal(t, "grafana.example.com", expanded.Addons.KubePrometheusStack.Grafana.IngressHost)

		// Network defaults populated
		assert.NotEmpty(t, expanded.Network.IPv4CIDR)
	})

	t.Run("cost command can use expanded config", func(t *testing.T) {
		// This test verifies the cost command's path:
		// loadConfig → LoadSpec → ExpandSpec → buildCostSummary
		// buildCostSummary needs: ClusterName, Workers, ControlPlane, Ingress, Addons

		result := &WizardResult{
			Name:        "cost-test",
			Region:      RegionHelsinki,
			Mode:        ModeDev,
			WorkerCount: 2,
			WorkerSize:  SizeCPX32,
		}

		spec := result.ToSpec()
		tmpFile := t.TempDir() + "/k8zner.yaml"

		err := WriteSpecYAML(spec, tmpFile)
		require.NoError(t, err)

		loaded, err := LoadSpec(tmpFile)
		require.NoError(t, err)

		expanded, err := ExpandSpec(loaded)
		require.NoError(t, err)

		// Cost command needs these fields to be populated
		assert.Equal(t, "cost-test", expanded.ClusterName)
		assert.Equal(t, "hel1", expanded.Location)

		// Workers must have ServerType and Location for pricing lookup
		require.Len(t, expanded.Workers, 1)
		assert.Equal(t, "cpx32", expanded.Workers[0].ServerType)
		assert.Equal(t, "hel1", expanded.Workers[0].Location)
		assert.Equal(t, 2, expanded.Workers[0].Count)

		// CP must have ServerType and Location
		require.Len(t, expanded.ControlPlane.NodePools, 1)
		assert.NotEmpty(t, expanded.ControlPlane.NodePools[0].ServerType)
		assert.Equal(t, "hel1", expanded.ControlPlane.NodePools[0].Location)

		// Traefik creates LB dynamically via CCM (not pre-provisioned ingress)
		assert.True(t, expanded.Addons.Traefik.Enabled)
	})

	t.Run("examples round-trip correctly", func(t *testing.T) {
		// Verify the example files from examples/ can be loaded and expanded
		examples := []struct {
			name   string
			spec   *Spec
			needCF bool
		}{
			{
				name: "dev",
				spec: &Spec{
					Name:   "dev",
					Region: RegionFalkenstein,
					Mode:   ModeDev,
					Workers: WorkerSpec{
						Count: 1,
						Size:  SizeCX23,
					},
				},
			},
			{
				name:   "full-production",
				needCF: true,
				spec: &Spec{
					Name:   "large-prod",
					Region: RegionFalkenstein,
					Mode:   ModeHA,
					Workers: WorkerSpec{
						Count: 5,
						Size:  SizeCX53,
					},
					ControlPlane:     &ControlPlaneSpec{Size: SizeCX43},
					Domain:           "example.com",
					CertEmail:        "ops@example.com",
					ArgoSubdomain:    "argocd",
					Monitoring:       true,
					GrafanaSubdomain: "metrics",
					Backup:           true,
				},
			},
		}

		for _, ex := range examples {
			t.Run(ex.name, func(t *testing.T) {
				if ex.needCF {
					t.Setenv("CF_API_TOKEN", "test-token")
				}
				if ex.spec.Backup {
					t.Setenv("HETZNER_S3_ACCESS_KEY", "test")
					t.Setenv("HETZNER_S3_SECRET_KEY", "test")
				}

				tmpFile := t.TempDir() + "/k8zner.yaml"

				// Write
				err := WriteSpecYAML(ex.spec, tmpFile)
				require.NoError(t, err)

				// Load
				loaded, err := LoadSpec(tmpFile)
				require.NoError(t, err)
				assert.Equal(t, ex.spec.Name, loaded.Name)

				// Expand (same path as apply/cost/doctor)
				expanded, err := ExpandSpec(loaded)
				require.NoError(t, err)
				assert.Equal(t, ex.spec.Name, expanded.ClusterName)
				assert.Equal(t, string(ex.spec.Region), expanded.Location)

				// Must have workers and CP for cost to work
				require.NotEmpty(t, expanded.Workers)
				require.NotEmpty(t, expanded.ControlPlane.NodePools)
			})
		}
	})
}
