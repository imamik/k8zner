// Package k8sclient provides a Kubernetes client for addon installation.
//
// This package wraps k8s.io/client-go to provide a simple interface for:
//   - Applying multi-document YAML manifests using Server-Side Apply
//   - Creating and deleting Kubernetes secrets
//
// The client is designed to work directly with kubeconfig bytes, eliminating
// the need for temporary files or external CLI tools like kubectl.
//
// Example usage:
//
//	client, err := k8sclient.NewFromKubeconfig(kubeconfigBytes)
//	if err != nil {
//	    return err
//	}
//
//	// Apply manifests
//	err = client.ApplyManifests(ctx, manifestYAML, "my-addon")
//
//	// Create a secret
//	secret := &corev1.Secret{...}
//	err = client.CreateSecret(ctx, secret)
package k8sclient
