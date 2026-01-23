package k8sclient

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateSecret creates or replaces a secret in the specified namespace.
// If the secret already exists, it will be deleted and recreated to ensure
// the data is exactly as specified (not merged).
func (c *client) CreateSecret(ctx context.Context, secret *corev1.Secret) error {
	if secret.Namespace == "" {
		return fmt.Errorf("secret namespace is required")
	}
	if secret.Name == "" {
		return fmt.Errorf("secret name is required")
	}

	secretsClient := c.clientset.CoreV1().Secrets(secret.Namespace)

	// Delete existing secret if it exists (ignore not found errors)
	err := secretsClient.Delete(ctx, secret.Name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete existing secret %s/%s: %w",
			secret.Namespace, secret.Name, err)
	}

	// Create the new secret
	_, err = secretsClient.Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create secret %s/%s: %w",
			secret.Namespace, secret.Name, err)
	}

	return nil
}

// DeleteSecret deletes a secret, returning nil if not found.
func (c *client) DeleteSecret(ctx context.Context, namespace, name string) error {
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if name == "" {
		return fmt.Errorf("secret name is required")
	}

	err := c.clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete secret %s/%s: %w", namespace, name, err)
	}

	return nil
}
