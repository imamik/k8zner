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
