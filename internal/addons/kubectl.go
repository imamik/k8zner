package addons

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

	// Use server-side apply to handle namespace creation race conditions
	// Skip client-side validation since server-side apply does server validation
	// Retry on temporary connection failures (API server might be briefly unavailable)
	// See: https://kubernetes.io/docs/reference/using-api/server-side-apply/
	maxRetries := 5
	retryDelay := 5 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// #nosec G204 - kubeconfigPath is from internal config, tmpfile.Name() is a secure temp file we created
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "--server-side", "--force-conflicts", "--validate=false", "-f", tmpfile.Name())
		output, err := cmd.CombinedOutput()

		if err == nil {
			return nil // Success
		}

		// Check if error is retryable (connection issues, EOF)
		outputStr := string(output)
		isRetryable := strings.Contains(outputStr, "EOF") ||
		              strings.Contains(outputStr, "connection refused") ||
		              strings.Contains(outputStr, "Unable to connect") ||
		              strings.Contains(outputStr, "connection reset")

		if !isRetryable || attempt == maxRetries {
			return fmt.Errorf("kubectl apply failed for addon %s: %w\nOutput: %s", addonName, err, outputStr)
		}

		// Wait before retry
		time.Sleep(retryDelay)
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
