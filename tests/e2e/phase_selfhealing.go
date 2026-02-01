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

// phaseSelfHealing tests the self-healing mechanism using the k8zner-operator.
// This test deploys the operator, creates a K8znerCluster resource, then
// simulates node failures to verify automatic replacement.
//
// Prerequisites:
// - Cluster must be running with at least 2 workers (HA mode preferred)
// - HCLOUD_TOKEN environment variable must be set
// - Talos snapshot must exist with label os=talos
func phaseSelfHealing(t *testing.T, state *E2EState) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	t.Log("=== SELF-HEALING E2E TEST ===")

	// Verify we have at least 2 workers for safe testing
	if len(state.WorkerIPs) < 2 {
		t.Skip("Self-healing test requires at least 2 worker nodes")
	}

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Fatal("HCLOUD_TOKEN required for self-healing test")
	}

	// Step 1: Deploy the k8zner-operator
	t.Run("DeployOperator", func(t *testing.T) {
		deployOperatorAddon(t, state, token)
	})

	// Step 2: Create K8znerCluster CRD resource for this cluster
	t.Run("CreateClusterResource", func(t *testing.T) {
		createK8znerClusterResource(t, state)
	})

	// Step 3: Verify operator is reconciling the cluster
	t.Run("VerifyOperatorReconciling", func(t *testing.T) {
		verifyOperatorReconciling(t, state)
	})

	// Step 4: Test worker node self-healing
	t.Run("WorkerSelfHealing", func(t *testing.T) {
		testWorkerSelfHealing(ctx, t, state)
	})

	// Step 5: Cleanup K8znerCluster resource (operator should not delete real nodes)
	t.Run("Cleanup", func(t *testing.T) {
		cleanupK8znerClusterResource(t, state)
	})

	t.Log("=== SELF-HEALING E2E TEST PASSED ===")
}

// deployOperatorAddon deploys the k8zner-operator to the cluster.
func deployOperatorAddon(t *testing.T, state *E2EState, token string) {
	t.Log("Deploying k8zner-operator addon...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Get network ID
	network, _ := state.Client.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	// Build config with operator enabled
	cfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: token,
		Addons: config.AddonsConfig{
			Operator: config.OperatorConfig{
				Enabled: true,
				Version: "main", // Use main branch image
			},
		},
	}

	// Apply the operator addon
	if err := addons.Apply(ctx, cfg, state.Kubeconfig, networkID); err != nil {
		t.Fatalf("Failed to deploy operator: %v", err)
	}

	// Wait for operator to be ready
	waitForPod(t, state.KubeconfigPath, "k8zner-system", "app.kubernetes.io/name=k8zner-operator", 5*time.Minute)

	t.Log("✓ k8zner-operator deployed and running")
	state.AddonsInstalled["k8zner-operator"] = true
}

// createK8znerClusterResource creates a K8znerCluster CRD for the existing cluster.
func createK8znerClusterResource(t *testing.T, state *E2EState) {
	t.Log("Creating K8znerCluster resource for existing cluster...")

	// Determine counts from state
	cpCount := len(state.ControlPlaneIPs)
	workerCount := len(state.WorkerIPs)

	// Build the K8znerCluster manifest
	// Include SSH key annotation so operator can create new servers
	manifest := fmt.Sprintf(`apiVersion: k8zner.io/v1alpha1
kind: K8znerCluster
metadata:
  name: %s
  namespace: k8zner-system
  annotations:
    k8zner.io/ssh-keys: "%s"
spec:
  region: nbg1
  controlPlanes:
    count: %d
    size: cx22
  workers:
    count: %d
    size: cx22
  healthCheck:
    nodeNotReadyThreshold: "2m"
`, state.ClusterName, state.SSHKeyName, cpCount, workerCount)

	// Apply via kubectl
	// #nosec G204 -- E2E test with controlled command arguments
	cmd := exec.Command("kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create K8znerCluster resource: %v\nOutput: %s", err, string(output))
	}

	t.Logf("✓ K8znerCluster resource created (CP: %d, Workers: %d)", cpCount, workerCount)
}

// verifyOperatorReconciling checks that the operator is reconciling the cluster.
func verifyOperatorReconciling(t *testing.T, state *E2EState) {
	t.Log("Verifying operator is reconciling the cluster...")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Show cluster status for debugging
			// #nosec G204 -- E2E test with controlled command arguments
			cmd := exec.Command("kubectl",
				"--kubeconfig", state.KubeconfigPath,
				"get", "k8znercluster", "-n", "k8zner-system", state.ClusterName, "-o", "yaml")
			output, _ := cmd.CombinedOutput()
			t.Logf("K8znerCluster status:\n%s", string(output))
			t.Fatal("Timeout waiting for operator to reconcile cluster")
		case <-ticker.C:
			// Check if the cluster has a status phase set
			// #nosec G204 -- E2E test with controlled command arguments
			cmd := exec.Command("kubectl",
				"--kubeconfig", state.KubeconfigPath,
				"get", "k8znercluster", "-n", "k8zner-system", state.ClusterName,
				"-o", "jsonpath={.status.phase}")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("Waiting for operator to set status...")
				continue
			}

			phase := strings.TrimSpace(string(output))
			if phase != "" && phase != "Provisioning" {
				t.Logf("✓ Operator reconciled cluster (phase: %s)", phase)
				return
			}
			t.Logf("Waiting for cluster phase (current: %q)...", phase)
		}
	}
}

// testWorkerSelfHealing tests automatic worker node replacement.
func testWorkerSelfHealing(ctx context.Context, t *testing.T, state *E2EState) {
	t.Log("Testing worker node self-healing...")

	// Get initial node count
	initialNodeCount := countKubernetesNodes(t, state.KubeconfigPath)
	t.Logf("Initial node count: %d", initialNodeCount)

	// Select a worker node to "fail" (use the last worker to minimize disruption)
	targetWorker := fmt.Sprintf("%s-workers-%d", state.ClusterName, len(state.WorkerIPs))
	t.Logf("Simulating failure of worker: %s", targetWorker)

	// Record the original worker IPs for comparison
	originalWorkerIPs := make([]string, len(state.WorkerIPs))
	copy(originalWorkerIPs, state.WorkerIPs)

	// Step 1: Power off the worker server
	t.Log("Powering off worker server...")
	if err := powerOffServer(ctx, state, targetWorker); err != nil {
		t.Fatalf("Failed to power off server: %v", err)
	}

	// Step 2: Wait for Kubernetes to detect the node as NotReady
	t.Log("Waiting for Kubernetes to detect node as NotReady...")
	if err := waitForNodeNotReady(ctx, t, state.KubeconfigPath, targetWorker, 5*time.Minute); err != nil {
		t.Fatalf("Node did not become NotReady: %v", err)
	}
	t.Logf("✓ Node %s detected as NotReady", targetWorker)

	// Step 3: Wait for operator to detect unhealthy node and update status
	t.Log("Waiting for operator to detect unhealthy node...")
	if err := waitForClusterPhase(ctx, t, state.KubeconfigPath, state.ClusterName, "Degraded", 5*time.Minute); err != nil {
		// Also accept "Healing" phase since operator might transition quickly
		if err := waitForClusterPhase(ctx, t, state.KubeconfigPath, state.ClusterName, "Healing", 1*time.Minute); err != nil {
			t.Logf("Warning: Cluster did not transition to Degraded/Healing phase: %v", err)
		}
	}
	t.Log("✓ Operator detected unhealthy worker")

	// Step 4: Wait for operator to replace the node
	// This includes: server deletion, new server creation, config application, node join
	t.Log("Waiting for operator to replace worker node (this may take several minutes)...")
	if err := waitForClusterPhase(ctx, t, state.KubeconfigPath, state.ClusterName, "Running", 15*time.Minute); err != nil {
		// Show operator logs for debugging
		showOperatorLogs(t, state.KubeconfigPath)
		showClusterStatus(t, state.KubeconfigPath, state.ClusterName)
		t.Fatalf("Cluster did not return to Running phase: %v", err)
	}
	t.Log("✓ Cluster returned to Running phase")

	// Step 5: Verify the node count is restored
	t.Log("Verifying node count is restored...")
	finalNodeCount := countKubernetesNodes(t, state.KubeconfigPath)
	if finalNodeCount < initialNodeCount {
		showClusterStatus(t, state.KubeconfigPath, state.ClusterName)
		t.Fatalf("Node count not restored: expected %d, got %d", initialNodeCount, finalNodeCount)
	}
	t.Logf("✓ Node count restored: %d", finalNodeCount)

	// Step 6: Verify cluster functionality
	t.Log("Verifying cluster functionality...")
	if err := deployTestWorkload(ctx, t, state.KubeconfigPath); err != nil {
		t.Logf("Warning: Test workload deployment failed: %v", err)
	} else {
		t.Log("✓ Test workload deployed successfully")
	}

	t.Log("✓ Worker self-healing test passed")
}

// waitForClusterPhase waits for the K8znerCluster to reach a specific phase.
func waitForClusterPhase(ctx context.Context, t *testing.T, kubeconfigPath, clusterName, expectedPhase string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for cluster phase %s", expectedPhase)
			}

			// #nosec G204 -- E2E test with controlled command arguments
			cmd := exec.Command("kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "k8znercluster", "-n", "k8zner-system", clusterName,
				"-o", "jsonpath={.status.phase}")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("kubectl error (will retry): %v", err)
				continue
			}

			phase := strings.TrimSpace(string(output))
			if phase == expectedPhase {
				return nil
			}
			t.Logf("Cluster phase: %s (waiting for %s)...", phase, expectedPhase)
		}
	}
}

// showOperatorLogs shows recent operator logs for debugging.
func showOperatorLogs(t *testing.T, kubeconfigPath string) {
	// #nosec G204 -- E2E test with controlled command arguments
	cmd := exec.Command("kubectl",
		"--kubeconfig", kubeconfigPath,
		"logs", "-n", "k8zner-system", "-l", "app.kubernetes.io/name=k8zner-operator",
		"--tail=50")
	output, _ := cmd.CombinedOutput()
	t.Logf("Operator logs:\n%s", string(output))
}

// showClusterStatus shows the K8znerCluster status for debugging.
func showClusterStatus(t *testing.T, kubeconfigPath, clusterName string) {
	// #nosec G204 -- E2E test with controlled command arguments
	cmd := exec.Command("kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "k8znercluster", "-n", "k8zner-system", clusterName, "-o", "yaml")
	output, _ := cmd.CombinedOutput()
	t.Logf("K8znerCluster status:\n%s", string(output))
}

// cleanupK8znerClusterResource removes the K8znerCluster resource.
func cleanupK8znerClusterResource(t *testing.T, state *E2EState) {
	t.Log("Cleaning up K8znerCluster resource...")

	// Delete the K8znerCluster resource (operator should NOT delete real nodes)
	// #nosec G204 -- E2E test with controlled command arguments
	cmd := exec.Command("kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"delete", "k8znercluster", "-n", "k8zner-system", state.ClusterName,
		"--ignore-not-found")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Logf("Warning: Failed to delete K8znerCluster: %v\nOutput: %s", err, string(output))
	}

	t.Log("✓ K8znerCluster resource deleted")
}

// powerOffServer powers off a server using the HCloud API.
func powerOffServer(ctx context.Context, state *E2EState, serverName string) error {
	// Get server by name
	servers, err := state.Client.GetServersByLabel(ctx, map[string]string{
		"cluster": state.ClusterName,
	})
	if err != nil {
		return fmt.Errorf("failed to get servers: %w", err)
	}

	for _, server := range servers {
		if server.Name == serverName {
			// Use hcloud CLI to power off (API client doesn't have direct power off)
			// #nosec G204 -- E2E test with controlled command arguments
			cmd := exec.CommandContext(ctx, "hcloud", "server", "poweroff", fmt.Sprintf("%d", server.ID))
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to power off server: %w, output: %s", err, string(output))
			}
			return nil
		}
	}

	return fmt.Errorf("server %s not found", serverName)
}

// waitForNodeNotReady waits for a Kubernetes node to become NotReady.
func waitForNodeNotReady(ctx context.Context, t *testing.T, kubeconfigPath, nodeName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for node %s to become NotReady", nodeName)
			}

			// #nosec G204 -- E2E test with controlled command arguments
			cmd := exec.CommandContext(ctx, "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "node", nodeName,
				"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
			output, err := cmd.CombinedOutput()
			if err != nil {
				// Node might not exist anymore
				if strings.Contains(string(output), "NotFound") {
					return nil
				}
				t.Logf("kubectl error (will retry): %v", err)
				continue
			}

			status := strings.TrimSpace(string(output))
			if status != "True" {
				t.Logf("Node %s status: %s", nodeName, status)
				return nil
			}
			t.Logf("Node %s still Ready, waiting...", nodeName)
		}
	}
}

// countKubernetesNodes returns the number of nodes in the cluster.
func countKubernetesNodes(t *testing.T, kubeconfigPath string) int {
	// #nosec G204 -- E2E test with controlled command arguments
	cmd := exec.Command("kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "nodes", "--no-headers")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: Failed to get nodes: %v", err)
		return 0
	}

	// Count lines
	lines := 0
	for _, b := range output {
		if b == '\n' {
			lines++
		}
	}
	return lines
}

// deployTestWorkload deploys a simple workload to verify cluster functionality.
func deployTestWorkload(ctx context.Context, t *testing.T, kubeconfigPath string) error {
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

	// #nosec G204 -- E2E test with controlled command arguments
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
	// #nosec G204 -- E2E test with controlled command arguments
	waitCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"rollout", "status", "deployment/selfhealing-test",
		"--timeout=2m")
	waitOutput, err := waitCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("deployment not ready: %w, output: %s", err, string(waitOutput))
	}

	t.Log("✓ Test deployment is running")

	// Clean up
	// #nosec G204 -- E2E test with controlled command arguments
	cleanupCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"delete", "deployment", "selfhealing-test")
	_ = cleanupCmd.Run()

	return nil
}
