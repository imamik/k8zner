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

	// Check networking configuration
	networking, ok := values["networking"].(helm.Values)
	assert.True(t, ok)
	assert.Equal(t, true, networking["enabled"])

	// Check network secret ref (nested in networking.network.valueFrom)
	network, ok := networking["network"].(helm.Values)
	assert.True(t, ok)
	valueFrom, ok := network["valueFrom"].(helm.Values)
	assert.True(t, ok)
	secretKeyRef, ok := valueFrom["secretKeyRef"].(helm.Values)
	assert.True(t, ok)
	assert.Equal(t, "hcloud", secretKeyRef["name"])
	assert.Equal(t, "network", secretKeyRef["key"])

	// Note: HCLOUD_TOKEN and HCLOUD_LOAD_BALANCERS_ENABLED are not set in values
	// because they use the chart defaults (HCLOUD_TOKEN from env, LB enabled by default)
}
