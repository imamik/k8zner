package addons

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const testMultiDocYAML = `apiVersion: v1
kind: ServiceAccount
metadata:
  name: hcloud-csi-controller
  namespace: kube-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hcloud-csi-controller
  namespace: kube-system
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: hcloud-csi-driver
        image: hetznercloud/hcloud-csi-driver:v2.18.3
      dnsPolicy: ClusterFirst
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: hcloud-csi-node
  namespace: kube-system
spec:
  template:
    spec:
      containers:
      - name: hcloud-csi-driver
        image: hetznercloud/hcloud-csi-driver:v2.18.3
`

func TestPatchDeploymentDNSPolicy(t *testing.T) {
	t.Parallel()
	t.Run("patches target deployment", func(t *testing.T) {
		t.Parallel()
		result, err := patchDeploymentDNSPolicy([]byte(testMultiDocYAML), "hcloud-csi-controller", "Default")
		require.NoError(t, err)

		resultStr := string(result)

		// The patched deployment should have dnsPolicy: Default
		assert.Contains(t, resultStr, "dnsPolicy: Default")
		// Original ClusterFirst should be replaced
		assert.NotContains(t, resultStr, "ClusterFirst")

		// Other documents should still be present
		assert.Contains(t, resultStr, "kind: ServiceAccount")
		assert.Contains(t, resultStr, "kind: DaemonSet")
		assert.Contains(t, resultStr, "hcloud-csi-node")
	})

	t.Run("preserves all documents", func(t *testing.T) {
		t.Parallel()
		result, err := patchDeploymentDNSPolicy([]byte(testMultiDocYAML), "hcloud-csi-controller", "Default")
		require.NoError(t, err)

		// Count YAML document separators - should have 2 separators for 3 documents
		separators := strings.Count(string(result), "---\n")
		assert.Equal(t, 2, separators, "should have 2 document separators for 3 documents")
	})

	t.Run("errors on non-matching deployment name", func(t *testing.T) {
		t.Parallel()
		_, err := patchDeploymentDNSPolicy([]byte(testMultiDocYAML), "nonexistent-deployment", "Default")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("does not modify other deployments", func(t *testing.T) {
		t.Parallel()
		multiDeployYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: target-deploy
spec:
  template:
    spec:
      containers:
      - name: app
      dnsPolicy: ClusterFirst
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: other-deploy
spec:
  template:
    spec:
      containers:
      - name: app
      dnsPolicy: ClusterFirst
`
		result, err := patchDeploymentDNSPolicy([]byte(multiDeployYAML), "target-deploy", "Default")
		require.NoError(t, err)

		// Split result back into documents and verify
		docs := strings.Split(string(result), "---\n")
		require.Len(t, docs, 2)

		// First doc (target) should have Default
		assert.Contains(t, docs[0], "dnsPolicy: Default")
		// Second doc (other) should still have ClusterFirst
		assert.Contains(t, docs[1], "dnsPolicy: ClusterFirst")
	})

	t.Run("adds dnsPolicy when not present", func(t *testing.T) {
		t.Parallel()
		noPolicyYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-deploy
spec:
  template:
    spec:
      containers:
      - name: app
`
		result, err := patchDeploymentDNSPolicy([]byte(noPolicyYAML), "my-deploy", "Default")
		require.NoError(t, err)
		assert.Contains(t, string(result), "dnsPolicy: Default")
	})
}

// --- patchManifestObjects direct tests ---

func TestPatchManifestObjects_InvalidYAML(t *testing.T) {
	t.Parallel()
	invalidYAML := []byte(`{{{invalid yaml`)

	_, err := patchManifestObjects(invalidYAML, "test",
		func(_ *unstructured.Unstructured) bool { return true },
		func(_ *unstructured.Unstructured) error { return nil },
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode")
}

func TestPatchManifestObjects_NoMatch(t *testing.T) {
	t.Parallel()
	yaml := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
`)
	_, err := patchManifestObjects(yaml, "Deployment/my-deploy",
		func(obj *unstructured.Unstructured) bool {
			return obj.GetKind() == "Deployment"
		},
		func(_ *unstructured.Unstructured) error { return nil },
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in manifests")
}

func TestPatchManifestObjects_PatchFunctionError(t *testing.T) {
	t.Parallel()
	yaml := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
`)
	_, err := patchManifestObjects(yaml, "ConfigMap/test-cm",
		func(_ *unstructured.Unstructured) bool { return true },
		func(_ *unstructured.Unstructured) error {
			return fmt.Errorf("patch failed")
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "patch failed")
}

func TestPatchManifestObjects_EmptyDocumentsSkipped(t *testing.T) {
	t.Parallel()
	// YAML with empty documents (just separators)
	yaml := []byte(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  key: value
---
`)
	called := false
	result, err := patchManifestObjects(yaml, "ConfigMap",
		func(obj *unstructured.Unstructured) bool {
			return obj.GetKind() == "ConfigMap"
		},
		func(obj *unstructured.Unstructured) error {
			called = true
			return nil
		},
	)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Contains(t, string(result), "kind: ConfigMap")
}

func TestPatchManifestObjects_PatchModifiesObject(t *testing.T) {
	t.Parallel()
	yaml := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
  labels: {}
`)
	result, err := patchManifestObjects(yaml, "ConfigMap",
		func(obj *unstructured.Unstructured) bool {
			return obj.GetKind() == "ConfigMap"
		},
		func(obj *unstructured.Unstructured) error {
			labels := obj.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels["patched"] = "true"
			obj.SetLabels(labels)
			return nil
		},
	)
	require.NoError(t, err)
	assert.Contains(t, string(result), "patched: \"true\"")
}

// --- patchHostNetworkAPIAccess tests ---

func TestPatchHostNetworkAPIAccess_Deployment(t *testing.T) {
	t.Parallel()
	yaml := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-controller
spec:
  template:
    spec:
      containers:
      - name: controller
        image: my-image:latest
        env:
        - name: FOO
          value: bar
`)
	result, err := patchHostNetworkAPIAccess(yaml, "my-controller")
	require.NoError(t, err)

	resultStr := string(result)
	assert.Contains(t, resultStr, "KUBERNETES_SERVICE_HOST")
	assert.Contains(t, resultStr, "KUBERNETES_SERVICE_PORT")
	assert.Contains(t, resultStr, "localhost")
	assert.Contains(t, resultStr, "\"6443\"")
	// Original env should be preserved
	assert.Contains(t, resultStr, "FOO")
}

func TestPatchHostNetworkAPIAccess_DaemonSet(t *testing.T) {
	t.Parallel()
	yaml := []byte(`apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: my-daemon
spec:
  template:
    spec:
      containers:
      - name: agent
        image: agent:latest
`)
	result, err := patchHostNetworkAPIAccess(yaml, "my-daemon")
	require.NoError(t, err)

	resultStr := string(result)
	assert.Contains(t, resultStr, "KUBERNETES_SERVICE_HOST")
	assert.Contains(t, resultStr, "localhost")
}

func TestPatchHostNetworkAPIAccess_DeduplicatesExistingEnvVars(t *testing.T) {
	t.Parallel()
	yaml := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: talos-ccm
spec:
  template:
    spec:
      containers:
      - name: ccm
        image: ccm:latest
        env:
        - name: KUBERNETES_SERVICE_HOST
          value: old-host
        - name: KUBERNETES_SERVICE_PORT
          value: "8443"
        - name: OTHER_VAR
          value: keep
`)
	result, err := patchHostNetworkAPIAccess(yaml, "talos-ccm")
	require.NoError(t, err)

	resultStr := string(result)
	// Should have localhost, not old-host
	assert.Contains(t, resultStr, "localhost")
	assert.NotContains(t, resultStr, "old-host")
	// Should have 6443, not 8443
	assert.Contains(t, resultStr, "\"6443\"")
	assert.NotContains(t, resultStr, "\"8443\"")
	// OTHER_VAR should be preserved
	assert.Contains(t, resultStr, "OTHER_VAR")
	assert.Contains(t, resultStr, "keep")
	// Should NOT have duplicate KUBERNETES_SERVICE_HOST entries
	assert.Equal(t, 1, strings.Count(resultStr, "KUBERNETES_SERVICE_HOST"))
	assert.Equal(t, 1, strings.Count(resultStr, "KUBERNETES_SERVICE_PORT"))
}

func TestPatchHostNetworkAPIAccess_NotFound(t *testing.T) {
	t.Parallel()
	yaml := []byte(`apiVersion: v1
kind: Service
metadata:
  name: my-service
`)
	_, err := patchHostNetworkAPIAccess(yaml, "my-controller")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPatchHostNetworkAPIAccess_NoContainers(t *testing.T) {
	t.Parallel()
	yaml := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: empty-deploy
spec:
  template:
    spec: {}
`)
	_, err := patchHostNetworkAPIAccess(yaml, "empty-deploy")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no containers found")
}

func TestPatchDeploymentDNSPolicy_NonDeploymentKind(t *testing.T) {
	t.Parallel()
	// A DaemonSet named the same as the target should NOT be patched,
	// because patchDeploymentDNSPolicy only matches kind=Deployment.
	daemonSetYAML := `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: my-deploy
spec:
  template:
    spec:
      containers:
      - name: app
      dnsPolicy: ClusterFirst
`
	_, err := patchDeploymentDNSPolicy([]byte(daemonSetYAML), "my-deploy", "Default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in manifests")
}

func TestPatchManifestObjects_EmptyInput(t *testing.T) {
	t.Parallel()

	t.Run("nil input", func(t *testing.T) {
		t.Parallel()
		_, err := patchManifestObjects(nil, "test",
			func(_ *unstructured.Unstructured) bool { return true },
			func(_ *unstructured.Unstructured) error { return nil },
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in manifests")
	})

	t.Run("empty byte slice", func(t *testing.T) {
		t.Parallel()
		_, err := patchManifestObjects([]byte{}, "test",
			func(_ *unstructured.Unstructured) bool { return true },
			func(_ *unstructured.Unstructured) error { return nil },
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in manifests")
	})
}

func TestPatchManifestObjects_InvalidYAMLUnmarshal(t *testing.T) {
	t.Parallel()
	// Use YAML that the decoder can partially parse but will fail on unmarshal
	// into an Unstructured object - a bare scalar is not a valid k8s object.
	invalidYAML := []byte(":\n  - :\n    - [[[")

	_, err := patchManifestObjects(invalidYAML, "test",
		func(_ *unstructured.Unstructured) bool { return true },
		func(_ *unstructured.Unstructured) error { return nil },
	)
	require.Error(t, err)
}
