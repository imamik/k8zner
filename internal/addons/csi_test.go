package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildCSIValues_SingleControlPlane(t *testing.T) {
	values := buildCSIValues(1, true)

	// Verify controller configuration
	controller, ok := values["controller"].(map[string]any)
	assert.True(t, ok, "controller should be a map")
	assert.Equal(t, 1, controller["replicaCount"])

	// Verify node selector
	nodeSelector, ok := controller["nodeSelector"].(map[string]string)
	assert.True(t, ok)
	assert.Equal(t, "", nodeSelector["node-role.kubernetes.io/control-plane"])

	// Verify storage class
	storageClasses, ok := values["storageClasses"].([]map[string]any)
	assert.True(t, ok)
	assert.Len(t, storageClasses, 1)
	assert.Equal(t, "hcloud-volumes", storageClasses[0]["name"])
	assert.Equal(t, true, storageClasses[0]["defaultStorageClass"])
	assert.Equal(t, "Delete", storageClasses[0]["reclaimPolicy"])
}

func TestBuildCSIValues_MultipleControlPlane(t *testing.T) {
	values := buildCSIValues(3, false)

	// Verify controller replicas scale with control plane
	controller, ok := values["controller"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, 2, controller["replicaCount"], "Should have 2 replicas for HA")

	// Verify storage class is not default
	storageClasses, ok := values["storageClasses"].([]map[string]any)
	assert.True(t, ok)
	assert.Equal(t, false, storageClasses[0]["defaultStorageClass"])
}

func TestBuildCSIValues_PodDisruptionBudget(t *testing.T) {
	values := buildCSIValues(2, true)

	controller, ok := values["controller"].(map[string]any)
	assert.True(t, ok)

	pdb, ok := controller["podDisruptionBudget"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, true, pdb["create"])
	assert.Equal(t, "1", pdb["maxUnavailable"])
	assert.Nil(t, pdb["minAvailable"])
}

func TestBuildCSIValues_TopologySpreadConstraints(t *testing.T) {
	values := buildCSIValues(1, true)

	controller, ok := values["controller"].(map[string]any)
	assert.True(t, ok)

	constraints, ok := controller["topologySpreadConstraints"].([]map[string]any)
	assert.True(t, ok)
	assert.Len(t, constraints, 1)

	constraint := constraints[0]
	assert.Equal(t, "kubernetes.io/hostname", constraint["topologyKey"])
	assert.Equal(t, 1, constraint["maxSkew"])
	assert.Equal(t, "DoNotSchedule", constraint["whenUnsatisfiable"])

	// Verify label selector
	labelSelector, ok := constraint["labelSelector"].(map[string]any)
	assert.True(t, ok)
	matchLabels, ok := labelSelector["matchLabels"].(map[string]string)
	assert.True(t, ok)
	assert.Equal(t, "hcloud-csi", matchLabels["app.kubernetes.io/name"])
	assert.Equal(t, "hcloud-csi", matchLabels["app.kubernetes.io/instance"])
	assert.Equal(t, "controller", matchLabels["app.kubernetes.io/component"])

	// Verify matchLabelKeys
	matchLabelKeys, ok := constraint["matchLabelKeys"].([]string)
	assert.True(t, ok)
	assert.Equal(t, []string{"pod-template-hash"}, matchLabelKeys)
}

func TestBuildCSIValues_Tolerations(t *testing.T) {
	values := buildCSIValues(1, true)

	controller, ok := values["controller"].(map[string]any)
	assert.True(t, ok)

	tolerations, ok := controller["tolerations"].([]map[string]any)
	assert.True(t, ok)
	assert.Len(t, tolerations, 1)

	toleration := tolerations[0]
	assert.Equal(t, "node-role.kubernetes.io/control-plane", toleration["key"])
	assert.Equal(t, "NoSchedule", toleration["effect"])
	assert.Equal(t, "Exists", toleration["operator"])
}

func TestBuildCSIValues_StorageClassConfiguration(t *testing.T) {
	tests := []struct {
		name                string
		defaultStorageClass bool
		want                bool
	}{
		{
			name:                "default storage class enabled",
			defaultStorageClass: true,
			want:                true,
		},
		{
			name:                "default storage class disabled",
			defaultStorageClass: false,
			want:                false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := buildCSIValues(1, tt.defaultStorageClass)

			storageClasses, ok := values["storageClasses"].([]map[string]any)
			assert.True(t, ok)
			assert.Len(t, storageClasses, 1)

			sc := storageClasses[0]
			assert.Equal(t, tt.want, sc["defaultStorageClass"])
			assert.Equal(t, "hcloud-volumes", sc["name"])
			assert.Equal(t, "Delete", sc["reclaimPolicy"])
		})
	}
}
