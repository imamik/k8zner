package addons

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/config"
)

// ============================================================================
// apply.go coverage improvements
// ============================================================================

func TestApplyCilium_EmptyKubeconfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{Enabled: true},
		},
	}
	err := ApplyCilium(context.Background(), cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig is required")
}

func TestApplyCilium_EmptyByteSliceKubeconfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{Enabled: true},
		},
	}
	err := ApplyCilium(context.Background(), cfg, []byte{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig is required")
}

func TestApplyWithoutCilium_EmptyKubeconfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{Enabled: true},
		},
	}
	err := ApplyWithoutCilium(context.Background(), cfg, nil, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig is required")
}

func TestApplyWithoutCilium_EmptyByteSliceKubeconfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{Enabled: true},
		},
	}
	err := ApplyWithoutCilium(context.Background(), cfg, []byte{}, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig is required")
}

func TestHasEnabledAddons_AllAddons(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cfg      *config.Config
		expected bool
	}{
		{
			name:     "gateway api crds enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{GatewayAPICRDs: config.GatewayAPICRDsConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "prometheus operator crds enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{PrometheusOperatorCRDs: config.PrometheusOperatorCRDsConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "talos ccm enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{TalosCCM: config.TalosCCMConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "ccm enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{CCM: config.CCMConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "csi enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{CSI: config.CSIConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "metrics server enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{MetricsServer: config.MetricsServerConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "cert manager enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{CertManager: config.CertManagerConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "argocd enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{ArgoCD: config.ArgoCDConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "cloudflare enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{Cloudflare: config.CloudflareConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "external dns enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{ExternalDNS: config.ExternalDNSConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "talos backup enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{TalosBackup: config.TalosBackupConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "kube prometheus stack enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{KubePrometheusStack: config.KubePrometheusStackConfig{Enabled: true}}},
			expected: true,
		},
		{
			name:     "operator enabled",
			cfg:      &config.Config{Addons: config.AddonsConfig{Operator: config.OperatorConfig{Enabled: true}}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, hasEnabledAddons(tt.cfg))
		})
	}
}

func TestGetWorkerCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		workers  []config.WorkerNodePool
		expected int
	}{
		{
			name:     "no workers",
			workers:  []config.WorkerNodePool{},
			expected: 0,
		},
		{
			name:     "single pool",
			workers:  []config.WorkerNodePool{{Count: 3}},
			expected: 3,
		},
		{
			name: "multiple pools",
			workers: []config.WorkerNodePool{
				{Count: 2},
				{Count: 3},
			},
			expected: 5,
		},
		{
			name:     "nil workers",
			workers:  nil,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{Workers: tt.workers}
			assert.Equal(t, tt.expected, cfg.WorkerCount())
		})
	}
}

func TestGetControlPlaneCount_MultiplePools(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp-1", Count: 1},
				{Name: "cp-2", Count: 2},
			},
		},
	}
	assert.Equal(t, 3, getControlPlaneCount(cfg))
}

func TestGetControlPlaneCount_Empty(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: nil,
		},
	}
	assert.Equal(t, 0, getControlPlaneCount(cfg))
}

// ============================================================================
// steps.go coverage improvements
// ============================================================================

func TestInstallStep_UnknownStepMessage(t *testing.T) {
	t.Parallel()
	// Use invalid kubeconfig to trigger kubeconfig error before the switch
	err := InstallStep(t.Context(), "nonexistent-addon", &config.Config{}, nil, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create kubernetes client")
}

func TestAddonStepStruct(t *testing.T) {
	t.Parallel()
	step := AddonStep{Name: "test-addon", Order: 5}
	assert.Equal(t, "test-addon", step.Name)
	assert.Equal(t, 5, step.Order)
}

func TestEnabledSteps_EmptyConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Addons: config.AddonsConfig{}}
	steps := EnabledSteps(cfg)
	assert.Empty(t, steps)
}

func TestEnabledSteps_SingleAddon(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			MetricsServer: config.MetricsServerConfig{Enabled: true},
		},
	}
	steps := EnabledSteps(cfg)
	require.Len(t, steps, 1)
	assert.Equal(t, StepMetricsServer, steps[0].Name)
	assert.Equal(t, 4, steps[0].Order)
}

// ============================================================================
// traefik.go coverage improvements
// ============================================================================

func TestBuildTraefikValues_NoWorkers(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Workers:     []config.WorkerNodePool{},
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{Enabled: true},
		},
	}

	values := buildTraefikValues(cfg)

	// With 0 workers, replicas default to 2
	deployment := values["deployment"].(helm.Values)
	assert.Equal(t, 2, deployment["replicas"])

	// With 0 workers (<= 1), topology spread should use ScheduleAnyway
	tsc := values["topologySpreadConstraints"].([]helm.Values)
	require.Len(t, tsc, 2)
	assert.Equal(t, "ScheduleAnyway", tsc[0]["whenUnsatisfiable"])
}

func TestBuildTraefikValues_LocationDefault(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "", // empty - should default to nbg1
		Workers:     []config.WorkerNodePool{{Count: 2}},
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{Enabled: true},
		},
	}

	values := buildTraefikValues(cfg)

	service := values["service"].(helm.Values)
	annotations := service["annotations"].(helm.Values)
	assert.Equal(t, "nbg1", annotations["load-balancer.hetzner.cloud/location"])
}

func TestBuildTraefikValues_CustomLocation(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Location:    "hel1",
		Workers:     []config.WorkerNodePool{{Count: 2}},
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{Enabled: true},
		},
	}

	values := buildTraefikValues(cfg)

	service := values["service"].(helm.Values)
	annotations := service["annotations"].(helm.Values)
	assert.Equal(t, "hel1", annotations["load-balancer.hetzner.cloud/location"])
}

func TestBuildTraefikValues_CustomHelmValues(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Workers:     []config.WorkerNodePool{{Count: 2}},
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{
				Enabled: true,
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						"customKey": "customValue",
					},
				},
			},
		},
	}

	values := buildTraefikValues(cfg)
	assert.Equal(t, "customValue", values["customKey"])
	// Ensure base values still exist
	_, hasDeployment := values["deployment"]
	assert.True(t, hasDeployment)
}

func TestBuildTraefikDeployment(t *testing.T) {
	t.Parallel()
	deployment := buildTraefikDeployment(3, "DaemonSet")
	assert.Equal(t, true, deployment["enabled"])
	assert.Equal(t, "DaemonSet", deployment["kind"])
	assert.Equal(t, 3, deployment["replicas"])

	pdb := deployment["podDisruptionBudget"].(helm.Values)
	assert.Equal(t, true, pdb["enabled"])
	assert.Equal(t, 1, pdb["maxUnavailable"])
}

func TestBuildTraefikIngressClass(t *testing.T) {
	t.Parallel()
	ic := buildTraefikIngressClass("my-traefik")
	assert.Equal(t, true, ic["enabled"])
	assert.Equal(t, true, ic["isDefaultClass"])
	assert.Equal(t, "my-traefik", ic["name"])
}

func TestBuildTraefikIngressRoute(t *testing.T) {
	t.Parallel()
	ir := buildTraefikIngressRoute()
	dashboard := ir["dashboard"].(helm.Values)
	assert.Equal(t, false, dashboard["enabled"])
}

func TestBuildTraefikProviders(t *testing.T) {
	t.Parallel()
	providers := buildTraefikProviders()

	crd := providers["kubernetesCRD"].(helm.Values)
	assert.Equal(t, false, crd["enabled"])

	ingress := providers["kubernetesIngress"].(helm.Values)
	assert.Equal(t, true, ingress["enabled"])
	assert.Equal(t, true, ingress["allowExternalNameServices"])

	publishedService := ingress["publishedService"].(helm.Values)
	assert.Equal(t, true, publishedService["enabled"])
}

func TestBuildTraefikPorts(t *testing.T) {
	t.Parallel()
	ports := buildTraefikPorts()

	web := ports["web"].(helm.Values)
	assert.Equal(t, 8000, web["port"])
	assert.Equal(t, 80, web["exposedPort"])
	assert.Equal(t, "TCP", web["protocol"])

	websecure := ports["websecure"].(helm.Values)
	assert.Equal(t, 8443, websecure["port"])
	assert.Equal(t, 443, websecure["exposedPort"])
	assert.Equal(t, "TCP", websecure["protocol"])

	// Check TLS is enabled on websecure
	httpCfg := websecure["http"].(helm.Values)
	tlsCfg := httpCfg["tls"].(helm.Values)
	assert.Equal(t, true, tlsCfg["enabled"])

	// Check traefik internal port
	traefikPort := ports["traefik"].(helm.Values)
	assert.Equal(t, 9000, traefikPort["port"])
}

func TestBuildTraefikService(t *testing.T) {
	t.Parallel()
	service := buildTraefikService("my-cluster", "Local", "hel1")

	assert.Equal(t, true, service["enabled"])
	assert.Equal(t, "LoadBalancer", service["type"])

	spec := service["spec"].(helm.Values)
	assert.Equal(t, "Local", spec["externalTrafficPolicy"])

	annotations := service["annotations"].(helm.Values)
	assert.Equal(t, "my-cluster-ingress", annotations["load-balancer.hetzner.cloud/name"])
	assert.Equal(t, "true", annotations["load-balancer.hetzner.cloud/use-private-ip"])
	assert.Equal(t, "true", annotations["load-balancer.hetzner.cloud/disable-private-ingress"])
	assert.Equal(t, "hel1", annotations["load-balancer.hetzner.cloud/location"])
}

// ============================================================================
// kube_prometheus_stack.go coverage improvements
// ============================================================================

func TestBuildGrafanaValues_DefaultEnabled(t *testing.T) {
	t.Parallel()
	// Test Grafana with nil Enabled pointer (should default to true)
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Grafana: config.KubePrometheusGrafanaConfig{
					// Enabled is nil - should default to true
				},
			},
		},
	}

	values := buildGrafanaValues(cfg)
	assert.Equal(t, true, values["enabled"])
}

func TestBuildGrafanaValues_Disabled(t *testing.T) {
	t.Parallel()
	disabled := false
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Grafana: config.KubePrometheusGrafanaConfig{
					Enabled: &disabled,
				},
			},
		},
	}

	values := buildGrafanaValues(cfg)
	assert.Equal(t, false, values["enabled"])
}

func TestBuildGrafanaValues_NoPersistence(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Grafana: config.KubePrometheusGrafanaConfig{
					Persistence: config.KubePrometheusPersistenceConfig{
						Enabled: false,
					},
				},
			},
		},
	}

	values := buildGrafanaValues(cfg)
	_, hasPersistence := values["persistence"]
	assert.False(t, hasPersistence)
}

func TestBuildGrafanaValues_NoAdminPassword(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Grafana: config.KubePrometheusGrafanaConfig{
					AdminPassword: "", // No password set
				},
			},
		},
	}

	values := buildGrafanaValues(cfg)
	_, hasPassword := values["adminPassword"]
	assert.False(t, hasPassword)
}

func TestBuildGrafanaValues_NoIngress(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Grafana: config.KubePrometheusGrafanaConfig{
					IngressEnabled: false,
					IngressHost:    "grafana.example.com",
				},
			},
		},
	}

	values := buildGrafanaValues(cfg)
	_, hasIngress := values["ingress"]
	assert.False(t, hasIngress, "ingress should not be set when IngressEnabled is false")
}

func TestBuildGrafanaValues_IngressEnabledEmptyHost(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Grafana: config.KubePrometheusGrafanaConfig{
					IngressEnabled: true,
					IngressHost:    "", // empty host
				},
			},
		},
	}

	values := buildGrafanaValues(cfg)
	_, hasIngress := values["ingress"]
	assert.False(t, hasIngress, "ingress should not be set when IngressHost is empty")
}

func TestBuildGrafanaIngress_NoTLS(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Grafana: config.KubePrometheusGrafanaConfig{
					IngressEnabled:   true,
					IngressHost:      "grafana.example.com",
					IngressClassName: "nginx",
					IngressTLS:       false,
				},
			},
		},
	}

	ingress := buildGrafanaIngress(cfg)
	assert.Equal(t, true, ingress["enabled"])
	assert.Equal(t, "nginx", ingress["ingressClassName"])
	_, hasTLS := ingress["tls"]
	assert.False(t, hasTLS)
	_, hasAnnotations := ingress["annotations"]
	assert.False(t, hasAnnotations)
}

func TestBuildGrafanaIngress_Path(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Grafana: config.KubePrometheusGrafanaConfig{
					IngressEnabled: true,
					IngressHost:    "grafana.example.com",
				},
			},
		},
	}

	ingress := buildGrafanaIngress(cfg)
	assert.Equal(t, "/", ingress["path"])
}

func TestBuildPrometheusValues_DefaultRetention(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Prometheus: config.KubePrometheusPrometheusConfig{
					// No RetentionDays set - should default to 15
				},
			},
		},
	}

	values := buildPrometheusValues(cfg)
	promSpec := values["prometheusSpec"].(helm.Values)
	assert.Equal(t, "15d", promSpec["retention"])
}

func TestBuildPrometheusValues_CustomRetention(t *testing.T) {
	t.Parallel()
	retentionDays := 30
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Prometheus: config.KubePrometheusPrometheusConfig{
					RetentionDays: &retentionDays,
				},
			},
		},
	}

	values := buildPrometheusValues(cfg)
	promSpec := values["prometheusSpec"].(helm.Values)
	assert.Equal(t, "30d", promSpec["retention"])
}

func TestBuildPrometheusValues_NoPersistence(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Prometheus: config.KubePrometheusPrometheusConfig{
					Persistence: config.KubePrometheusPersistenceConfig{
						Enabled: false,
					},
				},
			},
		},
	}

	values := buildPrometheusValues(cfg)
	promSpec := values["prometheusSpec"].(helm.Values)
	_, hasStorageSpec := promSpec["storageSpec"]
	assert.False(t, hasStorageSpec, "storageSpec should not be set when persistence is disabled")
}

func TestBuildPrometheusValues_PersistenceDefaults(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Prometheus: config.KubePrometheusPrometheusConfig{
					Persistence: config.KubePrometheusPersistenceConfig{
						Enabled: true,
						// No Size or StorageClass specified
					},
				},
			},
		},
	}

	values := buildPrometheusValues(cfg)
	promSpec := values["prometheusSpec"].(helm.Values)
	storageSpec := promSpec["storageSpec"].(helm.Values)
	vct := storageSpec["volumeClaimTemplate"].(helm.Values)
	spec := vct["spec"].(helm.Values)
	resources := spec["resources"].(helm.Values)
	requests := resources["requests"].(helm.Values)
	assert.Equal(t, "50Gi", requests["storage"])
}

func TestBuildPrometheusValues_NoIngress(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Prometheus: config.KubePrometheusPrometheusConfig{
					IngressEnabled: false,
				},
			},
		},
	}

	values := buildPrometheusValues(cfg)
	_, hasIngress := values["ingress"]
	assert.False(t, hasIngress, "ingress should not be set when disabled")
}

func TestBuildPrometheusValues_IngressEnabledEmptyHost(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Prometheus: config.KubePrometheusPrometheusConfig{
					IngressEnabled: true,
					IngressHost:    "",
				},
			},
		},
	}

	values := buildPrometheusValues(cfg)
	_, hasIngress := values["ingress"]
	assert.False(t, hasIngress, "ingress should not be set when host is empty")
}

func TestBuildPrometheusValues_Disabled(t *testing.T) {
	t.Parallel()
	disabled := false
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Prometheus: config.KubePrometheusPrometheusConfig{
					Enabled: &disabled,
				},
			},
		},
	}

	values := buildPrometheusValues(cfg)
	assert.Equal(t, false, values["enabled"])
}

func TestBuildPrometheusValues_CustomResources(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Prometheus: config.KubePrometheusPrometheusConfig{
					Resources: config.KubePrometheusResourcesConfig{
						Requests: config.KubePrometheusResourceSpec{
							CPU:    "1",
							Memory: "1Gi",
						},
						Limits: config.KubePrometheusResourceSpec{
							CPU:    "4",
							Memory: "4Gi",
						},
					},
				},
			},
		},
	}

	values := buildPrometheusValues(cfg)
	promSpec := values["prometheusSpec"].(helm.Values)
	resources := promSpec["resources"].(helm.Values)
	requests := resources["requests"].(helm.Values)
	assert.Equal(t, "1", requests["cpu"])
	assert.Equal(t, "1Gi", requests["memory"])
	limits := resources["limits"].(helm.Values)
	assert.Equal(t, "4", limits["cpu"])
	assert.Equal(t, "4Gi", limits["memory"])
}

func TestBuildAlertmanagerValues_DefaultEnabled(t *testing.T) {
	t.Parallel()
	// Test with nil Enabled pointer - should default to true
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Alertmanager: config.KubePrometheusAlertmanagerConfig{
					// Enabled is nil - defaults to true
				},
			},
		},
	}

	values := buildAlertmanagerValues(cfg)
	assert.Equal(t, true, values["enabled"])
}

func TestBuildAlertmanagerValues_NoIngress(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Alertmanager: config.KubePrometheusAlertmanagerConfig{
					IngressEnabled: false,
				},
			},
		},
	}

	values := buildAlertmanagerValues(cfg)
	_, hasIngress := values["ingress"]
	assert.False(t, hasIngress, "ingress should not be set when disabled")
}

func TestBuildAlertmanagerValues_IngressEnabledEmptyHost(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Alertmanager: config.KubePrometheusAlertmanagerConfig{
					IngressEnabled: true,
					IngressHost:    "", // empty host
				},
			},
		},
	}

	values := buildAlertmanagerValues(cfg)
	_, hasIngress := values["ingress"]
	assert.False(t, hasIngress, "ingress should not be set when host is empty")
}

func TestBuildAlertmanagerIngress_DefaultClassName(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Alertmanager: config.KubePrometheusAlertmanagerConfig{
					IngressEnabled:   true,
					IngressHost:      "alertmanager.example.com",
					IngressClassName: "", // empty - should default to "traefik"
				},
			},
		},
	}

	ingress := buildAlertmanagerIngress(cfg)
	assert.Equal(t, "traefik", ingress["ingressClassName"])
}

func TestBuildAlertmanagerIngress_NoTLS(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Alertmanager: config.KubePrometheusAlertmanagerConfig{
					IngressEnabled: true,
					IngressHost:    "alertmanager.example.com",
					IngressTLS:     false,
				},
			},
		},
	}

	ingress := buildAlertmanagerIngress(cfg)
	_, hasTLS := ingress["tls"]
	assert.False(t, hasTLS)
	_, hasAnnotations := ingress["annotations"]
	assert.False(t, hasAnnotations)
}

func TestBuildKubePrometheusStackValues_CustomHelmValues(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			KubePrometheusStack: config.KubePrometheusStackConfig{
				Enabled: true,
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						"customKey": "customValue",
					},
				},
			},
		},
	}

	values := buildKubePrometheusStackValues(cfg)
	assert.Equal(t, "customValue", values["customKey"])
	// Base values should still exist
	_, hasDefaultRules := values["defaultRules"]
	assert.True(t, hasDefaultRules)
}

func TestBuildResourceValues_PartialCustom(t *testing.T) {
	t.Parallel()
	// Test with only some custom values set
	resources := config.KubePrometheusResourcesConfig{
		Requests: config.KubePrometheusResourceSpec{
			CPU: "2", // Custom CPU, default memory
		},
		Limits: config.KubePrometheusResourceSpec{
			Memory: "8Gi", // Custom memory, default CPU
		},
	}

	values := buildResourceValues(resources, "100m", "256Mi", "500m", "1Gi")

	requests := values["requests"].(helm.Values)
	assert.Equal(t, "2", requests["cpu"])
	assert.Equal(t, "256Mi", requests["memory"]) // default

	limits := values["limits"].(helm.Values)
	assert.Equal(t, "500m", limits["cpu"]) // default
	assert.Equal(t, "8Gi", limits["memory"])
}

// ============================================================================
// argocd.go coverage improvements
// ============================================================================

func TestBuildArgoCDValues_NonHAControllerReplicas(t *testing.T) {
	t.Parallel()
	// When HA is false, ControllerReplicas should be ignored
	replicas := 5
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				Enabled:            true,
				HA:                 false,
				ControllerReplicas: &replicas,
			},
		},
	}

	values := buildArgoCDValues(cfg)
	controller := values["controller"].(helm.Values)
	assert.Equal(t, 1, controller["replicas"], "non-HA controller should have 1 replica regardless of ControllerReplicas setting")
}

func TestBuildArgoCDValues_GlobalDomain(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				Enabled:     true,
				IngressHost: "argocd.my-domain.io",
			},
		},
	}

	values := buildArgoCDValues(cfg)
	global := values["global"].(helm.Values)
	assert.Equal(t, "argocd.my-domain.io", global["domain"])
}

func TestBuildArgoCDValues_ConfigsParams(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{Enabled: true},
		},
	}

	values := buildArgoCDValues(cfg)
	configs := values["configs"].(helm.Values)
	params := configs["params"].(helm.Values)
	assert.Equal(t, true, params["server.insecure"])
}

func TestBuildArgoCDValues_DexDisabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{Enabled: true},
		},
	}

	values := buildArgoCDValues(cfg)
	dex := values["dex"].(helm.Values)
	assert.Equal(t, false, dex["enabled"])
}

func TestBuildArgoCDValues_ApplicationSetEnabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{Enabled: true},
		},
	}

	values := buildArgoCDValues(cfg)
	appSet := values["applicationSet"].(helm.Values)
	assert.Equal(t, true, appSet["enabled"])
}

func TestBuildArgoCDValues_NotificationsEnabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{Enabled: true},
		},
	}

	values := buildArgoCDValues(cfg)
	notifications := values["notifications"].(helm.Values)
	assert.Equal(t, true, notifications["enabled"])
}

func TestBuildArgoCDIngress_NoClassName(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				IngressEnabled:   true,
				IngressHost:      "argocd.example.com",
				IngressClassName: "", // empty
			},
		},
	}

	ingress := buildArgoCDIngress(cfg)
	_, hasClassName := ingress["ingressClassName"]
	assert.False(t, hasClassName, "ingressClassName should not be set when empty")
}

func TestBuildArgoCDIngress_NoTLS(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				IngressEnabled: true,
				IngressHost:    "argocd.example.com",
				IngressTLS:     false,
			},
		},
	}

	ingress := buildArgoCDIngress(cfg)
	_, hasTLS := ingress["tls"]
	assert.False(t, hasTLS)
	_, hasAnnotations := ingress["annotations"]
	assert.False(t, hasAnnotations)
}

func TestBuildArgoCDServer_HA_CustomReplicas(t *testing.T) {
	t.Parallel()
	replicas := 5
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				HA:             true,
				ServerReplicas: &replicas,
			},
		},
	}

	server := buildArgoCDServer(cfg)
	assert.Equal(t, 5, server["replicas"])
}

func TestBuildArgoCDServer_NoIngress(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				IngressEnabled: false,
			},
		},
	}

	server := buildArgoCDServer(cfg)
	_, hasIngress := server["ingress"]
	assert.False(t, hasIngress)
}

func TestBuildArgoCDServer_IngressEnabledButEmptyHost(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			ArgoCD: config.ArgoCDConfig{
				IngressEnabled: true,
				IngressHost:    "", // empty host
			},
		},
	}

	server := buildArgoCDServer(cfg)
	_, hasIngress := server["ingress"]
	assert.False(t, hasIngress, "ingress should not be set when host is empty")
}

func TestBuildArgoCDRepoServer_NonHA(t *testing.T) {
	t.Parallel()
	cfg := config.ArgoCDConfig{
		HA: false,
	}
	repoServer := buildArgoCDRepoServer(cfg)
	assert.Equal(t, 1, repoServer["replicas"])

	// Verify tolerations
	tolerations := repoServer["tolerations"].([]helm.Values)
	require.Len(t, tolerations, 1)
}

func TestBuildArgoCDRedis_StandaloneResources(t *testing.T) {
	t.Parallel()
	cfg := config.ArgoCDConfig{HA: false}
	redis := buildArgoCDRedis(cfg)

	assert.Equal(t, true, redis["enabled"])
	resources := redis["resources"].(helm.Values)
	requests := resources["requests"].(helm.Values)
	assert.Equal(t, "50m", requests["cpu"])
	assert.Equal(t, "64Mi", requests["memory"])

	limits := resources["limits"].(helm.Values)
	assert.Equal(t, "128Mi", limits["memory"])
}

// ============================================================================
// external_dns.go coverage improvements
// ============================================================================

func TestBuildExternalDNSValues_CustomHelmValues(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "test-cluster",
		Addons: config.AddonsConfig{
			ExternalDNS: config.ExternalDNSConfig{
				Enabled: true,
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						"customKey": "customValue",
					},
				},
			},
			Cloudflare: config.CloudflareConfig{
				Domain: "example.com",
			},
		},
	}

	values := buildExternalDNSValues(cfg)
	assert.Equal(t, "customValue", values["customKey"])
	// Base values should still exist
	_, hasProvider := values["provider"]
	assert.True(t, hasProvider)
}

func TestBuildExternalDNSValues_EmptyOwnerIDFallback(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ClusterName: "", // empty cluster name
		Addons: config.AddonsConfig{
			ExternalDNS: config.ExternalDNSConfig{
				Enabled: true,
			},
			Cloudflare: config.CloudflareConfig{},
		},
	}

	values := buildExternalDNSValues(cfg)
	assert.Equal(t, "", values["txtOwnerId"])
}

// ============================================================================
// metrics_server.go coverage improvements
// ============================================================================

func TestBuildMetricsServerPDB(t *testing.T) {
	t.Parallel()
	pdb := buildMetricsServerPDB()

	assert.Equal(t, true, pdb["enabled"])
	assert.Nil(t, pdb["minAvailable"])
	assert.Equal(t, 1, pdb["maxUnavailable"])
}

func TestBuildMetricsServerValues_CustomHelmValues(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Workers: []config.WorkerNodePool{{Count: 2}},
		Addons: config.AddonsConfig{
			MetricsServer: config.MetricsServerConfig{
				Enabled: true,
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						"customKey": "customValue",
					},
				},
			},
		},
	}

	values := buildMetricsServerValues(cfg)
	assert.Equal(t, "customValue", values["customKey"])
}

func TestBuildMetricsServerValues_WorkerScheduling(t *testing.T) {
	t.Parallel()
	// With workers, should NOT have nodeSelector or tolerations
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Workers: []config.WorkerNodePool{{Count: 2}},
	}

	values := buildMetricsServerValues(cfg)
	assert.Nil(t, values["nodeSelector"])
	assert.Nil(t, values["tolerations"])
	assert.Equal(t, 2, values["replicas"])
}

func TestBuildMetricsServerValues_SingleWorker(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Workers: []config.WorkerNodePool{{Count: 1}},
	}

	values := buildMetricsServerValues(cfg)
	assert.Equal(t, 1, values["replicas"])
}

// ============================================================================
// cilium.go coverage improvements
// ============================================================================

func TestBuildCiliumIPSecSecret_CustomKeySize(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				IPSecKeySize:   256,
				IPSecKeyID:     3,
				IPSecAlgorithm: "cbc(aes)",
			},
		},
	}

	secretYAML, err := buildCiliumIPSecSecret(cfg)
	require.NoError(t, err)
	assert.Contains(t, secretYAML, "\"256\"")
	assert.Contains(t, secretYAML, "\"3\"")
	assert.Contains(t, secretYAML, "cbc(aes)")
}

func TestBuildCiliumValues_TunnelModeNoConntrack(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Network: config.NetworkConfig{IPv4CIDR: "10.0.0.0/8"},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:                     true,
				RoutingMode:                 "tunnel",
				KubeProxyReplacementEnabled: true,
			},
		},
	}

	values := buildCiliumValues(cfg)
	// installNoConntrackIptablesRules should be false in tunnel mode
	assert.Equal(t, false, values["installNoConntrackIptablesRules"])
}

func TestBuildCiliumValues_NoKubeProxyReplacementHealthz(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Network: config.NetworkConfig{IPv4CIDR: "10.0.0.0/8"},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:                     true,
				KubeProxyReplacementEnabled: false,
			},
		},
	}

	values := buildCiliumValues(cfg)
	_, hasHealthz := values["kubeProxyReplacementHealthzBindAddr"]
	assert.False(t, hasHealthz, "healthz should not be set when kube-proxy replacement is disabled")
}

func TestBuildCiliumValues_NoGatewayAPI(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Network: config.NetworkConfig{IPv4CIDR: "10.0.0.0/8"},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:           true,
				GatewayAPIEnabled: false,
			},
		},
	}

	values := buildCiliumValues(cfg)
	_, hasGatewayAPI := values["gatewayAPI"]
	assert.False(t, hasGatewayAPI, "gatewayAPI should not be set when disabled")
}

func TestBuildCiliumValues_NoHubble(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Network: config.NetworkConfig{IPv4CIDR: "10.0.0.0/8"},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:       true,
				HubbleEnabled: false,
			},
		},
	}

	values := buildCiliumValues(cfg)
	_, hasHubble := values["hubble"]
	assert.False(t, hasHubble, "hubble should not be set when disabled")
}

func TestBuildCiliumValues_EgressGateway(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Network: config.NetworkConfig{IPv4CIDR: "10.0.0.0/8"},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:              true,
				EgressGatewayEnabled: true,
			},
		},
	}

	values := buildCiliumValues(cfg)
	egress := values["egressGateway"].(helm.Values)
	assert.Equal(t, true, egress["enabled"])
}

func TestBuildCiliumValues_LoadBalancerAcceleration(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Network: config.NetworkConfig{IPv4CIDR: "10.0.0.0/8"},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{Enabled: true},
		},
	}

	values := buildCiliumValues(cfg)
	lb := values["loadBalancer"].(helm.Values)
	assert.Equal(t, "disabled", lb["acceleration"])
}

func TestBuildCiliumValues_CustomHelmValues(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Network: config.NetworkConfig{IPv4CIDR: "10.0.0.0/8"},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled: true,
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						"debug": map[string]any{
							"enabled": true,
						},
					},
				},
			},
		},
	}

	values := buildCiliumValues(cfg)
	debug := values["debug"].(map[string]any)
	assert.Equal(t, true, debug["enabled"])
}

// ============================================================================
// cert_manager.go coverage improvements
// ============================================================================

func TestBuildCertManagerConfig_TraefikEnabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{Enabled: true},
		},
	}

	cmConfig := buildCertManagerConfig(cfg)
	featureGates := cmConfig["featureGates"].(helm.Values)
	// When Traefik is enabled, ACMEHTTP01IngressPathTypeExact should be false
	assert.Equal(t, false, featureGates["ACMEHTTP01IngressPathTypeExact"])
}

func TestBuildCertManagerConfig_TraefikDisabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Addons: config.AddonsConfig{
			Traefik: config.TraefikConfig{Enabled: false},
		},
	}

	cmConfig := buildCertManagerConfig(cfg)
	featureGates := cmConfig["featureGates"].(helm.Values)
	// When Traefik is disabled, ACMEHTTP01IngressPathTypeExact should be true
	assert.Equal(t, true, featureGates["ACMEHTTP01IngressPathTypeExact"])
}

func TestBuildCertManagerBaseConfig(t *testing.T) {
	t.Parallel()

	t.Run("single replica", func(t *testing.T) {
		t.Parallel()
		config := buildCertManagerBaseConfig(1)
		assert.Equal(t, 1, config["replicaCount"])

		pdb := config["podDisruptionBudget"].(helm.Values)
		assert.Equal(t, true, pdb["enabled"])
		assert.Equal(t, 1, pdb["maxUnavailable"])

		tolerations := config["tolerations"].([]helm.Values)
		require.Len(t, tolerations, 2)
		assert.Equal(t, "node-role.kubernetes.io/control-plane", tolerations[0]["key"])
		assert.Equal(t, "node.cloudprovider.kubernetes.io/uninitialized", tolerations[1]["key"])
	})

	t.Run("HA replicas", func(t *testing.T) {
		t.Parallel()
		config := buildCertManagerBaseConfig(2)
		assert.Equal(t, 2, config["replicaCount"])
	})
}

func TestBuildCertManagerTopologySpread(t *testing.T) {
	t.Parallel()
	tsc := buildCertManagerTopologySpread("webhook")
	require.Len(t, tsc, 1)
	assert.Equal(t, "kubernetes.io/hostname", tsc[0]["topologyKey"])
	assert.Equal(t, 1, tsc[0]["maxSkew"])
	assert.Equal(t, "DoNotSchedule", tsc[0]["whenUnsatisfiable"])

	labelSelector := tsc[0]["labelSelector"].(helm.Values)
	matchLabels := labelSelector["matchLabels"].(helm.Values)
	assert.Equal(t, "cert-manager", matchLabels["app.kubernetes.io/instance"])
	assert.Equal(t, "webhook", matchLabels["app.kubernetes.io/component"])
}

func TestBuildCertManagerValues_SingleCP(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
	}

	values := buildCertManagerValues(cfg)
	assert.Equal(t, 1, values["replicaCount"])

	webhook := values["webhook"].(helm.Values)
	assert.Equal(t, 1, webhook["replicaCount"])

	cainjector := values["cainjector"].(helm.Values)
	assert.Equal(t, 1, cainjector["replicaCount"])
}

func TestBuildCertManagerValues_HACP(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 3}},
		},
	}

	values := buildCertManagerValues(cfg)
	assert.Equal(t, 2, values["replicaCount"])

	webhook := values["webhook"].(helm.Values)
	assert.Equal(t, 2, webhook["replicaCount"])

	cainjector := values["cainjector"].(helm.Values)
	assert.Equal(t, 2, cainjector["replicaCount"])
}

func TestBuildCertManagerValues_CRDsEnabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
	}

	values := buildCertManagerValues(cfg)
	crds := values["crds"].(helm.Values)
	assert.Equal(t, true, crds["enabled"])
}

func TestBuildCertManagerValues_IngressShim(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
	}

	values := buildCertManagerValues(cfg)
	ingressShim := values["ingressShim"].(helm.Values)
	assert.Equal(t, "ClusterIssuer", ingressShim["defaultIssuerKind"])
}

// ============================================================================
// manifest_patch.go coverage improvements
// ============================================================================

func TestPatchManifestObjects_MultipleMatches(t *testing.T) {
	t.Parallel()
	// Test that multiple matching documents are all patched
	yaml := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: deploy-1
spec:
  template:
    spec:
      containers:
      - name: app
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deploy-2
spec:
  template:
    spec:
      containers:
      - name: app
`)

	patchCount := 0
	result, err := patchManifestObjects(yaml, "Deployment",
		func(obj *unstructured.Unstructured) bool {
			return obj.GetKind() == "Deployment"
		},
		func(obj *unstructured.Unstructured) error {
			patchCount++
			labels := obj.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels["patched"] = "true"
			obj.SetLabels(labels)
			return nil
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, patchCount)
	assert.Contains(t, string(result), "deploy-1")
	assert.Contains(t, string(result), "deploy-2")
}

func TestPatchHostNetworkAPIAccess_MultipleContainers(t *testing.T) {
	t.Parallel()
	yaml := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: multi-container
spec:
  template:
    spec:
      containers:
      - name: main
        image: main:latest
        env:
        - name: APP_ENV
          value: production
      - name: sidecar
        image: sidecar:latest
`)
	result, err := patchHostNetworkAPIAccess(yaml, "multi-container")
	require.NoError(t, err)

	resultStr := string(result)
	// Both containers should have the env vars
	assert.Equal(t, 2, strings.Count(resultStr, "KUBERNETES_SERVICE_HOST"))
	assert.Equal(t, 2, strings.Count(resultStr, "KUBERNETES_SERVICE_PORT"))
	// Original env should be preserved
	assert.Contains(t, resultStr, "APP_ENV")
}

// ============================================================================
// operator.go coverage improvements
// ============================================================================

func TestBuildOperatorValues_VersionWithSlashesInConfig(t *testing.T) {
	// Not parallel: uses t.Setenv
	t.Setenv("K8ZNER_OPERATOR_VERSION", "")

	cfg := &config.Config{
		HCloudToken: "test-token",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 3}},
		},
		Addons: config.AddonsConfig{
			Operator: config.OperatorConfig{
				Enabled: true,
				Version: "feature/my-branch",
			},
		},
	}

	values := buildOperatorValues(cfg)
	image := values["image"].(helm.Values)
	assert.Equal(t, "feature-my-branch", image["tag"], "slashes should be replaced with dashes")
}

func TestBuildOperatorValues_NoHostNetwork(t *testing.T) {
	// Not parallel: uses t.Setenv
	t.Setenv("K8ZNER_OPERATOR_VERSION", "")

	cfg := &config.Config{
		HCloudToken: "test-token",
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Addons: config.AddonsConfig{
			Operator: config.OperatorConfig{
				Enabled:     true,
				Version:     "v1.0.0",
				HostNetwork: false,
			},
		},
	}

	values := buildOperatorValues(cfg)
	_, hasHostNetwork := values["hostNetwork"]
	assert.False(t, hasHostNetwork, "hostNetwork should not be set when disabled")
	_, hasDNSPolicy := values["dnsPolicy"]
	assert.False(t, hasDNSPolicy, "dnsPolicy should not be set when hostNetwork is disabled")
}

// ============================================================================
// csi.go coverage improvements
// ============================================================================

func TestBuildCSIValues_CustomHelmValues(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
		Addons: config.AddonsConfig{
			CSI: config.CSIConfig{
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						"customKey": "customValue",
					},
				},
			},
		},
	}

	values := buildCSIValues(cfg)
	assert.Equal(t, "customValue", values["customKey"])
}

func TestBuildCSIValues_HCloudTokenRef(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{{Count: 1}},
		},
	}

	values := buildCSIValues(cfg)
	controller := values["controller"].(helm.Values)
	hcloudToken := controller["hcloudToken"].(helm.Values)
	existingSecret := hcloudToken["existingSecret"].(helm.Values)
	assert.Equal(t, "hcloud", existingSecret["name"])
	assert.Equal(t, "token", existingSecret["key"])
}

func TestGenerateEncryptionKey_DifferentLengths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		length         int
		expectedHexLen int
	}{
		{"16 bytes", 16, 32},
		{"32 bytes", 32, 64},
		{"64 bytes", 64, 128},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			key, err := generateEncryptionKey(tt.length)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedHexLen, len(key))
		})
	}
}

// ============================================================================
// ccm.go coverage improvements
// ============================================================================

func TestBuildCCMValues_KubeServiceOverrides(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Location: "fsn1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{
				Enabled: true,
			},
		},
	}

	values := buildCCMValues(cfg)
	env := values["env"].(helm.Values)

	// Check KUBERNETES_SERVICE_HOST override
	svcHost := env["KUBERNETES_SERVICE_HOST"].(helm.Values)
	assert.Equal(t, "localhost", svcHost["value"])

	// Check KUBERNETES_SERVICE_PORT override
	svcPort := env["KUBERNETES_SERVICE_PORT"].(helm.Values)
	assert.Equal(t, "6443", svcPort["value"])
}

func TestBuildCCMValues_HCloudTokenRef(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Location: "fsn1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{Enabled: true},
		},
	}

	values := buildCCMValues(cfg)
	env := values["env"].(helm.Values)

	hcloudToken := env["HCLOUD_TOKEN"].(helm.Values)
	valueFrom := hcloudToken["valueFrom"].(helm.Values)
	secretKeyRef := valueFrom["secretKeyRef"].(helm.Values)
	assert.Equal(t, "hcloud", secretKeyRef["name"])
	assert.Equal(t, "token", secretKeyRef["key"])
}

func TestBuildCCMValues_MinimalConfig(t *testing.T) {
	t.Parallel()
	// Test with minimal config - no LB options set
	cfg := &config.Config{
		Location: "fsn1",
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{
				Enabled: true,
			},
		},
	}

	values := buildCCMValues(cfg)

	env := values["env"].(helm.Values)

	// Location should still be set (fallback to cluster location)
	location := env["HCLOUD_LOAD_BALANCERS_LOCATION"].(helm.Values)
	assert.Equal(t, "fsn1", location["value"])

	// Optional fields should not be present
	_, hasType := env["HCLOUD_LOAD_BALANCERS_TYPE"]
	assert.False(t, hasType, "type should not be set when not configured")

	_, hasAlgorithm := env["HCLOUD_LOAD_BALANCERS_ALGORITHM_TYPE"]
	assert.False(t, hasAlgorithm, "algorithm should not be set when not configured")

	_, hasEnabled := env["HCLOUD_LOAD_BALANCERS_ENABLED"]
	assert.False(t, hasEnabled, "enabled should not be set when nil")
}

func TestBuildCCMValues_CustomHelmValues(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Location: "fsn1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{
				Enabled: true,
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						"customKey": "customValue",
					},
				},
			},
		},
	}

	values := buildCCMValues(cfg)
	assert.Equal(t, "customValue", values["customKey"])
}

func TestBuildCCMEnvVars_NoHealthCheck(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Location: "fsn1",
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{
				Enabled:       true,
				LoadBalancers: config.CCMLoadBalancerConfig{
					// No health check settings
				},
			},
		},
	}

	env := buildCCMEnvVars(cfg, &cfg.Addons.CCM.LoadBalancers)

	_, hasInterval := env["HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_INTERVAL"]
	assert.False(t, hasInterval)
	_, hasTimeout := env["HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_TIMEOUT"]
	assert.False(t, hasTimeout)
	_, hasRetries := env["HCLOUD_LOAD_BALANCERS_HEALTH_CHECK_RETRIES"]
	assert.False(t, hasRetries)
}

func TestBuildCCMEnvVars_NoNetworkRoutes(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Location: "fsn1",
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{
				Enabled: true,
				// NetworkRoutesEnabled is nil
			},
		},
	}

	env := buildCCMEnvVars(cfg, &cfg.Addons.CCM.LoadBalancers)

	_, hasRoutes := env["HCLOUD_NETWORK_ROUTES_ENABLED"]
	assert.False(t, hasRoutes)
}

// ============================================================================
// Traefik namespace security labels
// ============================================================================

func TestTraefikNamespace_SecurityLabels(t *testing.T) {
	t.Parallel()
	ns := helm.NamespaceManifest("traefik", baselinePodSecurityLabels)

	assert.Contains(t, ns, "pod-security.kubernetes.io/enforce: baseline")
	assert.Contains(t, ns, "pod-security.kubernetes.io/audit: baseline")
	assert.Contains(t, ns, "pod-security.kubernetes.io/warn: baseline")
}
