package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"hcloud-k8s/internal/addons/helm"
)

func TestBuildCCMValues(t *testing.T) {
	token := "test-token"
	networkID := int64(12345)

	values := buildCCMValues(token, networkID)

	// Check kind
	assert.Equal(t, "DaemonSet", values["kind"])

	// Check node selector
	nodeSelector, ok := values["nodeSelector"].(helm.Values)
	assert.True(t, ok)
	assert.Contains(t, nodeSelector, "node-role.kubernetes.io/control-plane")

	// Check networking
	networking, ok := values["networking"].(helm.Values)
	assert.True(t, ok)
	assert.Equal(t, true, networking["enabled"])

	// Check env variables
	env, ok := values["env"].(helm.Values)
	assert.True(t, ok)

	// Check token secret ref
	hcloudToken, ok := env["HCLOUD_TOKEN"].(helm.Values)
	assert.True(t, ok)
	valueFrom, ok := hcloudToken["valueFrom"].(helm.Values)
	assert.True(t, ok)
	secretKeyRef, ok := valueFrom["secretKeyRef"].(helm.Values)
	assert.True(t, ok)
	assert.Equal(t, "hcloud", secretKeyRef["name"])
	assert.Equal(t, "token", secretKeyRef["key"])

	// Check network secret ref
	hcloudNetwork, ok := env["HCLOUD_NETWORK"].(helm.Values)
	assert.True(t, ok)
	valueFrom, ok = hcloudNetwork["valueFrom"].(helm.Values)
	assert.True(t, ok)
	secretKeyRef, ok = valueFrom["secretKeyRef"].(helm.Values)
	assert.True(t, ok)
	assert.Equal(t, "hcloud", secretKeyRef["name"])
	assert.Equal(t, "network", secretKeyRef["key"])

	// Check load balancer enabled
	lbEnabled, ok := env["HCLOUD_LOAD_BALANCERS_ENABLED"].(helm.Values)
	assert.True(t, ok)
	assert.Equal(t, "true", lbEnabled["value"])
}
