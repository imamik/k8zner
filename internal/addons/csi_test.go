package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hcloud-k8s/internal/addons/helm"
)

func TestBuildCSIValues(t *testing.T) {
	tests := []struct {
		name                       string
		controlPlaneCount          int
		defaultStorageClass        bool
		expectedReplicas           int
		expectedDefaultStorageClass bool
	}{
		{
			name:                       "single control plane",
			controlPlaneCount:          1,
			defaultStorageClass:        true,
			expectedReplicas:           1,
			expectedDefaultStorageClass: true,
		},
		{
			name:                       "HA control plane",
			controlPlaneCount:          3,
			defaultStorageClass:        false,
			expectedReplicas:           2,
			expectedDefaultStorageClass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encryptionKey := "test-encryption-key"
			values := buildCSIValues(tt.controlPlaneCount, encryptionKey, tt.defaultStorageClass)

			// Check controller configuration
			controller, ok := values["controller"].(helm.Values)
			require.True(t, ok, "controller should be a Values map")
			assert.Equal(t, tt.expectedReplicas, controller["replicaCount"])

			// Check PDB
			pdb, ok := controller["podDisruptionBudget"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, true, pdb["create"])
			assert.Equal(t, "1", pdb["maxUnavailable"])

			// Check topology spread constraints
			tsc, ok := controller["topologySpreadConstraints"].([]helm.Values)
			require.True(t, ok)
			assert.Len(t, tsc, 1)
			assert.Equal(t, "kubernetes.io/hostname", tsc[0]["topologyKey"])
			assert.Equal(t, "DoNotSchedule", tsc[0]["whenUnsatisfiable"])

			// Check node selector
			nodeSelector, ok := controller["nodeSelector"].(helm.Values)
			require.True(t, ok)
			assert.Contains(t, nodeSelector, "node-role.kubernetes.io/control-plane")

			// Check tolerations
			tolerations, ok := controller["tolerations"].([]helm.Values)
			require.True(t, ok)
			assert.Len(t, tolerations, 1)
			assert.Equal(t, "node-role.kubernetes.io/control-plane", tolerations[0]["key"])

			// Check storage classes
			storageClasses, ok := values["storageClasses"].([]helm.Values)
			require.True(t, ok)
			assert.Len(t, storageClasses, 1)
			assert.Equal(t, "hcloud-volumes", storageClasses[0]["name"])
			assert.Equal(t, tt.expectedDefaultStorageClass, storageClasses[0]["defaultStorageClass"])

			// Check secret
			secret, ok := values["secret"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, true, secret["create"])
			secretData, ok := secret["data"].(helm.Values)
			require.True(t, ok)
			assert.Equal(t, encryptionKey, secretData["encryption-passphrase"])
		})
	}
}

func TestGenerateEncryptionKey(t *testing.T) {
	key, err := generateEncryptionKey(32)
	require.NoError(t, err)

	// Check that key is a valid hex string
	assert.Equal(t, 64, len(key), "32 bytes should produce 64 hex characters")

	// Check that multiple calls produce different keys
	key2, err := generateEncryptionKey(32)
	require.NoError(t, err)
	assert.NotEqual(t, key, key2, "multiple calls should produce different keys")
}
