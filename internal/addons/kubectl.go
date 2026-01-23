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
