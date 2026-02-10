package k8sclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCreateSecret(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset()

	// Create client with fake clientset (dynamic and mapper not needed for secret ops)
	c := &client{
		clientset: fakeClientset,
	}

	// Create a test secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}

	// Test creating a new secret
	err := c.CreateSecret(ctx, secret)
	require.NoError(t, err)

	// Verify secret was created
	created, err := fakeClientset.CoreV1().Secrets("default").Get(ctx, "test-secret", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "test-secret", created.Name)
	assert.Equal(t, []byte("value"), created.Data["key"])

	// Test recreating (should delete and create)
	secret.Data["key"] = []byte("new-value")
	err = c.CreateSecret(ctx, secret)
	require.NoError(t, err)

	// Verify secret was updated
	updated, err := fakeClientset.CoreV1().Secrets("default").Get(ctx, "test-secret", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []byte("new-value"), updated.Data["key"])
}

func TestCreateSecret_ValidationErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset()

	c := &client{
		clientset: fakeClientset,
	}

	// Test missing namespace
	err := c.CreateSecret(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required")

	// Test missing name
	err = c.CreateSecret(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestDeleteSecret(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Create a fake clientset with an existing secret
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-secret",
			Namespace: "kube-system",
		},
	}
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset(existingSecret)

	c := &client{
		clientset: fakeClientset,
	}

	// Test deleting existing secret
	err := c.DeleteSecret(ctx, "kube-system", "existing-secret")
	require.NoError(t, err)

	// Verify secret was deleted
	_, err = fakeClientset.CoreV1().Secrets("kube-system").Get(ctx, "existing-secret", metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err))

	// Test deleting non-existent secret (should not error)
	err = c.DeleteSecret(ctx, "kube-system", "non-existent")
	require.NoError(t, err)
}

func TestDeleteSecret_ValidationErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset()

	c := &client{
		clientset: fakeClientset,
	}

	// Test missing namespace
	err := c.DeleteSecret(ctx, "", "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required")

	// Test missing name
	err = c.DeleteSecret(ctx, "default", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestNewFromClients(t *testing.T) {
	t.Parallel()
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs

	fakeClientset := fake.NewSimpleClientset()

	// Test that NewFromClients returns a valid client
	c := NewFromClients(fakeClientset, nil, nil)
	assert.NotNil(t, c)

	// Verify it can perform secret operations
	ctx := context.Background()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
	err := c.CreateSecret(ctx, secret)
	assert.NoError(t, err)
}

// --- splitCRDName tests ---

func TestSplitCRDName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "group/version format",
			input:    "talos.dev/v1alpha1",
			expected: []string{"talos.dev", "v1alpha1"},
		},
		{
			name:     "group/version/kind format",
			input:    "cert-manager.io/v1/ClusterIssuer",
			expected: []string{"cert-manager.io", "v1", "ClusterIssuer"},
		},
		{
			name:     "single part (no slash)",
			input:    "configmaps",
			expected: []string{"configmaps"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
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

// --- HasReadyEndpoints tests ---

func TestHasReadyEndpoints_WithReadyAddresses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "default",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.0.0.1"},
				},
			},
		},
	}

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset(endpoints)
	c := &client{clientset: fakeClientset}

	ready, err := c.HasReadyEndpoints(ctx, "default", "my-service")
	require.NoError(t, err)
	assert.True(t, ready)
}

func TestHasReadyEndpoints_NoReadyAddresses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "default",
		},
		Subsets: []corev1.EndpointSubset{
			{
				NotReadyAddresses: []corev1.EndpointAddress{
					{IP: "10.0.0.1"},
				},
			},
		},
	}

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset(endpoints)
	c := &client{clientset: fakeClientset}

	ready, err := c.HasReadyEndpoints(ctx, "default", "my-service")
	require.NoError(t, err)
	assert.False(t, ready)
}

func TestHasReadyEndpoints_ServiceNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset()
	c := &client{clientset: fakeClientset}

	ready, err := c.HasReadyEndpoints(ctx, "default", "nonexistent")
	require.NoError(t, err)
	assert.False(t, ready)
}

func TestHasReadyEndpoints_EmptySubsets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "default",
		},
		Subsets: []corev1.EndpointSubset{},
	}

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset(endpoints)
	c := &client{clientset: fakeClientset}

	ready, err := c.HasReadyEndpoints(ctx, "default", "my-service")
	require.NoError(t, err)
	assert.False(t, ready)
}

func TestHasReadyEndpoints_MultipleSubsets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "kube-system",
		},
		Subsets: []corev1.EndpointSubset{
			{
				// First subset has no ready addresses
				NotReadyAddresses: []corev1.EndpointAddress{
					{IP: "10.0.0.1"},
				},
			},
			{
				// Second subset has ready addresses
				Addresses: []corev1.EndpointAddress{
					{IP: "10.0.0.2"},
				},
			},
		},
	}

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset(endpoints)
	c := &client{clientset: fakeClientset}

	ready, err := c.HasReadyEndpoints(ctx, "kube-system", "my-service")
	require.NoError(t, err)
	assert.True(t, ready)
}

// --- RefreshDiscovery tests ---

func TestRefreshDiscovery_EmptyKubeconfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Test client (no kubeconfig) should skip refresh
	c := &client{}
	err := c.RefreshDiscovery(ctx)
	require.NoError(t, err)
}

// --- HasCRD tests ---

func TestHasCRD_TestClient(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Test client (no kubeconfig) should assume CRDs exist
	c := &client{}
	found, err := c.HasCRD(ctx, "talos.dev/v1alpha1")
	require.NoError(t, err)
	assert.True(t, found)
}

// --- HasIngressClass tests ---

func TestHasIngressClass_TestClient(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Test client (no kubeconfig) should assume IngressClass exists
	c := &client{}
	found, err := c.HasIngressClass(ctx, "traefik")
	require.NoError(t, err)
	assert.True(t, found)
}

func TestHasIngressClass_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset()
	c := &client{
		clientset:  fakeClientset,
		kubeconfig: []byte("fake-kubeconfig"), // Non-empty to bypass test client shortcut
	}

	found, err := c.HasIngressClass(ctx, "traefik")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestHasIngressClass_Found(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ingressClass := &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "traefik",
		},
	}

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	fakeClientset := fake.NewSimpleClientset(ingressClass)
	c := &client{
		clientset:  fakeClientset,
		kubeconfig: []byte("fake-kubeconfig"),
	}

	found, err := c.HasIngressClass(ctx, "traefik")
	require.NoError(t, err)
	assert.True(t, found)
}
