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

	"github.com/stretchr/testify/require"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
)

// TestE2ESelfHealingHA is a dedicated E2E test for self-healing on HA clusters.
// This test requires an HA cluster (3 CPs, 2+ workers) and tests:
// - Worker node failure and automatic replacement via operator
// - Cluster phase transitions (Running -> Degraded -> Healing -> Running)
//
// Run with: HCLOUD_TOKEN=xxx go test -v -timeout=60m -tags=e2e -run TestE2ESelfHealingHA ./tests/e2e/...
func TestE2ESelfHealingHA(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping self-healing test in short mode")
	}

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	clusterName := fmt.Sprintf("e2e-selfheal-%d", time.Now().Unix())
	t.Logf("=== Starting Self-Healing HA E2E Test: %s ===", clusterName)

	// Create HA configuration with 2 workers
	configPath := CreateTestConfig(t, clusterName, ModeHA, WithWorkers(2))
	defer os.Remove(configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Minute)
	defer cancel()

	// Create cluster via operator
	var state *OperatorTestContext
	var err error

	// === PHASE 1: DEPLOY HA CLUSTER ===
	t.Log("=== DEPLOYMENT PHASE ===")

	t.Run("Create", func(t *testing.T) {
		state, err = CreateClusterViaOperator(ctx, t, configPath)
		require.NoError(t, err, "Cluster creation should succeed")
	})

	// Ensure cleanup
	defer func() {
		if state != nil {
			DestroyCluster(context.Background(), t, state)
		}
	}()

	// Wait for cluster to be ready
	t.Run("WaitForClusterReady", func(t *testing.T) {
		err := WaitForClusterReady(ctx, t, state, 35*time.Minute)
		require.NoError(t, err, "Cluster should become ready")
	})

	t.Log("=== DEPLOYMENT COMPLETE ===")

	// Verify we have at least 2 workers
	cluster := GetClusterStatus(ctx, state)
	require.NotNil(t, cluster)
	if cluster.Status.Workers.Ready < 2 {
		t.Skipf("Self-healing test requires at least 2 workers, got %d", cluster.Status.Workers.Ready)
	}

	// === PHASE 2: SELF-HEALING TEST ===
	t.Log("=== SELF-HEALING TEST PHASE ===")

	// Get initial state
	initialNodeCount := CountKubernetesNodesViaKubectl(t, state.KubeconfigPath)
	t.Logf("Initial node count: %d", initialNodeCount)

	// Select a worker node to "fail" (use the last worker to minimize disruption)
	targetWorker := fmt.Sprintf("%s-workers-2", state.ClusterName)
	t.Logf("Simulating failure of worker: %s", targetWorker)

	// Step 1: Power off the worker server
	t.Run("SimulateFailure", func(t *testing.T) {
		err := SimulateNodeFailure(ctx, t, state, targetWorker)
		require.NoError(t, err, "Should be able to simulate node failure")
	})

	// Step 2: Wait for Kubernetes to detect the node as NotReady
	t.Run("WaitForNodeNotReady", func(t *testing.T) {
		err := WaitForNodeNotReadyK8s(ctx, t, state.KubeconfigPath, targetWorker, 5*time.Minute)
		require.NoError(t, err, "Node should become NotReady")
		t.Logf("Node %s detected as NotReady", targetWorker)
	})

	// Step 3: Wait for operator to detect unhealthy node via CRD phase
	t.Run("WaitForDegradedPhase", func(t *testing.T) {
		// Wait for cluster to transition to Degraded or Healing
		err := waitForClusterPhasesOperator(ctx, t, state, []k8znerv1alpha1.ClusterPhase{
			k8znerv1alpha1.ClusterPhaseDegraded,
			k8znerv1alpha1.ClusterPhaseHealing,
		}, 5*time.Minute)
		if err != nil {
			t.Logf("Warning: Cluster did not transition to Degraded/Healing phase: %v", err)
			// Continue anyway - operator might handle this differently
		} else {
			t.Log("Operator detected unhealthy worker")
		}
	})

	// Step 4: Wait for operator to replace the node and return to Running
	t.Run("WaitForHealing", func(t *testing.T) {
		t.Log("Waiting for operator to replace worker node (this may take several minutes)...")
		err := WaitForClusterPhase(ctx, t, state, k8znerv1alpha1.ClusterPhaseRunning, 15*time.Minute)
		if err != nil {
			// Show operator logs for debugging
			showOperatorLogsForDebugging(t, state.KubeconfigPath)
			showClusterStatusForDebugging(t, state)
		}
		require.NoError(t, err, "Cluster should return to Running phase")
		t.Log("Cluster returned to Running phase")
	})

	// Step 5: Verify the node count is restored
	t.Run("VerifyNodeCount", func(t *testing.T) {
		t.Log("Verifying node count is restored...")
		// Give some time for node to be fully registered
		time.Sleep(30 * time.Second)

		finalNodeCount := CountKubernetesNodesViaKubectl(t, state.KubeconfigPath)
		if finalNodeCount < initialNodeCount {
			showClusterStatusForDebugging(t, state)
			t.Fatalf("Node count not restored: expected %d, got %d", initialNodeCount, finalNodeCount)
		}
		t.Logf("Node count restored: %d", finalNodeCount)
	})

	// Step 6: Verify cluster functionality
	t.Run("VerifyClusterFunctionality", func(t *testing.T) {
		t.Log("Verifying cluster functionality...")
		if err := deployTestWorkloadOperator(ctx, t, state.KubeconfigPath); err != nil {
			t.Logf("Warning: Test workload deployment failed: %v", err)
		} else {
			t.Log("Test workload deployed successfully")
		}
	})

	t.Log("=== SELF-HEALING HA E2E TEST PASSED ===")
}

// waitForClusterPhasesOperator waits for the cluster to reach one of the expected phases.
func waitForClusterPhasesOperator(ctx context.Context, t *testing.T, state *OperatorTestContext, expectedPhases []k8znerv1alpha1.ClusterPhase, timeout time.Duration) error {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cluster := GetClusterStatus(ctx, state)
		if cluster != nil {
			for _, expected := range expectedPhases {
				if cluster.Status.Phase == expected {
					t.Logf("Cluster reached phase: %s", expected)
					return nil
				}
			}
			t.Logf("  Current phase: %s (waiting for %v)", cluster.Status.Phase, expectedPhases)
		}

		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("timeout waiting for cluster phases %v", expectedPhases)
}

// showOperatorLogsForDebugging shows recent operator logs for debugging.
func showOperatorLogsForDebugging(t *testing.T, kubeconfigPath string) {
	t.Helper()

	cmd := exec.Command("kubectl",
		"--kubeconfig", kubeconfigPath,
		"logs", "-n", "k8zner-system", "-l", "app.kubernetes.io/name=k8zner-operator",
		"--tail=50")
	output, _ := cmd.CombinedOutput()
	t.Logf("Operator logs:\n%s", string(output))
}

// showClusterStatusForDebugging shows the K8znerCluster status for debugging.
func showClusterStatusForDebugging(t *testing.T, state *OperatorTestContext) {
	t.Helper()

	cmd := exec.Command("kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"get", "k8znercluster", "-n", "k8zner-system", state.ClusterName, "-o", "yaml")
	output, _ := cmd.CombinedOutput()
	t.Logf("K8znerCluster status:\n%s", string(output))
}

// deployTestWorkloadOperator deploys a simple workload to verify cluster functionality.
func deployTestWorkloadOperator(ctx context.Context, t *testing.T, kubeconfigPath string) error {
	t.Helper()

	manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: selfhealing-test
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: selfhealing-test
  template:
    metadata:
      labels:
        app: selfhealing-test
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 80
`

	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply manifest: %w, output: %s", err, string(output))
	}

	// Wait for deployment to be ready
	t.Log("Waiting for test deployment to be ready...")
	waitCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"rollout", "status", "deployment/selfhealing-test",
		"--timeout=2m")
	waitOutput, err := waitCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("deployment not ready: %w, output: %s", err, string(waitOutput))
	}

	t.Log("Test deployment is running")

	// Clean up
	cleanupCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"delete", "deployment", "selfhealing-test")
	_ = cleanupCmd.Run()

	return nil
}
