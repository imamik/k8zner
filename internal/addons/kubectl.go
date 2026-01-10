package addons

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
