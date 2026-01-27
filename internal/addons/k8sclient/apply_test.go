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
	manifests := []byte(``)

	client := setupApplyTestClient(t)

	err := client.ApplyManifests(context.Background(), manifests, "test-manager")
	require.NoError(t, err)
}

func TestApplyManifests_InvalidYAML(t *testing.T) {
	manifests := []byte(`{invalid yaml: [`)

	client := setupApplyTestClient(t)

	err := client.ApplyManifests(context.Background(), manifests, "test-manager")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode manifest")
}

func TestApplyManifests_NoKindInDocument(t *testing.T) {
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
	invalidKubeconfig := []byte(`invalid kubeconfig content`)

	_, err := NewFromKubeconfig(invalidKubeconfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create REST config")
}

func TestNewFromKubeconfig_EmptyKubeconfig(t *testing.T) {
	_, err := NewFromKubeconfig([]byte{})
	require.Error(t, err)
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
	// Verify that client implements the Client interface
	var _ Client = &client{}
}

func TestApplyObject_NoKind(t *testing.T) {
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
