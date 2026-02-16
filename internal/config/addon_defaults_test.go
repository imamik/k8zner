package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultCilium(t *testing.T) {
	t.Parallel()
	c := DefaultCilium()
	assert.True(t, c.Enabled)
	assert.True(t, c.KubeProxyReplacementEnabled)
	assert.Equal(t, "tunnel", c.RoutingMode)
	assert.True(t, c.HubbleEnabled)
	assert.True(t, c.HubbleRelayEnabled)
	assert.True(t, c.HubbleUIEnabled)
}

func TestDefaultTraefik(t *testing.T) {
	t.Parallel()

	t.Run("enabled", func(t *testing.T) {
		t.Parallel()
		tr := DefaultTraefik(true)
		assert.True(t, tr.Enabled)
		assert.Equal(t, "Cluster", tr.ExternalTrafficPolicy)
		assert.Equal(t, "traefik", tr.IngressClass)
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		tr := DefaultTraefik(false)
		assert.False(t, tr.Enabled)
	})
}

func TestDefaultCCM(t *testing.T) {
	t.Parallel()
	assert.True(t, DefaultCCM().Enabled)
}

func TestDefaultCSI(t *testing.T) {
	t.Parallel()
	c := DefaultCSI()
	assert.True(t, c.Enabled)
	assert.True(t, c.DefaultStorageClass)
}

func TestDefaultGatewayAPICRDs(t *testing.T) {
	t.Parallel()
	assert.True(t, DefaultGatewayAPICRDs().Enabled)
}

func TestDefaultPrometheusOperatorCRDs(t *testing.T) {
	t.Parallel()
	assert.True(t, DefaultPrometheusOperatorCRDs().Enabled)
}

func TestBuildIngressHost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		domain     string
		defaultSub string
		customSub  string
		want       string
	}{
		{"default subdomain", "example.com", "argo", "", "argo.example.com"},
		{"custom subdomain", "example.com", "argo", "gitops", "gitops.example.com"},
		{"empty domain", "", "argo", "", ""},
		{"empty domain with custom", "", "argo", "gitops", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, BuildIngressHost(tt.domain, tt.defaultSub, tt.customSub))
		})
	}
}

func TestDefaultExternalDNS(t *testing.T) {
	t.Parallel()

	t.Run("enabled", func(t *testing.T) {
		t.Parallel()
		dns := DefaultExternalDNS(true)
		assert.True(t, dns.Enabled)
		assert.Equal(t, "sync", dns.Policy)
		assert.Equal(t, []string{"ingress"}, dns.Sources)
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		dns := DefaultExternalDNS(false)
		assert.False(t, dns.Enabled)
		assert.Empty(t, dns.Policy)
		assert.Empty(t, dns.Sources)
	})
}

func TestDefaultTalosBackup(t *testing.T) {
	t.Parallel()
	b := DefaultTalosBackup()
	assert.Equal(t, "etcd-backups", b.S3Prefix)
	assert.True(t, b.EnableCompression)
	assert.True(t, b.EncryptionDisabled)
	assert.False(t, b.Enabled, "callers must explicitly set Enabled")
}

func TestDefaultPrometheusPersistence(t *testing.T) {
	t.Parallel()
	p := DefaultPrometheusPersistence()
	assert.True(t, p.Enabled)
	assert.Equal(t, "50Gi", p.Size)
}
