package k8sclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestCreateSecret_Success(t *testing.T) {
	t.Parallel(
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	)

	clientset := fake.NewSimpleClientset()

	c := &client{clientset: clientset}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}

	err := c.CreateSecret(context.Background(), secret)
	require.NoError(t, err)

	// Verify secret was created
	created, err := clientset.CoreV1().Secrets("default").Get(context.Background(), "test-secret", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "test-secret", created.Name)
	assert.Equal(t, []byte("value"), created.Data["key"])
}

func TestCreateSecret_MissingNamespace(t *testing.T) {
	t.Parallel(
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	)

	clientset := fake.NewSimpleClientset()

	c := &client{clientset: clientset}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret",
			// Missing namespace
		},
	}

	err := c.CreateSecret(context.Background(), secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret namespace is required")
}

func TestCreateSecret_MissingName(t *testing.T) {
	t.Parallel(
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	)

	clientset := fake.NewSimpleClientset()

	c := &client{clientset: clientset}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			// Missing name
		},
	}

	err := c.CreateSecret(context.Background(), secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret name is required")
}

func TestCreateSecret_ReplacesExisting(t *testing.T) {
	t.Parallel(
	// Create existing secret
	)

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"old-key": []byte("old-value"),
		},
	}

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	clientset := fake.NewSimpleClientset(existingSecret)

	c := &client{clientset: clientset}

	// Create new secret with same name
	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"new-key": []byte("new-value"),
		},
	}

	err := c.CreateSecret(context.Background(), newSecret)
	require.NoError(t, err)

	// Verify secret was replaced
	created, err := clientset.CoreV1().Secrets("default").Get(context.Background(), "test-secret", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []byte("new-value"), created.Data["new-key"])
	_, hasOldKey := created.Data["old-key"]
	assert.False(t, hasOldKey, "old key should not exist in replaced secret")
}

func TestCreateSecret_DeleteError(t *testing.T) {
	t.Parallel(
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	)

	clientset := fake.NewSimpleClientset()

	// Add reactor to fail delete operations
	clientset.PrependReactor("delete", "secrets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.NewForbidden(corev1.Resource("secrets"), "test-secret", nil)
	})

	c := &client{clientset: clientset}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
	}

	err := c.CreateSecret(context.Background(), secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete existing secret")
}

func TestCreateSecret_CreateError(t *testing.T) {
	t.Parallel(
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	)

	clientset := fake.NewSimpleClientset()

	// Add reactor to fail create operations
	clientset.PrependReactor("create", "secrets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.NewServiceUnavailable("service unavailable")
	})

	c := &client{clientset: clientset}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
	}

	err := c.CreateSecret(context.Background(), secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create secret")
}

func TestDeleteSecret_Success(t *testing.T) {
	t.Parallel(
	// Create existing secret
	)

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
	}

	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	clientset := fake.NewSimpleClientset(existingSecret)

	c := &client{clientset: clientset}

	err := c.DeleteSecret(context.Background(), "default", "test-secret")
	require.NoError(t, err)

	// Verify secret was deleted
	_, err = clientset.CoreV1().Secrets("default").Get(context.Background(), "test-secret", metav1.GetOptions{})
	require.True(t, errors.IsNotFound(err))
}

func TestDeleteSecret_NotFound(t *testing.T) {
	t.Parallel(
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	)

	clientset := fake.NewSimpleClientset()

	c := &client{clientset: clientset}

	// Deleting non-existent secret should succeed
	err := c.DeleteSecret(context.Background(), "default", "nonexistent")
	require.NoError(t, err)
}

func TestDeleteSecret_MissingNamespace(t *testing.T) {
	t.Parallel(
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	)

	clientset := fake.NewSimpleClientset()

	c := &client{clientset: clientset}

	err := c.DeleteSecret(context.Background(), "", "test-secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required")
}

func TestDeleteSecret_MissingName(t *testing.T) {
	t.Parallel(
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	)

	clientset := fake.NewSimpleClientset()

	c := &client{clientset: clientset}

	err := c.DeleteSecret(context.Background(), "default", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret name is required")
}

func TestDeleteSecret_Error(t *testing.T) {
	t.Parallel(
	//nolint:staticcheck // SA1019: NewSimpleClientset is sufficient for our testing needs
	)

	clientset := fake.NewSimpleClientset()

	// Add reactor to fail delete operations
	clientset.PrependReactor("delete", "secrets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.NewForbidden(corev1.Resource("secrets"), "test-secret", nil)
	})

	c := &client{clientset: clientset}

	err := c.DeleteSecret(context.Background(), "default", "test-secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete secret")
}
