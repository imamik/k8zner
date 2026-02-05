package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/addons/helm"
	"github.com/imamik/k8zner/internal/config"
)

func TestBuildCSIValues(t *testing.T) {
	tests := []struct {
		name                        string
		controlPlaneCount           int
		defaultStorageClass         bool
		expectedReplicas            int
		expectedDefaultStorageClass bool
	}{
		{
			name:                        "single control plane",
			controlPlaneCount:           1,
			defaultStorageClass:         true,
			expectedReplicas:            1,
			expectedDefaultStorageClass: true,
		},
		{
			name:                        "HA control plane",
			controlPlaneCount:           3,
			defaultStorageClass:         false,
			expectedReplicas:            2,
			expectedDefaultStorageClass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				ControlPlane: config.ControlPlaneConfig{
					NodePools: []config.ControlPlaneNodePool{
						{Name: "cp", Count: tt.controlPlaneCount},
					},
				},
				Addons: config.AddonsConfig{
					CSI: config.CSIConfig{
						DefaultStorageClass: tt.defaultStorageClass,
					},
				},
			}
			values := buildCSIValues(cfg)

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

			// Note: dnsPolicy is injected via post-render patching (patchDeploymentDNSPolicy),
			// not as a helm value, because the CSI chart doesn't support it natively.

			// Check node selector
			nodeSelector, ok := controller["nodeSelector"].(helm.Values)
			require.True(t, ok)
			assert.Contains(t, nodeSelector, "node-role.kubernetes.io/control-plane")

			// Check tolerations (control-plane, uninitialized, and not-ready)
			// All three are required for CSI to schedule during bootstrap
			tolerations, ok := controller["tolerations"].([]helm.Values)
			require.True(t, ok)
			assert.Len(t, tolerations, 3)
			assert.Equal(t, "node-role.kubernetes.io/control-plane", tolerations[0]["key"])
			assert.Equal(t, "node.cloudprovider.kubernetes.io/uninitialized", tolerations[1]["key"])
			assert.Equal(t, "node.kubernetes.io/not-ready", tolerations[2]["key"])

			// Check storage classes - we now have two: encrypted (default) and non-encrypted
			storageClasses, ok := values["storageClasses"].([]helm.Values)
			require.True(t, ok)
			assert.Len(t, storageClasses, 2, "should have 2 storage classes (encrypted and non-encrypted)")

			// Check encrypted storage class (first one, should be default if requested)
			encryptedSC := storageClasses[0]
			assert.Equal(t, "hcloud-volumes-encrypted", encryptedSC["name"])
			assert.Equal(t, tt.expectedDefaultStorageClass, encryptedSC["defaultStorageClass"])

			// Verify extraParameters for encryption
			extraParams, ok := encryptedSC["extraParameters"].(helm.Values)
			require.True(t, ok, "encrypted storage class should have extraParameters")
			assert.Equal(t, "hcloud-csi-secret", extraParams["csi.storage.k8s.io/node-publish-secret-name"])

			// Check non-encrypted storage class (second one)
			nonEncryptedSC := storageClasses[1]
			assert.Equal(t, "hcloud-volumes", nonEncryptedSC["name"])
			assert.Equal(t, false, nonEncryptedSC["defaultStorageClass"], "non-encrypted should not be default")
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
