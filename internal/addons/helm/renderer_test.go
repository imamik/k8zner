package helm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"

	"github.com/imamik/k8zner/internal/config"
)

// TestGetChartSpec verifies GetChartSpec returns correct defaults.
func TestGetChartSpec(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
	cachePath := GetCachePath()

	if cachePath == "" {
		t.Error("GetCachePath returned empty string")
	}

	// Path should contain k8zner/charts
	if !strings.Contains(cachePath, "k8zner") || !strings.Contains(cachePath, "charts") {
		t.Errorf("GetCachePath = %q, should contain 'k8zner' and 'charts'", cachePath)
	}
}

// TestGetCachePath_WithXDGEnv tests cache path with XDG_CACHE_HOME set.
func TestGetCachePath_WithXDGEnv(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/test-cache")

	cachePath := GetCachePath()
	assert.Equal(t, "/tmp/test-cache/k8zner/charts", cachePath)
}

// TestClearMemoryCache verifies memory cache can be cleared.
func TestClearMemoryCache(t *testing.T) {
	t.Parallel(
	// This should not panic
	)

	ClearMemoryCache()
}

// Renderer tests

func TestNewRenderer(t *testing.T) {
	t.Parallel()
	r := NewRenderer("my-chart", "my-namespace")

	require.NotNil(t, r)
	assert.Equal(t, "my-chart", r.chartName)
	assert.Equal(t, "my-namespace", r.namespace)
}

func TestNewRenderer_EmptyValues(t *testing.T) {
	t.Parallel()
	r := NewRenderer("", "")

	require.NotNil(t, r)
	assert.Equal(t, "", r.chartName)
	assert.Equal(t, "", r.namespace)
}

func TestRenderChart_MinimalChart(t *testing.T) {
	t.Parallel()
	r := NewRenderer("test-chart", "test-namespace")

	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "1.0.0",
		},
		Templates: []*chart.File{
			{
				Name: "templates/configmap.yaml",
				Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-config
  namespace: {{ .Release.Namespace }}
data:
  replicas: "{{ .Values.replicas }}"
`),
			},
		},
	}

	values := Values{"replicas": 3}

	result, err := r.renderChart(ch, values)
	require.NoError(t, err)

	output := string(result)
	assert.Contains(t, output, "kind: ConfigMap")
	assert.Contains(t, output, "name: test-chart-config")
	assert.Contains(t, output, "namespace: test-namespace")
	assert.Contains(t, output, `replicas: "3"`)
}

func TestRenderChart_WithChartDefaults(t *testing.T) {
	t.Parallel()
	r := NewRenderer("test-chart", "default")

	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "1.0.0",
		},
		Values: map[string]interface{}{
			"replicas":        1,
			"imagePullPolicy": "IfNotPresent",
		},
		Templates: []*chart.File{
			{
				Name: "templates/deployment.yaml",
				Data: []byte(`replicas: {{ .Values.replicas }}
imagePullPolicy: {{ .Values.imagePullPolicy }}
`),
			},
		},
	}

	// Only override replicas, imagePullPolicy should use chart default
	values := Values{"replicas": 5}

	result, err := r.renderChart(ch, values)
	require.NoError(t, err)

	output := string(result)
	assert.Contains(t, output, "replicas: 5")
	assert.Contains(t, output, "imagePullPolicy: IfNotPresent")
}

func TestRenderChart_SkipsNotesFile(t *testing.T) {
	t.Parallel()
	r := NewRenderer("test-chart", "default")

	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "1.0.0",
		},
		Templates: []*chart.File{
			{
				Name: "templates/configmap.yaml",
				Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test
`),
			},
			{
				Name: "templates/NOTES.txt",
				Data: []byte("Thank you for installing test-chart!"),
			},
		},
	}

	result, err := r.renderChart(ch, Values{})
	require.NoError(t, err)

	output := string(result)
	assert.Contains(t, output, "kind: ConfigMap")
	assert.NotContains(t, output, "Thank you for installing")
}

func TestRenderChart_SkipsEmptyTemplates(t *testing.T) {
	t.Parallel()
	r := NewRenderer("test-chart", "default")

	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "1.0.0",
		},
		Templates: []*chart.File{
			{
				Name: "templates/configmap.yaml",
				Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test
`),
			},
			{
				Name: "templates/empty.yaml",
				Data: []byte("   \n\n   "),
			},
			{
				Name: "templates/conditional.yaml",
				Data: []byte(`{{ if .Values.enabled }}apiVersion: v1
kind: Secret
{{ end }}`),
			},
		},
	}

	result, err := r.renderChart(ch, Values{"enabled": false})
	require.NoError(t, err)

	output := string(result)
	assert.Contains(t, output, "kind: ConfigMap")
	assert.NotContains(t, output, "kind: Secret")
}

func TestRenderChart_MultipleDocuments(t *testing.T) {
	t.Parallel()
	r := NewRenderer("test-chart", "default")

	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "1.0.0",
		},
		Templates: []*chart.File{
			{
				Name: "templates/configmap.yaml",
				Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: config1
`),
			},
			{
				Name: "templates/secret.yaml",
				Data: []byte(`apiVersion: v1
kind: Secret
metadata:
  name: secret1
`),
			},
		},
	}

	result, err := r.renderChart(ch, Values{})
	require.NoError(t, err)

	output := string(result)
	assert.Contains(t, output, "kind: ConfigMap")
	assert.Contains(t, output, "kind: Secret")
	assert.Contains(t, output, "---")
}

func TestRenderChart_DeepMergesValues(t *testing.T) {
	t.Parallel()
	r := NewRenderer("test-chart", "default")

	ch := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "1.0.0",
		},
		Values: map[string]interface{}{
			"controller": map[string]interface{}{
				"replicas": 1,
				"image": map[string]interface{}{
					"repository": "default-repo",
					"tag":        "v1.0.0",
				},
			},
		},
		Templates: []*chart.File{
			{
				Name: "templates/deployment.yaml",
				Data: []byte(`replicas: {{ .Values.controller.replicas }}
image: {{ .Values.controller.image.repository }}:{{ .Values.controller.image.tag }}
`),
			},
		},
	}

	// Override only tag, repository should use default
	values := Values{
		"controller": Values{
			"image": Values{
				"tag": "v2.0.0",
			},
		},
	}

	result, err := r.renderChart(ch, values)
	require.NoError(t, err)

	output := string(result)
	assert.Contains(t, output, "replicas: 1")
	assert.Contains(t, output, "image: default-repo:v2.0.0")
}
