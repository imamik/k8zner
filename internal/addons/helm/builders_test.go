package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCCMUninitializedToleration(t *testing.T) {
	t.Parallel()
	tol := CCMUninitializedToleration()
	assert.Equal(t, "node.cloudprovider.kubernetes.io/uninitialized", tol["key"])
	assert.Equal(t, "Exists", tol["operator"])
}

func TestBootstrapTolerations(t *testing.T) {
	t.Parallel()
	tols := BootstrapTolerations()
	require.Len(t, tols, 3)
	assert.Equal(t, "node-role.kubernetes.io/control-plane", tols[0]["key"])
	assert.Equal(t, "node.cloudprovider.kubernetes.io/uninitialized", tols[1]["key"])
	assert.Equal(t, "true", tols[1]["value"])
	assert.Equal(t, "node.kubernetes.io/not-ready", tols[2]["key"])
}

func TestControlPlaneNodeSelector(t *testing.T) {
	t.Parallel()
	ns := ControlPlaneNodeSelector()
	assert.Equal(t, "", ns["node-role.kubernetes.io/control-plane"])
}

func TestTopologySpreadFunc(t *testing.T) {
	t.Parallel()
	constraints := TopologySpread("my-instance", "my-name", "DoNotSchedule")
	require.Len(t, constraints, 2)

	hostname := constraints[0]
	assert.Equal(t, "kubernetes.io/hostname", hostname["topologyKey"])
	assert.Equal(t, "DoNotSchedule", hostname["whenUnsatisfiable"])
	labels := hostname["labelSelector"].(Values)["matchLabels"].(Values)
	assert.Equal(t, "my-instance", labels["app.kubernetes.io/instance"])
	assert.Equal(t, "my-name", labels["app.kubernetes.io/name"])

	zone := constraints[1]
	assert.Equal(t, "topology.kubernetes.io/zone", zone["topologyKey"])
	assert.Equal(t, "ScheduleAnyway", zone["whenUnsatisfiable"])
}

func TestNamespaceManifest(t *testing.T) {
	t.Parallel()
	t.Run("with labels", func(t *testing.T) {
		t.Parallel()
		ns := NamespaceManifest("test-ns", map[string]string{"name": "test-ns"})
		assert.Contains(t, ns, "apiVersion: v1")
		assert.Contains(t, ns, "kind: Namespace")
		assert.Contains(t, ns, "name: test-ns")
		assert.Contains(t, ns, "labels:")
	})

	t.Run("without labels", func(t *testing.T) {
		t.Parallel()
		ns := NamespaceManifest("cert-manager", nil)
		assert.Contains(t, ns, "name: cert-manager")
		assert.NotContains(t, ns, "labels:")
	})
}
