package k8s

import (
	"context"
	"encoding/base64"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretExists checks if a secret exists in the given namespace.
func (c *Client) SecretExists(ctx context.Context, namespace, name string) (bool, error) {
	_, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, nil
	}
	return true, nil
}

// GetSecretData retrieves data from a secret.
func (c *Client) GetSecretData(ctx context.Context, namespace, name, key string) ([]byte, error) {
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	data, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("key %s not found in secret", key)
	}

	return data, nil
}

// DeleteSecret deletes a secret from the given namespace.
func (c *Client) DeleteSecret(ctx context.Context, namespace, name string) error {
	err := c.clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	return nil
}

// EncodeBase64 encodes a string to base64.
func EncodeBase64(data string) string {
	return base64.StdEncoding.EncodeToString([]byte(data))
}

// DecodeBase64 decodes a base64 string.
func DecodeBase64(encoded string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}
	return string(decoded), nil
}
