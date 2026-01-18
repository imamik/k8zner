package addons

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// applyWithKubectl applies Kubernetes manifests using kubectl.
// It writes the manifests to a temporary file, executes kubectl apply,
// and cleans up the temporary file.
func applyWithKubectl(ctx context.Context, kubeconfigPath, addonName string, manifestBytes []byte) error {
	tmpfile, err := os.CreateTemp("", fmt.Sprintf("%s-*.yaml", addonName))
	if err != nil {
		return fmt.Errorf("failed to create temp manifest file: %w", err)
	}
	// Best-effort cleanup; failure to remove temp file is non-critical
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	if _, err := tmpfile.Write(manifestBytes); err != nil {
		_ = tmpfile.Close()
		return fmt.Errorf("failed to write manifest to temp file: %w", err)
	}
	if err := tmpfile.Close(); err != nil {
		return fmt.Errorf("failed to close temp manifest file: %w", err)
	}

	// #nosec G204 - kubeconfigPath is from internal config, tmpfile.Name() is a secure temp file we created
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", tmpfile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed for addon %s: %w\nOutput: %s", addonName, err, string(output))
	}

	return nil
}

// writeTempKubeconfig writes kubeconfig to a temporary file and returns the path.
func writeTempKubeconfig(kubeconfig []byte) (string, error) {
	tmpfile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp kubeconfig: %w", err)
	}

	if _, err := tmpfile.Write(kubeconfig); err != nil {
		_ = tmpfile.Close()
		_ = os.Remove(tmpfile.Name())
		return "", fmt.Errorf("failed to write temp kubeconfig: %w", err)
	}

	if err := tmpfile.Close(); err != nil {
		_ = os.Remove(tmpfile.Name())
		return "", fmt.Errorf("failed to close temp kubeconfig: %w", err)
	}

	return tmpfile.Name(), nil
}

// waitForSecret waits for a Kubernetes secret to exist in the specified namespace.
// This is useful when we need to wait for cert-manager to create a certificate secret
// before proceeding with resources that depend on it.
func waitForSecret(ctx context.Context, kubeconfigPath, namespace, secretName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if secret exists using kubectl
		// #nosec G204 - kubeconfigPath is from internal config, namespace/secretName are controlled strings
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"get", "secret", secretName, "-n", namespace, "-o", "name")
		output, err := cmd.CombinedOutput()
		if err == nil && len(output) > 0 {
			// Secret exists
			return nil
		}

		// Secret doesn't exist yet, wait and retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
			// Continue polling
		}
	}

	return fmt.Errorf("timeout waiting for secret %s/%s after %v", namespace, secretName, timeout)
}

// waitForDeploymentReady waits for a Kubernetes deployment to have all replicas ready.
// This is useful when we need to ensure a service (like cert-manager webhook) is fully
// operational before proceeding with dependent resources.
func waitForDeploymentReady(ctx context.Context, kubeconfigPath, namespace, deploymentName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 5 * time.Second

	for time.Now().Before(deadline) {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if deployment is ready using kubectl rollout status
		// #nosec G204 - kubeconfigPath is from internal config, namespace/deploymentName are controlled strings
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", "deployment/"+deploymentName, "-n", namespace, "--timeout=5s")
		_, err := cmd.CombinedOutput()
		if err == nil {
			// Deployment is ready
			return nil
		}

		// Deployment not ready yet, wait and retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
			// Continue polling
		}
	}

	return fmt.Errorf("timeout waiting for deployment %s/%s to be ready after %v", namespace, deploymentName, timeout)
}

// applyFromURL downloads a manifest from a URL and applies it using kubectl.
// This is useful for applying CRDs or other manifests hosted remotely.
func applyFromURL(ctx context.Context, kubeconfigPath, addonName, manifestURL string) error {
	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", manifestURL, err)
	}

	// Download the manifest
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download manifest from %s: %w", manifestURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download manifest from %s: HTTP %d", manifestURL, resp.StatusCode)
	}

	// Read the manifest content
	manifestBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read manifest from %s: %w", manifestURL, err)
	}

	// Apply using the existing helper
	return applyWithKubectl(ctx, kubeconfigPath, addonName, manifestBytes)
}
