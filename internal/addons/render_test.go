package addons

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelmRender downloads real Helm charts and renders them with our value builders.
// This validates the contract: our values produce valid YAML from the actual chart templates.
// These tests require network access; skip with -short.
func TestHelmRender(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping helm render tests in short mode (requires network)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tests := []struct {
		name      string
		chartName string
		values    helm.Values
		namespace string
		// Strings that must appear in rendered output
		mustContain []string
		// Strings that must NOT appear (indicates template error)
		mustNotContain []string
	}{
		{
			name:      "Traefik",
			chartName: "traefik",
			values: buildTraefikValues(&config.Config{
				ClusterName: "test-cluster",
				Addons: config.AddonsConfig{
					Traefik: config.TraefikConfig{
						Enabled:               true,
						Kind:                  "Deployment",
						ExternalTrafficPolicy: "Cluster",
						IngressClass:          "traefik",
					},
				},
			}),
			namespace:   "traefik",
			mustContain: []string{"kind: Deployment", "kind: Service", "kind: IngressClass"},
		},
		{
			name:      "CertManager",
			chartName: "cert-manager",
			values: buildCertManagerValues(&config.Config{
				Addons: config.AddonsConfig{
					CertManager: config.CertManagerConfig{Enabled: true},
				},
			}),
			namespace:   "cert-manager",
			mustContain: []string{"kind: Deployment", "kind: ServiceAccount", "cert-manager"},
		},
		{
			name:      "MetricsServer",
			chartName: "metrics-server",
			values: buildMetricsServerValues(&config.Config{
				Addons: config.AddonsConfig{
					MetricsServer: config.MetricsServerConfig{Enabled: true},
				},
			}),
			namespace:   "kube-system",
			mustContain: []string{"kind: Deployment", "metrics-server"},
		},
		{
			name:      "CCM",
			chartName: "hcloud-ccm",
			values: buildCCMValues(&config.Config{
				ClusterName: "test-cluster",
				Addons: config.AddonsConfig{
					CCM: config.CCMConfig{
						Enabled: true,
						LoadBalancers: config.CCMLoadBalancerConfig{
							Location: "fsn1",
						},
					},
				},
			}),
			namespace:   "kube-system",
			mustContain: []string{"kind: Deployment", "hcloud"},
		},
		{
			name:      "ExternalDNS",
			chartName: "external-dns",
			values: buildExternalDNSValues(&config.Config{
				ClusterName: "test-cluster",
				Addons: config.AddonsConfig{
					ExternalDNS: config.ExternalDNSConfig{
						Enabled: true,
					},
				},
			}),
			namespace:   "external-dns",
			mustContain: []string{"kind: Deployment", "external-dns"},
		},
		{
			name:      "Cilium",
			chartName: "cilium",
			values: buildCiliumValues(&config.Config{
				ClusterName: "test-cluster",
				Addons: config.AddonsConfig{
					Cilium: config.CiliumConfig{
						Enabled:                     true,
						KubeProxyReplacementEnabled: true,
						RoutingMode:                 "tunnel",
					},
				},
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{{Count: 1}},
				},
			}),
			namespace:   "kube-system",
			mustContain: []string{"kind: DaemonSet", "cilium"},
		},
		{
			name:      "ArgoCD",
			chartName: "argo-cd",
			values: buildArgoCDValues(&config.Config{
				ClusterName: "test-cluster",
				Addons: config.AddonsConfig{
					ArgoCD: config.ArgoCDConfig{
						Enabled: true,
					},
				},
			}),
			namespace:   "argocd",
			mustContain: []string{"kind: Deployment", "argocd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := helm.GetChartSpec(tt.chartName, config.HelmChartConfig{})
			require.NotEmpty(t, spec.Repository, "no chart spec for %s", tt.chartName)

			rendered, err := helm.RenderFromSpec(ctx, spec, tt.namespace, tt.values)
			require.NoError(t, err, "failed to render %s chart", tt.chartName)
			require.NotEmpty(t, rendered, "rendered output is empty for %s", tt.chartName)

			output := string(rendered)

			for _, s := range tt.mustContain {
				assert.Contains(t, output, s, "%s output missing expected string: %s", tt.chartName, s)
			}
			for _, s := range tt.mustNotContain {
				assert.NotContains(t, output, s, "%s output contains unexpected string: %s", tt.chartName, s)
			}

			// Validate all documents are valid YAML (no template errors like <no value>)
			assert.NotContains(t, output, "<no value>", "%s has unresolved template values", tt.chartName)

			// Count rendered documents
			docs := strings.Count(output, "kind:")
			t.Logf("%s: rendered %d resources (%d bytes)", tt.chartName, docs, len(rendered))
		})
	}
}

// TestHelmRenderKubePrometheusStack is separate because it's a large chart
// and takes longer to download.
func TestHelmRenderKubePrometheusStack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping helm render tests in short mode (requires network)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	values := buildKubePrometheusStackValues(&config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Enabled: true,
			},
		},
	})

	spec := helm.GetChartSpec("kube-prometheus-stack", config.HelmChartConfig{})
	require.NotEmpty(t, spec.Repository)

	rendered, err := helm.RenderFromSpec(ctx, spec, "monitoring", values)
	require.NoError(t, err)
	require.NotEmpty(t, rendered)

	output := string(rendered)
	assert.Contains(t, output, "kind: Deployment")
	assert.Contains(t, output, "prometheus")
	assert.NotContains(t, output, "<no value>")

	docs := strings.Count(output, "kind:")
	t.Logf("kube-prometheus-stack: rendered %d resources (%d bytes)", docs, len(rendered))
}
