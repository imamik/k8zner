package addons

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSIManifests_TemplateProcessing(t *testing.T) {
	templateData := map[string]string{
		"Token":                 "test-token-123",
		"ControllerReplicas":    "2",
		"IsDefaultStorageClass": "true",
	}

	manifestBytes, err := readAndProcessManifests("hcloud-csi", templateData)
	require.NoError(t, err)
	assert.NotEmpty(t, manifestBytes)

	manifest := string(manifestBytes)

	// Verify token is injected
	assert.Contains(t, manifest, "test-token-123")

	// Verify controller replicas is injected
	assert.Contains(t, manifest, "replicas: 2")

	// Verify storage class is default
	assert.Contains(t, manifest, `storageclass.kubernetes.io/is-default-class: "true"`)

	// Verify expected resources are present
	assert.Contains(t, manifest, "kind: Secret")
	assert.Contains(t, manifest, "kind: CSIDriver")
	assert.Contains(t, manifest, "kind: ServiceAccount")
	assert.Contains(t, manifest, "kind: ClusterRole")
	assert.Contains(t, manifest, "kind: ClusterRoleBinding")
	assert.Contains(t, manifest, "kind: Deployment")
	assert.Contains(t, manifest, "kind: DaemonSet")
	assert.Contains(t, manifest, "kind: StorageClass")

	// Verify CSI driver name
	assert.Contains(t, manifest, "csi.hetzner.cloud")
}

func TestCSIManifests_SingleReplica(t *testing.T) {
	templateData := map[string]string{
		"Token":                 "test-token",
		"ControllerReplicas":    "1",
		"IsDefaultStorageClass": "false",
	}

	manifestBytes, err := readAndProcessManifests("hcloud-csi", templateData)
	require.NoError(t, err)

	manifest := string(manifestBytes)

	// Verify single replica
	assert.Contains(t, manifest, "replicas: 1")

	// Verify storage class is not default
	assert.Contains(t, manifest, `storageclass.kubernetes.io/is-default-class: "false"`)
}

func TestCSIManifests_RequiredComponents(t *testing.T) {
	templateData := map[string]string{
		"Token":                 "test-token",
		"ControllerReplicas":    "1",
		"IsDefaultStorageClass": "true",
	}

	manifestBytes, err := readAndProcessManifests("hcloud-csi", templateData)
	require.NoError(t, err)

	manifest := string(manifestBytes)

	// Verify CSI controller components
	assert.Contains(t, manifest, "hcloud-csi-controller")
	assert.Contains(t, manifest, "hcloud-csi-driver-controller")

	// Verify CSI node components
	assert.Contains(t, manifest, "hcloud-csi-node")
	assert.Contains(t, manifest, "hcloud-csi-driver-node")

	// Verify sidecar containers
	assert.Contains(t, manifest, "csi-attacher")
	assert.Contains(t, manifest, "csi-provisioner")
	assert.Contains(t, manifest, "csi-resizer")
	assert.Contains(t, manifest, "csi-node-driver-registrar")
	assert.Contains(t, manifest, "liveness-probe")

	// Verify RBAC
	assert.Contains(t, manifest, "persistentvolumes")
	assert.Contains(t, manifest, "persistentvolumeclaims")

	// Verify StorageClass settings
	assert.Contains(t, manifest, "WaitForFirstConsumer")
	assert.Contains(t, manifest, "allowVolumeExpansion: true")
}

func TestCSIManifests_SecurityContext(t *testing.T) {
	templateData := map[string]string{
		"Token":                 "test-token",
		"ControllerReplicas":    "1",
		"IsDefaultStorageClass": "true",
	}

	manifestBytes, err := readAndProcessManifests("hcloud-csi", templateData)
	require.NoError(t, err)

	manifest := string(manifestBytes)

	// Verify security contexts are present
	assert.Contains(t, manifest, "readOnlyRootFilesystem: true")
	assert.Contains(t, manifest, "allowPrivilegeEscalation: false")

	// Node plugin needs privileged access for device mounting
	assert.Contains(t, manifest, "privileged: true")
}

func TestCSIManifests_Tolerations(t *testing.T) {
	templateData := map[string]string{
		"Token":                 "test-token",
		"ControllerReplicas":    "1",
		"IsDefaultStorageClass": "true",
	}

	manifestBytes, err := readAndProcessManifests("hcloud-csi", templateData)
	require.NoError(t, err)

	manifest := string(manifestBytes)

	// Verify control plane tolerations for controller
	assert.Contains(t, manifest, "node-role.kubernetes.io/control-plane")

	// Verify node tolerations for DaemonSet
	assert.Contains(t, manifest, "NoExecute")
	assert.Contains(t, manifest, "NoSchedule")
	assert.Contains(t, manifest, "CriticalAddonsOnly")
}

func TestCSIManifests_AllManifestFilesIncluded(t *testing.T) {
	templateData := map[string]string{
		"Token":                 "test-token",
		"ControllerReplicas":    "1",
		"IsDefaultStorageClass": "true",
	}

	manifestBytes, err := readAndProcessManifests("hcloud-csi", templateData)
	require.NoError(t, err)

	manifest := string(manifestBytes)

	// Count YAML document separators to verify all files are included
	// We expect 7 manifest files (some may have multiple documents)
	docCount := strings.Count(manifest, "---")
	assert.GreaterOrEqual(t, docCount, 6, "Expected at least 6 YAML document separators")
}
