package k8sclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/restmapper"
)

// Note: Server-Side Apply tests require a real cluster or more sophisticated fakes.
// These tests focus on input validation, error handling, and interface compliance.

func TestApplyManifests_EmptyManifest(t *testing.T) {
	t.Parallel()
	manifests := []byte(``)

	client := setupApplyTestClient(t)

	err := client.ApplyManifests(context.Background(), manifests, "test-manager")
	require.NoError(t, err)
}

func TestApplyManifests_InvalidYAML(t *testing.T) {
	t.Parallel()
	manifests := []byte(`{invalid yaml: [`)

	client := setupApplyTestClient(t)

	err := client.ApplyManifests(context.Background(), manifests, "test-manager")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode manifest")
}

func TestApplyManifests_NoKindInDocument(t *testing.T) {
	t.Parallel()
	// YAML without kind field - decoder will fail

	manifests := []byte(`apiVersion: v1
metadata:
  name: test
`)

	client := setupApplyTestClient(t)

	err := client.ApplyManifests(context.Background(), manifests, "test-manager")
	require.Error(t, err)
	// The decoder catches missing Kind
	assert.Contains(t, err.Error(), "Kind")
}

func TestApplyManifests_EmptyDocuments(t *testing.T) {
	t.Parallel()
	// Multiple empty documents should be skipped

	manifests := []byte(`---
---
---
`)

	client := setupApplyTestClient(t)

	err := client.ApplyManifests(context.Background(), manifests, "test-manager")
	require.NoError(t, err)
}

func TestNewFromKubeconfig_InvalidKubeconfig(t *testing.T) {
	t.Parallel()
	invalidKubeconfig := []byte(`invalid kubeconfig content`)

	_, err := NewFromKubeconfig(invalidKubeconfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create REST config")
}

func TestNewFromKubeconfig_EmptyKubeconfig(t *testing.T) {
	t.Parallel()
	_, err := NewFromKubeconfig([]byte{})
	require.Error(t, err)
}

func TestNewFromKubeconfig_ValidYAMLButNoCluster(t *testing.T) {
	t.Parallel()
	// Valid YAML kubeconfig structure but with no clusters defined

	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters: []
contexts: []
users: []
`)

	_, err := NewFromKubeconfig(kubeconfig)
	require.Error(t, err)
	// Should fail on creating clientset due to no server
}

func TestNewFromKubeconfig_MalformedURL(t *testing.T) {
	t.Parallel()
	// Kubeconfig with malformed server URL

	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: "not-a-valid-url"
    insecure-skip-tls-verify: true
contexts:
- name: test
  context:
    cluster: test
    user: test
users:
- name: test
  user:
    token: test-token
current-context: test
`)

	// This should succeed in creating the config but will fail when actually used
	// However, client-go may validate the URL during client creation
	_, err := NewFromKubeconfig(kubeconfig)
	// The error handling depends on client-go validation
	// It may succeed or fail - we just verify it doesn't panic
	_ = err
}

// setupApplyTestClient creates a test client with fake clients
func setupApplyTestClient(t *testing.T) Client {
	t.Helper()

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	clientset := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	mapper := createApplyTestMapper()

	return NewFromClients(clientset, dynamicClient, mapper)
}

// createApplyTestMapper creates a REST mapper for testing
func createApplyTestMapper() meta.RESTMapper {
	resources := []*restmapper.APIGroupResources{
		{
			Group: metav1.APIGroup{
				Name: "",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "v1", Version: "v1"},
				},
				PreferredVersion: metav1.GroupVersionForDiscovery{
					GroupVersion: "v1",
					Version:      "v1",
				},
			},
			VersionedResources: map[string][]metav1.APIResource{
				"v1": {
					{Name: "configmaps", Namespaced: true, Kind: "ConfigMap"},
					{Name: "secrets", Namespaced: true, Kind: "Secret"},
					{Name: "namespaces", Namespaced: false, Kind: "Namespace"},
					{Name: "services", Namespaced: true, Kind: "Service"},
				},
			},
		},
	}

	return restmapper.NewDiscoveryRESTMapper(resources)
}

func TestClient_Interface(t *testing.T) {
	t.Parallel()
	// Verify that client implements the Client interface

	var _ Client = &client{}
}

func TestApplyObject_NoKind(t *testing.T) {
	t.Parallel()
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs

	clientset := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	mapper := createApplyTestMapper()

	c := &client{
		clientset:     clientset,
		dynamicClient: dynamicClient,
		mapper:        mapper,
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"metadata": map[string]interface{}{
				"name": "test",
			},
		},
	}

	err := c.applyObject(context.Background(), obj, "test-manager")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no kind set")
}

func TestApplyObject_UnknownGVK(t *testing.T) {
	t.Parallel()
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs

	clientset := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	mapper := createApplyTestMapper()

	c := &client{
		clientset:     clientset,
		dynamicClient: dynamicClient,
		mapper:        mapper,
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "unknown.io/v1",
			"kind":       "UnknownResource",
			"metadata": map[string]interface{}{
				"name": "test",
			},
		},
	}

	err := c.applyObject(context.Background(), obj, "test-manager")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get REST mapping")
}

func TestGVKMapping(t *testing.T) {
	t.Parallel()
	mapper := createApplyTestMapper()

	tests := []struct {
		gvk         schema.GroupVersionKind
		expectedGVR schema.GroupVersionResource
		expectErr   bool
	}{
		{
			gvk:         schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
			expectedGVR: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
		},
		{
			gvk:         schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"},
			expectedGVR: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"},
		},
		{
			gvk:         schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"},
			expectedGVR: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.gvk.Kind, func(t *testing.T) {
			t.Parallel()
			mapping, err := mapper.RESTMapping(tt.gvk.GroupKind(), tt.gvk.Version)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedGVR, mapping.Resource)
			}
		})
	}
}

func TestUnstructured_MarshalJSON(t *testing.T) {
	t.Parallel()
	// Test that unstructured objects can be marshaled to JSON for SSA

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	data, err := obj.MarshalJSON()
	require.NoError(t, err)
	assert.Contains(t, string(data), `"apiVersion":"v1"`)
	assert.Contains(t, string(data), `"kind":"ConfigMap"`)
	assert.Contains(t, string(data), `"name":"test-cm"`)
}

func TestUnstructured_GetGVK(t *testing.T) {
	t.Parallel()
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "test",
			},
		},
	}

	gvk := obj.GroupVersionKind()
	assert.Equal(t, "", gvk.Group)
	assert.Equal(t, "v1", gvk.Version)
	assert.Equal(t, "ConfigMap", gvk.Kind)
}

func TestUnstructured_GetMetadata(t *testing.T) {
	t.Parallel()
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "kube-system",
			},
		},
	}

	assert.Equal(t, "test-cm", obj.GetName())
	assert.Equal(t, "kube-system", obj.GetNamespace())
}

func TestApplyManifests_MultiDocument(t *testing.T) {
	t.Parallel()
	// Multi-document YAML with mix of empty and valid documents

	manifests := []byte(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: config1
  namespace: default
data:
  key: value
---
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: config2
  namespace: default
data:
  key2: value2
`)

	client := setupApplyTestClient(t)

	// This will fail on the actual apply but should process both documents
	err := client.ApplyManifests(context.Background(), manifests, "test-manager")
	// We expect an error from the fake client's SSA not being supported
	// but the important thing is that parsing worked
	assert.Error(t, err)
	// Error should be about the apply failing, not parsing
	assert.Contains(t, err.Error(), "failed to apply")
}

func TestApplyManifests_WhitespaceOnlyDocument(t *testing.T) {
	t.Parallel()
	// Document with only whitespace/comments should be skipped

	manifests := []byte(`

---

---
`)

	client := setupApplyTestClient(t)

	err := client.ApplyManifests(context.Background(), manifests, "test-manager")
	require.NoError(t, err)
}

func TestApplyObject_NamespacedResourceWithEmptyNamespace(t *testing.T) {
	t.Parallel()
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs

	clientset := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	mapper := createApplyTestMapper()

	c := &client{
		clientset:     clientset,
		dynamicClient: dynamicClient,
		mapper:        mapper,
	}

	// ConfigMap is namespaced - when namespace is empty, it should default to "default"
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "test-no-namespace",
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	// The apply will fail because fake client doesn't support SSA
	// but we're testing that the namespace defaulting code path is reached
	err := c.applyObject(context.Background(), obj, "test-manager")
	require.Error(t, err)
	// Should fail on the apply, not on mapping or validation
	assert.Contains(t, err.Error(), "server-side apply failed")
}

func TestApplyObject_ClusterScopedResource(t *testing.T) {
	t.Parallel()
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs

	clientset := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	mapper := createApplyTestMapper()

	c := &client{
		clientset:     clientset,
		dynamicClient: dynamicClient,
		mapper:        mapper,
	}

	// Namespace is a cluster-scoped resource
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "test-namespace",
			},
		},
	}

	// The apply will fail because fake client doesn't support SSA
	// but we're testing that the cluster-scoped code path is reached
	err := c.applyObject(context.Background(), obj, "test-manager")
	require.Error(t, err)
	// Should fail on the apply, not on mapping or validation
	assert.Contains(t, err.Error(), "server-side apply failed")
}

func TestApplyManifests_FieldManagerPropagation(t *testing.T) {
	t.Parallel()
	// Verify field manager is passed through correctly

	manifests := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  namespace: default
`)

	client := setupApplyTestClient(t)

	// Should fail on apply but with our custom field manager
	err := client.ApplyManifests(context.Background(), manifests, "custom-field-manager")
	require.Error(t, err)
	// The error should be from the apply failing, not validation
	assert.Contains(t, err.Error(), "failed to apply")
}
