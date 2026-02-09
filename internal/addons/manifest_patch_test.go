package addons

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
