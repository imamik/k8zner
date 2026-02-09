package provisioning

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

func newTestCluster(name, domain string, addons *k8znerv1alpha1.AddonSpec) *k8znerv1alpha1.K8znerCluster {
	return &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region: "fsn1",
			Domain: domain,
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{
				Count: 1, Size: "cx23",
			},
			Workers: k8znerv1alpha1.WorkerSpec{
				Count: 1, Size: "cx23",
			},
			Kubernetes:     k8znerv1alpha1.KubernetesSpec{Version: "1.32.2"},
			Talos:          k8znerv1alpha1.TalosSpec{Version: "v1.10.2"},
			Addons:         addons,
			CredentialsRef: corev1.LocalObjectReference{Name: "creds"},
		},
	}
}

func TestSpecToConfig_DomainIngress(t *testing.T) {
	t.Parallel()
	baseCreds := &Credentials{
		HCloudToken:        "test-token",
		CloudflareAPIToken: "cf-token",
	}

	t.Run("domain with ArgoCD enables ingress", func(t *testing.T) {
		t.Parallel()
		cluster := newTestCluster("test-cluster", "example.com", &k8znerv1alpha1.AddonSpec{
			ArgoCD:      true,
			ExternalDNS: true,
			CertManager: true,
			Traefik:     true,
		})

		cfg, err := SpecToConfig(cluster, baseCreds)
		require.NoError(t, err)

		assert.True(t, cfg.Addons.ArgoCD.Enabled)
		assert.True(t, cfg.Addons.ArgoCD.IngressEnabled)
		assert.Equal(t, "argo.example.com", cfg.Addons.ArgoCD.IngressHost)
		assert.Equal(t, "traefik", cfg.Addons.ArgoCD.IngressClassName)
		assert.True(t, cfg.Addons.ArgoCD.IngressTLS)
	})

	t.Run("domain with Monitoring enables Grafana ingress", func(t *testing.T) {
		t.Parallel()
		cluster := newTestCluster("test-cluster", "example.com", &k8znerv1alpha1.AddonSpec{
			Monitoring:  true,
			ExternalDNS: true,
			CertManager: true,
			Traefik:     true,
		})

		cfg, err := SpecToConfig(cluster, baseCreds)
		require.NoError(t, err)

		assert.True(t, cfg.Addons.KubePrometheusStack.Enabled)
		assert.True(t, cfg.Addons.KubePrometheusStack.Grafana.IngressEnabled)
		assert.Equal(t, "grafana.example.com", cfg.Addons.KubePrometheusStack.Grafana.IngressHost)
		assert.Equal(t, "traefik", cfg.Addons.KubePrometheusStack.Grafana.IngressClassName)
		assert.True(t, cfg.Addons.KubePrometheusStack.Grafana.IngressTLS)
	})

	t.Run("no domain means no ingress even if addon enabled", func(t *testing.T) {
		t.Parallel()
		cluster := newTestCluster("test-cluster", "", &k8znerv1alpha1.AddonSpec{
			ArgoCD:     true,
			Monitoring: true,
		})

		cfg, err := SpecToConfig(cluster, baseCreds)
		require.NoError(t, err)

		assert.True(t, cfg.Addons.ArgoCD.Enabled)
		assert.False(t, cfg.Addons.ArgoCD.IngressEnabled)
		assert.Empty(t, cfg.Addons.ArgoCD.IngressHost)

		assert.True(t, cfg.Addons.KubePrometheusStack.Enabled)
		assert.False(t, cfg.Addons.KubePrometheusStack.Grafana.IngressEnabled)
		assert.Empty(t, cfg.Addons.KubePrometheusStack.Grafana.IngressHost)
	})

	t.Run("custom subdomains override defaults", func(t *testing.T) {
		t.Parallel()
		cluster := newTestCluster("test-cluster", "example.com", &k8znerv1alpha1.AddonSpec{
			ArgoCD:           true,
			Monitoring:       true,
			ExternalDNS:      true,
			CertManager:      true,
			Traefik:          true,
			ArgoSubdomain:    "gitops",
			GrafanaSubdomain: "metrics",
		})

		cfg, err := SpecToConfig(cluster, baseCreds)
		require.NoError(t, err)

		assert.Equal(t, "gitops.example.com", cfg.Addons.ArgoCD.IngressHost)
		assert.Equal(t, "metrics.example.com", cfg.Addons.KubePrometheusStack.Grafana.IngressHost)
	})

	t.Run("domain with ExternalDNS sets Cloudflare domain and TXTOwnerID", func(t *testing.T) {
		t.Parallel()
		cluster := newTestCluster("my-cluster", "example.com", &k8znerv1alpha1.AddonSpec{
			ExternalDNS: true,
			CertManager: true,
		})

		cfg, err := SpecToConfig(cluster, baseCreds)
		require.NoError(t, err)

		assert.True(t, cfg.Addons.Cloudflare.Enabled)
		assert.Equal(t, "example.com", cfg.Addons.Cloudflare.Domain)
		assert.Equal(t, "cf-token", cfg.Addons.Cloudflare.APIToken)
		assert.Equal(t, "my-cluster", cfg.Addons.ExternalDNS.TXTOwnerID)
		assert.Equal(t, "sync", cfg.Addons.ExternalDNS.Policy)
		assert.Equal(t, []string{"ingress"}, cfg.Addons.ExternalDNS.Sources)
	})

	t.Run("CertManager Cloudflare enabled with production and email", func(t *testing.T) {
		t.Parallel()
		cluster := newTestCluster("test-cluster", "example.com", &k8znerv1alpha1.AddonSpec{
			ExternalDNS: true,
			CertManager: true,
		})

		cfg, err := SpecToConfig(cluster, baseCreds)
		require.NoError(t, err)

		assert.True(t, cfg.Addons.CertManager.Cloudflare.Enabled)
		assert.True(t, cfg.Addons.CertManager.Cloudflare.Production)
		assert.Equal(t, "admin@example.com", cfg.Addons.CertManager.Cloudflare.Email)
	})

	t.Run("Traefik uses Deployment with LoadBalancer", func(t *testing.T) {
		t.Parallel()
		cluster := newTestCluster("test-cluster", "example.com", &k8znerv1alpha1.AddonSpec{
			Traefik:     true,
			ExternalDNS: true,
			CertManager: true,
		})

		cfg, err := SpecToConfig(cluster, baseCreds)
		require.NoError(t, err)

		assert.True(t, cfg.Addons.Traefik.Enabled)
		assert.Equal(t, "Deployment", cfg.Addons.Traefik.Kind)
		assert.Nil(t, cfg.Addons.Traefik.HostNetwork)
		assert.Equal(t, "Cluster", cfg.Addons.Traefik.ExternalTrafficPolicy)
		assert.Equal(t, "traefik", cfg.Addons.Traefik.IngressClass)
	})

	t.Run("Cilium uses kube-proxy replacement with tunnel routing", func(t *testing.T) {
		t.Parallel()
		cluster := newTestCluster("test-cluster", "example.com", &k8znerv1alpha1.AddonSpec{
			Traefik: true,
		})

		cfg, err := SpecToConfig(cluster, baseCreds)
		require.NoError(t, err)

		assert.True(t, cfg.Addons.Cilium.Enabled)
		assert.True(t, cfg.Addons.Cilium.KubeProxyReplacementEnabled)
		assert.Equal(t, "tunnel", cfg.Addons.Cilium.RoutingMode)
		assert.True(t, cfg.Addons.Cilium.HubbleEnabled)
		assert.True(t, cfg.Addons.Cilium.HubbleRelayEnabled)
		assert.True(t, cfg.Addons.Cilium.HubbleUIEnabled)
	})

	t.Run("firewall has no extra rules for 80/443 (LB handles ingress)", func(t *testing.T) {
		t.Parallel()
		cluster := newTestCluster("test-cluster", "example.com", &k8znerv1alpha1.AddonSpec{
			Traefik: true,
		})

		cfg, err := SpecToConfig(cluster, baseCreds)
		require.NoError(t, err)

		require.NotNil(t, cfg.Firewall.UseCurrentIPv4)
		assert.True(t, *cfg.Firewall.UseCurrentIPv4)
		require.NotNil(t, cfg.Firewall.UseCurrentIPv6)
		assert.True(t, *cfg.Firewall.UseCurrentIPv6)
		assert.Empty(t, cfg.Firewall.ExtraRules, "no extra rules needed - LB handles ingress traffic")
	})

	t.Run("firewall has no extra rules without Traefik", func(t *testing.T) {
		t.Parallel()
		cluster := newTestCluster("test-cluster", "", &k8znerv1alpha1.AddonSpec{
			ArgoCD: true,
		})

		cfg, err := SpecToConfig(cluster, baseCreds)
		require.NoError(t, err)

		require.NotNil(t, cfg.Firewall.UseCurrentIPv4)
		assert.True(t, *cfg.Firewall.UseCurrentIPv4)
		assert.Empty(t, cfg.Firewall.ExtraRules)
	})
}
