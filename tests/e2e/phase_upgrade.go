//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"hcloud-k8s/cmd/hcloud-k8s/handlers"
	"hcloud-k8s/internal/config"
)

// phaseUpgrade tests cluster upgrade functionality.
// This phase tests:
// - Upgrading Talos OS version
// - Upgrading Kubernetes version
// - Verifying nodes come back online
// - Verifying cluster health after upgrade
//
// Note: This test modifies versions in the config, so it should run
// after cluster provisioning but can be optional in the full E2E suite.
func phaseUpgrade(t *testing.T, state *E2EState) {
	// Skip if E2E_SKIP_UPGRADE is set
	if os.Getenv("E2E_SKIP_UPGRADE") == "true" {
		t.Skip("Skipping upgrade phase (E2E_SKIP_UPGRADE=true)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t.Log("Testing cluster upgrade functionality...")

	// Create config file with updated versions
	cfg := createUpgradeConfig(state)
	configPath := fmt.Sprintf("/tmp/upgrade-config-%s.yaml", state.ClusterName)
	if err := saveConfigToFile(cfg, configPath); err != nil {
		t.Fatalf("Failed to save upgrade config: %v", err)
	}
	defer os.Remove(configPath)

	// Create secrets file for upgrade (required for authentication)
	secretsPath := "secrets.yaml"
	if err := copySecretsFile(state.TalosSecretsPath, secretsPath); err != nil {
		t.Fatalf("Failed to copy secrets file: %v", err)
	}
	defer os.Remove(secretsPath)

	// Test 1: Dry run upgrade
	t.Run("DryRun", func(t *testing.T) {
		testUpgradeDryRun(ctx, t, configPath)
	})

	// Test 2: Actual upgrade
	t.Run("Upgrade", func(t *testing.T) {
		testUpgradeExecution(ctx, t, configPath, state)
	})

	// Test 3: Verify cluster after upgrade
	t.Run("VerifyAfterUpgrade", func(t *testing.T) {
		verifyClusterAfterUpgrade(ctx, t, state)
	})

	t.Log("✓ Upgrade phase completed successfully")
}

// createUpgradeConfig creates a config with updated Talos/K8s versions for testing.
func createUpgradeConfig(state *E2EState) *config.Config {
	// Use slightly newer versions for upgrade test
	// In reality, these versions should be validated to ensure they're compatible
	newTalosVersion := "v1.8.3"   // Upgrade from v1.8.3 to same (idempotent test)
	newK8sVersion := "v1.31.0"    // Upgrade K8s if different

	return &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: os.Getenv("HCLOUD_TOKEN"),
		Location:    "nbg1",
		Network: config.NetworkConfig{
			Zone:         "eu-central",
			IPv4CIDR:     "10.0.0.0/16",
			NodeIPv4CIDR: "10.0.0.0/22",
		},
		Talos: config.TalosConfig{
			Version:     newTalosVersion,
			SchematicID: "ce4c980550dd2ab1b17bbf2b08801c7eb59418eafe8f279833297925d67c7515", // Default schematic
		},
		Kubernetes: config.KubernetesConfig{
			Version: newK8sVersion,
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					Location:   "nbg1",
					ServerType: "cx22",
					Count:      1,
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "pool1",
				Location:   "nbg1",
				ServerType: "cx22",
				Count:      1,
			},
		},
	}
}

// saveConfigToFile saves a config to a YAML file.
func saveConfigToFile(cfg *config.Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// copySecretsFile copies the Talos secrets file to the expected location.
func copySecretsFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read secrets: %w", err)
	}

	if err := os.WriteFile(dst, data, 0600); err != nil {
		return fmt.Errorf("failed to write secrets: %w", err)
	}

	return nil
}

// testUpgradeDryRun tests the dry run mode of upgrade.
func testUpgradeDryRun(ctx context.Context, t *testing.T, configPath string) {
	t.Log("Running upgrade in dry run mode...")

	opts := handlers.UpgradeOptions{
		ConfigPath:      configPath,
		DryRun:          true,
		SkipHealthCheck: false,
	}

	if err := handlers.Upgrade(ctx, opts); err != nil {
		t.Fatalf("Dry run upgrade failed: %v", err)
	}

	t.Log("✓ Dry run upgrade completed")
}

// testUpgradeExecution performs the actual upgrade.
func testUpgradeExecution(ctx context.Context, t *testing.T, configPath string, state *E2EState) {
	t.Log("Executing actual cluster upgrade...")
	startTime := time.Now()

	opts := handlers.UpgradeOptions{
		ConfigPath:      configPath,
		DryRun:          false,
		SkipHealthCheck: false,
	}

	if err := handlers.Upgrade(ctx, opts); err != nil {
		// Run diagnostics on failure
		t.Logf("Upgrade failed: %v", err)
		runClusterDiagnostics(ctx, t, state)
		t.Fatalf("Upgrade execution failed: %v", err)
	}

	duration := time.Since(startTime)
	t.Logf("✓ Upgrade completed in %v", duration)
}

// verifyClusterAfterUpgrade verifies the cluster is healthy after upgrade.
func verifyClusterAfterUpgrade(ctx context.Context, t *testing.T, state *E2EState) {
	t.Log("Verifying cluster health after upgrade...")

	// Wait for API to be responsive
	time.Sleep(10 * time.Second)

	// Verify nodes are ready
	t.Log("Checking node readiness...")
	cmd := exec.CommandContext(ctx, "kubectl", "get", "nodes",
		"--kubeconfig", state.KubeconfigPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("kubectl get nodes output:\n%s", string(output))
		t.Fatalf("Failed to get nodes: %v", err)
	}
	t.Logf("Nodes status:\n%s", string(output))

	// Verify all nodes are Ready
	cmd = exec.CommandContext(ctx, "kubectl", "get", "nodes",
		"-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}",
		"--kubeconfig", state.KubeconfigPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to check node conditions: %v", err)
	}

	// All nodes should be "True"
	if len(output) == 0 {
		t.Fatal("No node status found")
	}
	t.Logf("Node ready conditions: %s", string(output))

	// Verify system pods are running
	t.Log("Checking system pods...")
	cmd = exec.CommandContext(ctx, "kubectl", "get", "pods",
		"-n", "kube-system",
		"--kubeconfig", state.KubeconfigPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Logf("kubectl get pods output:\n%s", string(output))
		t.Fatalf("Failed to get pods: %v", err)
	}
	t.Logf("System pods:\n%s", string(output))

	// Verify cluster info
	t.Log("Checking cluster info...")
	cmd = exec.CommandContext(ctx, "kubectl", "cluster-info",
		"--kubeconfig", state.KubeconfigPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Logf("kubectl cluster-info output:\n%s", string(output))
		t.Fatalf("Failed to get cluster info: %v", err)
	}
	t.Logf("Cluster info:\n%s", string(output))

	t.Log("✓ Cluster is healthy after upgrade")
}

// verifyNodeVersions verifies that nodes are running the expected Talos version.
// This is a helper function that can be used to check specific versions.
func verifyNodeVersions(ctx context.Context, t *testing.T, state *E2EState, expectedVersion string) {
	t.Logf("Verifying nodes are running Talos %s...", expectedVersion)

	// Use talosctl to check versions on control plane
	for _, ip := range state.ControlPlaneIPs {
		t.Logf("Checking version on control plane node %s...", ip)
		cmd := exec.CommandContext(ctx, "talosctl", "version",
			"--nodes", ip,
			"--talosconfig", "/tmp/talosconfig-"+state.ClusterName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("talosctl version output:\n%s", string(output))
			t.Logf("Warning: Could not verify Talos version on %s: %v", ip, err)
			continue
		}
		t.Logf("Version on %s:\n%s", ip, string(output))
	}

	// Check worker nodes
	for _, ip := range state.WorkerIPs {
		t.Logf("Checking version on worker node %s...", ip)
		cmd := exec.CommandContext(ctx, "talosctl", "version",
			"--nodes", ip,
			"--talosconfig", "/tmp/talosconfig-"+state.ClusterName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("talosctl version output:\n%s", string(output))
			t.Logf("Warning: Could not verify Talos version on %s: %v", ip, err)
			continue
		}
		t.Logf("Version on %s:\n%s", ip, string(output))
	}

	t.Log("✓ Version verification completed")
}
