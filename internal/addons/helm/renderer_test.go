package helm

import (
	"testing"

	"github.com/imamik/k8zner/internal/config"
)

// TestGetChartSpec verifies GetChartSpec returns correct defaults.
func TestGetChartSpec(t *testing.T) {
	tests := []struct {
		name     string
		addon    string
		helmCfg  config.HelmChartConfig
		wantRepo string
		wantName string
		wantVer  string
	}{
		{
			name:     "hcloud-ccm defaults",
			addon:    "hcloud-ccm",
			helmCfg:  config.HelmChartConfig{},
			wantRepo: "https://charts.hetzner.cloud",
			wantName: "hcloud-cloud-controller-manager",
			wantVer:  "1.29.0",
		},
		{
			name:     "cilium defaults",
			addon:    "cilium",
			helmCfg:  config.HelmChartConfig{},
			wantRepo: "https://helm.cilium.io",
			wantName: "cilium",
			wantVer:  "1.18.5",
		},
		{
			name:     "traefik defaults",
			addon:    "traefik",
			helmCfg:  config.HelmChartConfig{},
			wantRepo: "https://traefik.github.io/charts",
			wantName: "traefik",
			wantVer:  "39.0.0",
		},
		{
			name:  "version override",
			addon: "cilium",
			helmCfg: config.HelmChartConfig{
				Version: "1.16.0",
			},
			wantRepo: "https://helm.cilium.io",
			wantName: "cilium",
			wantVer:  "1.16.0",
		},
		{
			name:  "repository override",
			addon: "cilium",
			helmCfg: config.HelmChartConfig{
				Repository: "https://my-custom-repo.example.com",
			},
			wantRepo: "https://my-custom-repo.example.com",
			wantName: "cilium",
			wantVer:  "1.18.5",
		},
		{
			name:  "chart name override",
			addon: "cilium",
			helmCfg: config.HelmChartConfig{
				Chart: "custom-cilium",
			},
			wantRepo: "https://helm.cilium.io",
			wantName: "custom-cilium",
			wantVer:  "1.18.5",
		},
		{
			name:  "all overrides",
			addon: "cilium",
			helmCfg: config.HelmChartConfig{
				Repository: "https://custom.example.com",
				Chart:      "my-cilium",
				Version:    "2.0.0",
			},
			wantRepo: "https://custom.example.com",
			wantName: "my-cilium",
			wantVer:  "2.0.0",
		},
		{
			name:     "unknown addon returns empty",
			addon:    "unknown-addon",
			helmCfg:  config.HelmChartConfig{},
			wantRepo: "",
			wantName: "",
			wantVer:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := GetChartSpec(tt.addon, tt.helmCfg)

			if spec.Repository != tt.wantRepo {
				t.Errorf("Repository = %q, want %q", spec.Repository, tt.wantRepo)
			}
			if spec.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", spec.Name, tt.wantName)
			}
			if spec.Version != tt.wantVer {
				t.Errorf("Version = %q, want %q", spec.Version, tt.wantVer)
			}
		})
	}
}

// TestDefaultChartSpecsComplete verifies all expected addons have specs defined.
func TestDefaultChartSpecsComplete(t *testing.T) {
	expectedAddons := []string{
		"hcloud-ccm",
		"hcloud-csi",
		"cilium",
		"cert-manager",
		"ingress-nginx",
		"traefik",
		"metrics-server",
		"cluster-autoscaler",
		"longhorn",
	}

	for _, addon := range expectedAddons {
		t.Run(addon, func(t *testing.T) {
			spec, ok := DefaultChartSpecs[addon]
			if !ok {
				t.Fatalf("DefaultChartSpecs missing entry for %s", addon)
			}
			if spec.Repository == "" {
				t.Errorf("Repository is empty for %s", addon)
			}
			if spec.Name == "" {
				t.Errorf("Name is empty for %s", addon)
			}
			if spec.Version == "" {
				t.Errorf("Version is empty for %s", addon)
			}
		})
	}
}

// TestGetCachePath verifies cache path is returned correctly.
func TestGetCachePath(t *testing.T) {
	cachePath := GetCachePath()

	if cachePath == "" {
		t.Error("GetCachePath returned empty string")
	}

	// Path should contain k8zner/charts
	if !contains(cachePath, "k8zner") || !contains(cachePath, "charts") {
		t.Errorf("GetCachePath = %q, should contain 'k8zner' and 'charts'", cachePath)
	}
}

// TestClearMemoryCache verifies memory cache can be cleared.
func TestClearMemoryCache(t *testing.T) {
	// This should not panic
	ClearMemoryCache()
}

// contains checks if substr is in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
