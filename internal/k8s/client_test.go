package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestApplyManifest(t *testing.T) {
	// Setup fake clients
	scheme := runtime.NewScheme()
	fakeClientset := k8sfake.NewSimpleClientset()
	fakeDynamic := dynfake.NewSimpleDynamicClient(scheme)

	// Mock RESTMapper
	mockMapper := &mockRESTMapper{}

	client := &Client{
		Clientset: fakeClientset,
		Dynamic:   fakeDynamic,
		Mapper:    mockMapper,
	}

	// Pre-create the secret because fake dynamic client doesn't support SSA create-on-patch
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	fakeDynamic.Resource(gvr).Namespace("kube-system").Create(context.TODO(), &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "test-secret",
				"namespace": "kube-system",
			},
		},
	}, metav1.CreateOptions{})

	manifest := []byte(`
apiVersion: v1
kind: Secret
metadata:
  name: test-secret
  namespace: kube-system
type: Opaque
data:
  token: dGVzdC10b2tlbg==
`)

	ctx := context.Background()
	err := client.ApplyManifest(ctx, manifest)
	assert.NoError(t, err)
}

type mockRESTMapper struct {
	meta.RESTMapper
}

func (m *mockRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return &meta.RESTMapping{
		Resource:         schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"},
		GroupVersionKind: gk.WithVersion("v1"),
		Scope:            meta.RESTScopeNamespace,
	}, nil
}
