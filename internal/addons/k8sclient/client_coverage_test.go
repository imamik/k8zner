package k8sclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// --- RefreshDiscovery tests for non-empty kubeconfig paths ---

func TestRefreshDiscovery_InvalidKubeconfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	c := &client{
		kubeconfig: []byte("invalid kubeconfig content"),
	}

	err := c.RefreshDiscovery(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create REST config")
}

func TestRefreshDiscovery_WithFakeKubeAPIServer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Set up a fake API server that returns discovery info
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIVersions{
			TypeMeta: metav1.TypeMeta{Kind: "APIVersions"},
			Versions: []string{"v1"},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIGroupList{
			TypeMeta: metav1.TypeMeta{Kind: "APIGroupList"},
			Groups:   []metav1.APIGroup{},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIResourceList{
			TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "configmaps", Namespaced: true, Kind: "ConfigMap"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	kubeconfig := buildTestKubeconfig(server.URL)

	c := &client{
		kubeconfig: kubeconfig,
	}

	// RefreshDiscovery should succeed against our fake server
	err := c.RefreshDiscovery(ctx)
	require.NoError(t, err)
	assert.NotNil(t, c.mapper)
}

// --- HasCRD tests with httptest server ---

func TestHasCRD_InvalidKubeconfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	c := &client{
		kubeconfig: []byte("invalid kubeconfig content"),
	}

	found, err := c.HasCRD(ctx, "talos.dev/v1alpha1")
	require.Error(t, err)
	assert.False(t, found)
	assert.Contains(t, err.Error(), "failed to create REST config")
}

func TestHasCRD_InvalidCRDNameFormat(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Create a fake API server for the discovery call
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIVersions{
			TypeMeta: metav1.TypeMeta{Kind: "APIVersions"},
			Versions: []string{"v1"},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIGroupList{
			TypeMeta: metav1.TypeMeta{Kind: "APIGroupList"},
			Groups:   []metav1.APIGroup{},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIResourceList{
			TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	kubeconfig := buildTestKubeconfig(server.URL)
	c := &client{
		kubeconfig: kubeconfig,
	}

	// Single part is invalid (needs at least group/version)
	found, err := c.HasCRD(ctx, "configmaps")
	require.Error(t, err)
	assert.False(t, found)
	assert.Contains(t, err.Error(), "invalid CRD name format")
}

func TestHasCRD_GroupVersionFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIVersions{
			TypeMeta: metav1.TypeMeta{Kind: "APIVersions"},
			Versions: []string{"v1"},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIGroupList{
			TypeMeta: metav1.TypeMeta{Kind: "APIGroupList"},
			Groups: []metav1.APIGroup{
				{
					Name: "cert-manager.io",
					Versions: []metav1.GroupVersionForDiscovery{
						{GroupVersion: "cert-manager.io/v1", Version: "v1"},
					},
					PreferredVersion: metav1.GroupVersionForDiscovery{
						GroupVersion: "cert-manager.io/v1",
						Version:      "v1",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIResourceList{
			TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "configmaps", Namespaced: true, Kind: "ConfigMap"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/apis/cert-manager.io/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIResourceList{
			TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
			GroupVersion: "cert-manager.io/v1",
			APIResources: []metav1.APIResource{
				{Name: "clusterissuers", Namespaced: false, Kind: "ClusterIssuer"},
				{Name: "certificates", Namespaced: true, Kind: "Certificate"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	kubeconfig := buildTestKubeconfig(server.URL)
	c := &client{
		kubeconfig: kubeconfig,
	}

	// Test finding just the group/version (no kind)
	found, err := c.HasCRD(ctx, "cert-manager.io/v1")
	require.NoError(t, err)
	assert.True(t, found)
}

func TestHasCRD_GroupVersionKindFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIVersions{
			TypeMeta: metav1.TypeMeta{Kind: "APIVersions"},
			Versions: []string{"v1"},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIGroupList{
			TypeMeta: metav1.TypeMeta{Kind: "APIGroupList"},
			Groups: []metav1.APIGroup{
				{
					Name: "cert-manager.io",
					Versions: []metav1.GroupVersionForDiscovery{
						{GroupVersion: "cert-manager.io/v1", Version: "v1"},
					},
					PreferredVersion: metav1.GroupVersionForDiscovery{
						GroupVersion: "cert-manager.io/v1",
						Version:      "v1",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIResourceList{
			TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/apis/cert-manager.io/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIResourceList{
			TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
			GroupVersion: "cert-manager.io/v1",
			APIResources: []metav1.APIResource{
				{Name: "clusterissuers", Namespaced: false, Kind: "ClusterIssuer"},
				{Name: "certificates", Namespaced: true, Kind: "Certificate"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	kubeconfig := buildTestKubeconfig(server.URL)
	c := &client{
		kubeconfig: kubeconfig,
	}

	// Test finding a specific kind
	found, err := c.HasCRD(ctx, "cert-manager.io/v1/ClusterIssuer")
	require.NoError(t, err)
	assert.True(t, found)

	// Test not finding a specific kind
	found, err = c.HasCRD(ctx, "cert-manager.io/v1/NonExistentKind")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestHasCRD_GroupVersionNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIVersions{
			TypeMeta: metav1.TypeMeta{Kind: "APIVersions"},
			Versions: []string{"v1"},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIGroupList{
			TypeMeta: metav1.TypeMeta{Kind: "APIGroupList"},
			Groups:   []metav1.APIGroup{},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := metav1.APIResourceList{
			TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	kubeconfig := buildTestKubeconfig(server.URL)
	c := &client{
		kubeconfig: kubeconfig,
	}

	// Test that nonexistent group/version returns false
	found, err := c.HasCRD(ctx, "nonexistent.io/v1")
	require.NoError(t, err)
	assert.False(t, found)
}

// --- Additional splitCRDName edge case tests ---

func TestSplitCRDName_AdditionalCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "trailing slash",
			input:    "talos.dev/v1alpha1/",
			expected: []string{"talos.dev", "v1alpha1"},
		},
		{
			name:     "four parts",
			input:    "a/b/c/d",
			expected: []string{"a", "b", "c", "d"},
		},
		{
			name:     "single slash",
			input:    "/",
			expected: []string{""},
		},
		{
			name:     "leading slash",
			input:    "/v1/ConfigMap",
			expected: []string{"", "v1", "ConfigMap"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := splitCRDName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- HasReadyEndpoints with API error ---

func TestHasReadyEndpoints_APIError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset()
	fakeClientset.PrependReactor("get", "endpoints", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("server error")
	})
	c := &client{clientset: fakeClientset}

	// API error should return false, nil (code treats it as "not found yet")
	ready, err := c.HasReadyEndpoints(ctx, "default", "my-service")
	require.NoError(t, err)
	assert.False(t, ready)
}

// --- HasIngressClass edge cases ---

func TestHasIngressClass_DifferentNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		className  string
		expectFind bool
	}{
		{
			name:       "nginx class not found",
			className:  "nginx",
			expectFind: false,
		},
		{
			name:       "empty string class",
			className:  "",
			expectFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
			fakeClientset := fake.NewSimpleClientset()
			c := &client{
				clientset:  fakeClientset,
				kubeconfig: []byte("fake-kubeconfig"),
			}

			found, err := c.HasIngressClass(ctx, tt.className)
			if tt.className == "" {
				// Empty name is still a valid request, just returns not found
				require.NoError(t, err)
			}
			assert.Equal(t, tt.expectFind, found)
		})
	}
}

// --- NewFromClients with all parameters ---

func TestNewFromClients_VerifyFields(t *testing.T) {
	t.Parallel()

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset()
	mapper := createApplyTestMapper()

	c := NewFromClients(fakeClientset, nil, mapper)
	require.NotNil(t, c)

	// Verify RefreshDiscovery works on test client (no kubeconfig)
	err := c.RefreshDiscovery(context.Background())
	require.NoError(t, err)

	// Verify HasCRD returns true for test client
	found, err := c.HasCRD(context.Background(), "anything/v1")
	require.NoError(t, err)
	assert.True(t, found)

	// Verify HasIngressClass returns true for test client
	found, err = c.HasIngressClass(context.Background(), "traefik")
	require.NoError(t, err)
	assert.True(t, found)
}

// --- CreateSecret with multiple operations ---

func TestCreateSecret_MultipleSecretsInDifferentNamespaces(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset()
	c := &client{clientset: fakeClientset}

	secrets := []*corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "secret-1", Namespace: "ns-a"},
			Data:       map[string][]byte{"key": []byte("val-a")},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "secret-1", Namespace: "ns-b"},
			Data:       map[string][]byte{"key": []byte("val-b")},
		},
	}

	for _, s := range secrets {
		err := c.CreateSecret(ctx, s)
		require.NoError(t, err)
	}

	// Verify both exist independently
	sa, err := fakeClientset.CoreV1().Secrets("ns-a").Get(ctx, "secret-1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []byte("val-a"), sa.Data["key"])

	sb, err := fakeClientset.CoreV1().Secrets("ns-b").Get(ctx, "secret-1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []byte("val-b"), sb.Data["key"])
}

// --- ApplyManifests with error in second document ---

func TestApplyManifests_ErrorInSecondDocument(t *testing.T) {
	t.Parallel()
	// First document is valid but empty, second has invalid reference
	manifests := []byte(`---
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  namespace: default
`)

	client := setupApplyTestClient(t)

	err := client.ApplyManifests(context.Background(), manifests, "test-manager")
	// The first empty doc is skipped, the second hits SSA failure
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to apply")
}

// --- buildTestKubeconfig generates a kubeconfig pointing to an httptest server ---

func buildTestKubeconfig(serverURL string) []byte {
	return []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: %s
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
`, serverURL))
}
