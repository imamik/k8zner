//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
)

// phaseOperatorScale tests scaling the cluster via the K8znerCluster CRD.
// This validates the operator-centric architecture where scaling is driven
// by changes to the CRD spec rather than CLI commands.
//
// Prerequisites:
// - Cluster must be running with at least 1 worker
// - HCLOUD_TOKEN environment variable must be set
// - Talos snapshot must exist with label os=talos
func phaseOperatorScale(t *testing.T, state *E2EState) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t.Log("=== OPERATOR-CENTRIC SCALING E2E TEST ===")

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Fatal("HCLOUD_TOKEN required for operator scaling test")
	}

	// Step 1: Deploy the k8zner-operator if not already deployed
	t.Run("DeployOperator", func(t *testing.T) {
		if state.AddonsInstalled["k8zner-operator"] {
			t.Log("Operator already deployed, skipping...")
			return
		}
		deployOperatorForScaling(t, state, token)
	})

	// Step 2: Create initial K8znerCluster CRD
	initialWorkerCount := len(state.WorkerIPs)
	t.Run("CreateInitialCRD", func(t *testing.T) {
		createK8znerClusterForScaling(t, state, initialWorkerCount)
	})

	// Step 3: Verify operator is reconciling
	t.Run("VerifyOperatorReady", func(t *testing.T) {
		waitForOperatorReconciling(ctx, t, state)
	})

	// Step 4: Scale UP workers via CRD patch
	scaledUpCount := initialWorkerCount + 1
	t.Run("ScaleUpWorkers", func(t *testing.T) {
		testScaleUpViaCRD(ctx, t, state, scaledUpCount)
	})

	// Step 5: Scale DOWN workers via CRD patch
	t.Run("ScaleDownWorkers", func(t *testing.T) {
		testScaleDownViaCRD(ctx, t, state, initialWorkerCount)
	})

	// Step 6: Cleanup
	t.Run("Cleanup", func(t *testing.T) {
		cleanupK8znerClusterForScaling(t, state)
	})

	t.Log("=== OPERATOR-CENTRIC SCALING E2E TEST PASSED ===")
}

// deployOperatorForScaling deploys the k8zner-operator addon.
func deployOperatorForScaling(t *testing.T, state *E2EState, token string) {
	t.Log("Deploying k8zner-operator addon for scaling test...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	network, _ := state.Client.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: token,
		Addons: config.AddonsConfig{
			Operator: config.OperatorConfig{
				Enabled: true,
				Version: "main",
			},
		},
	}

	if err := addons.Apply(ctx, cfg, state.Kubeconfig, networkID); err != nil {
		t.Fatalf("Failed to deploy operator: %v", err)
	}

	waitForPod(t, state.KubeconfigPath, "k8zner-system", "app.kubernetes.io/name=k8zner-operator", 5*time.Minute)

	t.Log("Operator deployed and running")
	state.AddonsInstalled["k8zner-operator"] = true
}

// createK8znerClusterForScaling creates a K8znerCluster CRD for scaling tests.
func createK8znerClusterForScaling(t *testing.T, state *E2EState, workerCount int) {
	t.Logf("Creating K8znerCluster CRD (workers: %d)...", workerCount)

	cpCount := len(state.ControlPlaneIPs)

	// Note: We no longer need SSH keys annotation since we use ephemeral keys
	manifest := fmt.Sprintf(`apiVersion: k8zner.io/v1alpha1
kind: K8znerCluster
metadata:
  name: %s
  namespace: k8zner-system
spec:
  region: nbg1
  controlPlanes:
    count: %d
    size: cx22
  workers:
    count: %d
    size: cx22
  healthCheck:
    nodeNotReadyThreshold: "3m"
`, state.ClusterName, cpCount, workerCount)

	cmd := exec.Command("kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create K8znerCluster: %v\nOutput: %s", err, string(output))
	}

	t.Logf("K8znerCluster created (CP: %d, Workers: %d)", cpCount, workerCount)
}

// waitForOperatorReconciling waits for the operator to start reconciling.
func waitForOperatorReconciling(ctx context.Context, t *testing.T, state *E2EState) {
	t.Log("Waiting for operator to reconcile cluster...")

	deadline := time.Now().Add(3 * time.Minute)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("Context cancelled waiting for operator")
		case <-ticker.C:
			if time.Now().After(deadline) {
				showOperatorLogsForScaling(t, state.KubeconfigPath)
				t.Fatal("Timeout waiting for operator to reconcile")
			}

			cmd := exec.Command("kubectl",
				"--kubeconfig", state.KubeconfigPath,
				"get", "k8znercluster", "-n", "k8zner-system", state.ClusterName,
				"-o", "jsonpath={.status.phase}")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("Waiting for status... (%v)", err)
				continue
			}

			phase := strings.TrimSpace(string(output))
			if phase != "" && phase != "Provisioning" {
				t.Logf("Operator reconciled (phase: %s)", phase)
				return
			}
			t.Logf("Current phase: %q", phase)
		}
	}
}

// testScaleUpViaCRD tests scaling up workers by patching the CRD.
func testScaleUpViaCRD(ctx context.Context, t *testing.T, state *E2EState, targetCount int) {
	t.Logf("Scaling UP workers to %d via CRD patch...", targetCount)

	initialNodeCount := countNodes(t, state.KubeconfigPath)
	t.Logf("Initial node count: %d", initialNodeCount)

	// Patch the CRD to increase worker count
	patchJSON := fmt.Sprintf(`{"spec":{"workers":{"count":%d}}}`, targetCount)
	cmd := exec.Command("kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"patch", "k8znercluster", state.ClusterName,
		"-n", "k8zner-system",
		"--type=merge",
		"-p", patchJSON)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to patch K8znerCluster: %v\nOutput: %s", err, string(output))
	}
	t.Log("CRD patched, waiting for operator to scale up...")

	// Wait for scaling event
	if err := waitForScalingEvent(ctx, t, state.KubeconfigPath, state.ClusterName, "ScalingUp", 2*time.Minute); err != nil {
		t.Logf("Warning: Did not see ScalingUp event: %v", err)
	}

	// Wait for new node to appear
	expectedNodes := initialNodeCount + 1
	if err := waitForNodeCount(ctx, t, state.KubeconfigPath, expectedNodes, 10*time.Minute); err != nil {
		showOperatorLogsForScaling(t, state.KubeconfigPath)
		showClusterStatusForScaling(t, state.KubeconfigPath, state.ClusterName)
		t.Fatalf("Scale up failed: %v", err)
	}

	// Verify cluster is Running
	if err := waitForClusterPhaseForScaling(ctx, t, state.KubeconfigPath, state.ClusterName, "Running", 5*time.Minute); err != nil {
		t.Logf("Warning: Cluster did not return to Running: %v", err)
	}

	t.Logf("Scale UP successful: %d -> %d nodes", initialNodeCount, expectedNodes)
}

// testScaleDownViaCRD tests scaling down workers by patching the CRD.
func testScaleDownViaCRD(ctx context.Context, t *testing.T, state *E2EState, targetCount int) {
	t.Logf("Scaling DOWN workers to %d via CRD patch...", targetCount)

	initialNodeCount := countNodes(t, state.KubeconfigPath)
	t.Logf("Current node count: %d", initialNodeCount)

	// Patch the CRD to decrease worker count
	patchJSON := fmt.Sprintf(`{"spec":{"workers":{"count":%d}}}`, targetCount)
	cmd := exec.Command("kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"patch", "k8znercluster", state.ClusterName,
		"-n", "k8zner-system",
		"--type=merge",
		"-p", patchJSON)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to patch K8znerCluster: %v\nOutput: %s", err, string(output))
	}
	t.Log("CRD patched, waiting for operator to scale down...")

	// Wait for scaling event
	if err := waitForScalingEvent(ctx, t, state.KubeconfigPath, state.ClusterName, "ScalingDown", 2*time.Minute); err != nil {
		t.Logf("Warning: Did not see ScalingDown event: %v", err)
	}

	// Wait for node to be removed
	expectedNodes := initialNodeCount - 1
	if err := waitForNodeCount(ctx, t, state.KubeconfigPath, expectedNodes, 10*time.Minute); err != nil {
		showOperatorLogsForScaling(t, state.KubeconfigPath)
		showClusterStatusForScaling(t, state.KubeconfigPath, state.ClusterName)
		t.Fatalf("Scale down failed: %v", err)
	}

	// Verify cluster is Running
	if err := waitForClusterPhaseForScaling(ctx, t, state.KubeconfigPath, state.ClusterName, "Running", 5*time.Minute); err != nil {
		t.Logf("Warning: Cluster did not return to Running: %v", err)
	}

	t.Logf("Scale DOWN successful: %d -> %d nodes", initialNodeCount, expectedNodes)
}

// waitForScalingEvent waits for a scaling event on the K8znerCluster.
func waitForScalingEvent(ctx context.Context, t *testing.T, kubeconfigPath, clusterName, eventReason string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for %s event", eventReason)
			}

			cmd := exec.Command("kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "events", "-n", "k8zner-system",
				"--field-selector", fmt.Sprintf("involvedObject.name=%s,reason=%s", clusterName, eventReason),
				"-o", "jsonpath={.items[*].reason}")
			output, _ := cmd.CombinedOutput()

			if strings.Contains(string(output), eventReason) {
				t.Logf("Detected %s event", eventReason)
				return nil
			}
		}
	}
}

// waitForNodeCount waits for the cluster to have exactly the expected number of nodes.
func waitForNodeCount(ctx context.Context, t *testing.T, kubeconfigPath string, expectedCount int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for %d nodes", expectedCount)
			}

			count := countNodes(t, kubeconfigPath)
			if count == expectedCount {
				t.Logf("Node count reached: %d", count)
				return nil
			}
			t.Logf("Waiting for nodes... (current: %d, expected: %d)", count, expectedCount)
		}
	}
}

// waitForClusterPhaseForScaling waits for the cluster to reach a specific phase.
func waitForClusterPhaseForScaling(ctx context.Context, t *testing.T, kubeconfigPath, clusterName, expectedPhase string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for phase %s", expectedPhase)
			}

			cmd := exec.Command("kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "k8znercluster", "-n", "k8zner-system", clusterName,
				"-o", "jsonpath={.status.phase}")
			output, _ := cmd.CombinedOutput()

			phase := strings.TrimSpace(string(output))
			if phase == expectedPhase {
				return nil
			}
			t.Logf("Phase: %s (waiting for %s)", phase, expectedPhase)
		}
	}
}

// countNodes returns the number of Ready nodes in the cluster.
func countNodes(t *testing.T, kubeconfigPath string) int {
	cmd := exec.Command("kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "nodes", "--no-headers")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Failed to get nodes: %v", err)
		return 0
	}

	lines := 0
	for _, b := range output {
		if b == '\n' {
			lines++
		}
	}
	return lines
}

// showOperatorLogsForScaling shows operator logs for debugging.
func showOperatorLogsForScaling(t *testing.T, kubeconfigPath string) {
	cmd := exec.Command("kubectl",
		"--kubeconfig", kubeconfigPath,
		"logs", "-n", "k8zner-system",
		"-l", "app.kubernetes.io/name=k8zner-operator",
		"--tail=100")
	output, _ := cmd.CombinedOutput()
	t.Logf("Operator logs:\n%s", string(output))
}

// showClusterStatusForScaling shows K8znerCluster status for debugging.
func showClusterStatusForScaling(t *testing.T, kubeconfigPath, clusterName string) {
	cmd := exec.Command("kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "k8znercluster", "-n", "k8zner-system", clusterName,
		"-o", "yaml")
	output, _ := cmd.CombinedOutput()
	t.Logf("K8znerCluster:\n%s", string(output))
}

// cleanupK8znerClusterForScaling removes the K8znerCluster resource.
func cleanupK8znerClusterForScaling(t *testing.T, state *E2EState) {
	t.Log("Cleaning up K8znerCluster resource...")

	cmd := exec.Command("kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"delete", "k8znercluster", "-n", "k8zner-system", state.ClusterName,
		"--ignore-not-found")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Logf("Warning: Failed to delete K8znerCluster: %v\nOutput: %s", err, string(output))
	}

	t.Log("K8znerCluster resource deleted")
}
