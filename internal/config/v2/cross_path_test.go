package v2_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/internal/config"
	v2 "github.com/imamik/k8zner/internal/config/v2"
	opprovisioning "github.com/imamik/k8zner/internal/operator/provisioning"
)

// TestCrossPath_SharedDefaultsUsed verifies that both config paths use the
// shared config.Default*() builders, producing identical defaults for all
// core addon fields. Since both paths now call the same functions, this test
// is a spot-check safety net rather than exhaustive field comparison.
func TestCrossPath_SharedDefaultsUsed(t *testing.T) {
	t.Parallel()

	os.Setenv("HCLOUD_TOKEN", "test-token")
	os.Setenv("CF_API_TOKEN", "cf-token")
	defer os.Unsetenv("HCLOUD_TOKEN")
	defer os.Unsetenv("CF_API_TOKEN")

	// --- CLI path ---
	v2Cfg := &v2.Config{
		Name:   "cross-path-test",
		Region: v2.RegionFalkenstein,
		Mode:   v2.ModeHA,
		Workers: v2.Worker{
			Count: 3,
			Size:  v2.SizeCX33,
		},
		Domain:     "example.com",
		Monitoring: true,
	}

	expanded, err := v2.Expand(v2Cfg)
	require.NoError(t, err, "v2.Expand should not error")

	// --- Operator path ---
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cross-path-test"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region: "fsn1",
			Domain: "example.com",
			Kubernetes: k8znerv1alpha1.KubernetesSpec{
				Version: expanded.Kubernetes.Version,
			},
			Talos: k8znerv1alpha1.TalosSpec{
				Version: expanded.Talos.Version,
			},
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 3, Size: "cx23"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 3, Size: "cx33"},
			Addons: &k8znerv1alpha1.AddonSpec{
				MetricsServer: true,
				CertManager:   true,
				Traefik:       true,
				ExternalDNS:   true,
				ArgoCD:        true,
				Monitoring:    true,
			},
			CredentialsRef: corev1.LocalObjectReference{Name: "creds"},
		},
	}

	creds := &opprovisioning.Credentials{
		HCloudToken:        "test-token",
		CloudflareAPIToken: "cf-token",
	}

	fromCRD, err := opprovisioning.SpecToConfig(cluster, creds)
	require.NoError(t, err, "SpecToConfig should not error")

	// Both paths must produce identical structs from shared defaults
	t.Run("Cilium identical", func(t *testing.T) {
		t.Parallel()
		want := config.DefaultCilium()
		assert.Equal(t, want, expanded.Addons.Cilium, "CLI Cilium should match DefaultCilium()")
		assert.Equal(t, want, fromCRD.Addons.Cilium, "CRD Cilium should match DefaultCilium()")
	})

	t.Run("Traefik identical", func(t *testing.T) {
		t.Parallel()
		want := config.DefaultTraefik(true)
		assert.Equal(t, want, expanded.Addons.Traefik, "CLI Traefik should match DefaultTraefik(true)")
		assert.Equal(t, want, fromCRD.Addons.Traefik, "CRD Traefik should match DefaultTraefik(true)")
	})

	t.Run("CCM identical", func(t *testing.T) {
		t.Parallel()
		want := config.DefaultCCM()
		assert.Equal(t, want, expanded.Addons.CCM)
		assert.Equal(t, want, fromCRD.Addons.CCM)
	})

	t.Run("CSI identical", func(t *testing.T) {
		t.Parallel()
		want := config.DefaultCSI()
		assert.Equal(t, want, expanded.Addons.CSI)
		assert.Equal(t, want, fromCRD.Addons.CSI)
	})

	t.Run("GatewayAPICRDs identical", func(t *testing.T) {
		t.Parallel()
		want := config.DefaultGatewayAPICRDs()
		assert.Equal(t, want, expanded.Addons.GatewayAPICRDs)
		assert.Equal(t, want, fromCRD.Addons.GatewayAPICRDs)
	})

	t.Run("PrometheusOperatorCRDs identical", func(t *testing.T) {
		t.Parallel()
		want := config.DefaultPrometheusOperatorCRDs()
		assert.Equal(t, want, expanded.Addons.PrometheusOperatorCRDs)
		assert.Equal(t, want, fromCRD.Addons.PrometheusOperatorCRDs)
	})

	t.Run("Kubernetes domain", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "cluster.local", expanded.Kubernetes.Domain)
		assert.Equal(t, "cluster.local", fromCRD.Kubernetes.Domain)
	})

	t.Run("Firewall defaults match", func(t *testing.T) {
		t.Parallel()
		require.NotNil(t, expanded.Firewall.UseCurrentIPv4)
		require.NotNil(t, fromCRD.Firewall.UseCurrentIPv4)
		assert.Equal(t, *expanded.Firewall.UseCurrentIPv4, *fromCRD.Firewall.UseCurrentIPv4)

		require.NotNil(t, expanded.Firewall.UseCurrentIPv6)
		require.NotNil(t, fromCRD.Firewall.UseCurrentIPv6)
		assert.Equal(t, *expanded.Firewall.UseCurrentIPv6, *fromCRD.Firewall.UseCurrentIPv6)
	})

	t.Run("ArgoCD ingress defaults match", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, expanded.Addons.ArgoCD.IngressEnabled, fromCRD.Addons.ArgoCD.IngressEnabled)
		assert.Equal(t, expanded.Addons.ArgoCD.IngressHost, fromCRD.Addons.ArgoCD.IngressHost)
		assert.Equal(t, expanded.Addons.ArgoCD.IngressClassName, fromCRD.Addons.ArgoCD.IngressClassName)
		assert.Equal(t, expanded.Addons.ArgoCD.IngressTLS, fromCRD.Addons.ArgoCD.IngressTLS)
	})

	t.Run("Monitoring ingress defaults match", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, expanded.Addons.KubePrometheusStack.Enabled, fromCRD.Addons.KubePrometheusStack.Enabled)
		assert.Equal(t, expanded.Addons.KubePrometheusStack.Grafana.IngressHost, fromCRD.Addons.KubePrometheusStack.Grafana.IngressHost)
	})

	t.Run("Cloudflare and ExternalDNS match", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, expanded.Addons.Cloudflare.Enabled, fromCRD.Addons.Cloudflare.Enabled)
		assert.Equal(t, expanded.Addons.ExternalDNS.Policy, fromCRD.Addons.ExternalDNS.Policy)
		assert.Equal(t, expanded.Addons.ExternalDNS.Sources, fromCRD.Addons.ExternalDNS.Sources)
	})
}

// TestCrossPath_WithoutDomain verifies both paths match when no domain is set.
func TestCrossPath_WithoutDomain(t *testing.T) {
	t.Parallel()

	os.Setenv("HCLOUD_TOKEN", "test-token")
	defer os.Unsetenv("HCLOUD_TOKEN")

	// CLI path
	v2Cfg := &v2.Config{
		Name:   "no-domain-test",
		Region: v2.RegionNuremberg,
		Mode:   v2.ModeDev,
		Workers: v2.Worker{
			Count: 1,
			Size:  v2.SizeCX23,
		},
	}

	expanded, err := v2.Expand(v2Cfg)
	require.NoError(t, err)

	// Operator path
	cluster := &k8znerv1alpha1.K8znerCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "no-domain-test"},
		Spec: k8znerv1alpha1.K8znerClusterSpec{
			Region: "nbg1",
			Kubernetes: k8znerv1alpha1.KubernetesSpec{
				Version: expanded.Kubernetes.Version,
			},
			Talos: k8znerv1alpha1.TalosSpec{
				Version: expanded.Talos.Version,
			},
			ControlPlanes: k8znerv1alpha1.ControlPlaneSpec{Count: 1, Size: "cx23"},
			Workers:       k8znerv1alpha1.WorkerSpec{Count: 1, Size: "cx23"},
			Addons: &k8znerv1alpha1.AddonSpec{
				MetricsServer: true,
				CertManager:   true,
				Traefik:       true,
			},
			CredentialsRef: corev1.LocalObjectReference{Name: "creds"},
		},
	}

	fromCRD, err := opprovisioning.SpecToConfig(cluster, &opprovisioning.Credentials{
		HCloudToken: "test-token",
	})
	require.NoError(t, err)

	// Domain-dependent addons should be disabled in both
	assert.False(t, expanded.Addons.Cloudflare.Enabled)
	assert.False(t, fromCRD.Addons.Cloudflare.Enabled)
	assert.False(t, expanded.Addons.ExternalDNS.Enabled)
	assert.False(t, fromCRD.Addons.ExternalDNS.Enabled)

	// Shared defaults still identical
	assert.Equal(t, config.DefaultCilium(), expanded.Addons.Cilium)
	assert.Equal(t, config.DefaultCilium(), fromCRD.Addons.Cilium)
	assert.Equal(t, config.DefaultTraefik(true), expanded.Addons.Traefik)
	assert.Equal(t, config.DefaultTraefik(true), fromCRD.Addons.Traefik)
}
