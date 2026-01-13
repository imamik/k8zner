package addons

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

// createHCloudSecret creates the hcloud secret that addon charts reference.
// This secret must exist before applying CCM or CSI addons.
func createHCloudSecret(ctx context.Context, kubeconfigPath, token string, networkID int64) error {
	// Delete existing secret if it exists (ignore errors)
	deleteCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"delete", "secret", "hcloud",
		"--namespace", "kube-system",
		"--ignore-not-found",
	)
	_ = deleteCmd.Run()

	// Create new secret
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"create", "secret", "generic", "hcloud",
		"--namespace", "kube-system",
		"--from-literal=token="+token,
		"--from-literal=network="+strconv.FormatInt(networkID, 10),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create hcloud secret: %w\nOutput: %s", err, output)
	}

	return nil
}
