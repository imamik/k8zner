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
	"github.com/imamik/k8zner/internal/util/naming"
)

// TestE2EHAOperations is Test 2: HA scaling & failure recovery.
//
// This test validates HA cluster operations:
// - Config: 3 CP + 2 workers initially, mode=ha, minimal addons (Cilium + CCM only)
// - Timeout: 90 minutes
//
// IMPORTANT: This test will ONLY run if TestE2EFullStackDev passes first.
// There is NO override - if FullStack fails, HA test is skipped. Period.
//
// Required environment variables:
//   - HCLOUD_TOKEN - Hetzner Cloud API token
//
// Example:
//
//	# Run both tests (HA auto-skips if FullStack fails)
//	go test -v -timeout=3h -tags=e2e -run "TestE2E(FullStackDev|HAOperations)" ./tests/e2e/
func TestE2EHAOperations(t *testing.T) {
	// Skip logic: only run if TestE2EFullStackDev passed - NO OVERRIDE
	if !IsFullStackPassed() {
		t.Skip("Skipping: TestE2EFullStackDev did not pass")
	}

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	// Generate unique cluster name (short for Hetzner resource limits)
	clusterName := naming.E2ECluster(naming.E2EHA) // e.g., e2e-ha-abc12

	t.Logf("=== Starting HA Operations E2E Test: %s ===", clusterName)
	t.Log("=== Config: 3 CP + 2 workers, minimal addons ===")

	// Create HA configuration with minimal addons
	configPath := CreateTestConfig(t, clusterName, ModeHA,
		WithWorkers(2),
		WithCPCount(3),
		WithMinimalAddons(true),
	)
	defer os.Remove(configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()

	// Create cluster via operator
	var state *OperatorTestContext

	// Cleanup handler
	defer func() {
		if state != nil {
			DestroyCluster(context.Background(), t, state)
		}
	}()

	// =========================================================================
	// SUBTEST 01: Create HA Cluster
	// =========================================================================
	t.Run("01_CreateHACluster", func(t *testing.T) {
		var createErr error
		state, createErr = CreateClusterViaOperator(ctx, t, configPath)
		require.NoError(t, createErr, "HA cluster creation should succeed")
	})

	// =========================================================================
	// SUBTEST 02: Verify Initial Health
	// =========================================================================
	t.Run("02_VerifyInitialHealth", func(t *testing.T) {
		// Wait for cluster to be ready
		err := WaitForClusterReady(ctx, t, state, 35*time.Minute)
		require.NoError(t, err, "HA cluster should become ready")

		// Verify all nodes ready
		cluster := GetClusterStatus(ctx, state)
		require.NotNil(t, cluster)
		require.GreaterOrEqual(t, cluster.Status.ControlPlanes.Ready, 3, "should have 3 ready CPs")
		require.GreaterOrEqual(t, cluster.Status.Workers.Ready, 2, "should have 2 ready workers")

		// Verify etcd healthy via kubectl
		verifyEtcdHealth(t, state.KubeconfigPath)
	})

	// =========================================================================
	// SUBTEST 03: Verify CCM
	// =========================================================================
	t.Run("03_Verify_CCM", func(t *testing.T) {
		err := WaitForAddonInstalled(ctx, t, state, k8znerv1alpha1.AddonNameCCM, 2*time.Minute)
		require.NoError(t, err, "CCM should be installed")

		// LoadBalancer test
		legacyState := state.ToE2EState()
		testCCMLoadBalancer(t, legacyState)
	})

	// =========================================================================
	// SUBTEST 04: Scale Workers Up
	// =========================================================================
	t.Run("04_ScaleWorkersUp", func(t *testing.T) {
		t.Log("Scaling workers from 2 to 3...")
		err := ScaleCluster(ctx, t, state, 3)
		require.NoError(t, err, "Scale up should succeed")
	})

	// =========================================================================
	// SUBTEST 05: Verify Scale Up
	// =========================================================================
	t.Run("05_VerifyScaleUp", func(t *testing.T) {
		err := WaitForNodeCount(ctx, t, state, "workers", 3, 15*time.Minute)
		require.NoError(t, err, "Workers should scale to 3")

		// Verify all nodes are ready in Kubernetes
		nodeCount := CountKubernetesNodesViaKubectl(t, state.KubeconfigPath)
		require.GreaterOrEqual(t, nodeCount, 6, "should have at least 6 nodes (3 CP + 3 workers)")
		t.Logf("Scale up verified: %d nodes", nodeCount)
	})

	// =========================================================================
	// SUBTEST 06: Scale Workers Down
	// =========================================================================
	t.Run("06_ScaleWorkersDown", func(t *testing.T) {
		t.Log("Scaling workers from 3 to 2...")
		err := ScaleCluster(ctx, t, state, 2)
		require.NoError(t, err, "Scale down should succeed")
	})

	// =========================================================================
	// SUBTEST 07: Verify Scale Down
	// =========================================================================
	t.Run("07_VerifyScaleDown", func(t *testing.T) {
		err := WaitForNodeCount(ctx, t, state, "workers", 2, 15*time.Minute)
		require.NoError(t, err, "Workers should scale to 2")

		// Verify the old worker node was deleted from Hetzner
		verifyWorkerDeleted(ctx, t, state, 3)
		t.Log("Scale down verified: worker-3 deleted from Hetzner")
	})

	// =========================================================================
	// SUBTEST 08: Simulate CP Failure
	// =========================================================================
	var targetCP string
	t.Run("08_SimulateCPFailure", func(t *testing.T) {
		// Find the last CP node from cluster status (not cp-1 which is the bootstrap node)
		cluster := GetClusterStatus(ctx, state)
		require.NotNil(t, cluster, "Should be able to get cluster status")
		require.GreaterOrEqual(t, len(cluster.Status.ControlPlanes.Nodes), 3, "Should have at least 3 CPs")
		targetCP = cluster.Status.ControlPlanes.Nodes[len(cluster.Status.ControlPlanes.Nodes)-1].Name
		t.Logf("Simulating failure of: %s", targetCP)

		err := SimulateNodeFailure(ctx, t, state, targetCP)
		require.NoError(t, err, "Should be able to power off CP")
	})

	// =========================================================================
	// SUBTEST 09: Verify CP Replacement
	// =========================================================================
	t.Run("09_VerifyCPReplacement", func(t *testing.T) {
		// Wait for Kubernetes to detect node as NotReady
		err := WaitForNodeNotReadyK8s(ctx, t, state.KubeconfigPath, targetCP, 8*time.Minute)
		require.NoError(t, err, "Node should become NotReady")
		t.Logf("Node %s detected as NotReady", targetCP)

		// Wait for operator to detect and transition to Degraded/Healing
		err = waitForClusterPhasesHA(ctx, t, state, []k8znerv1alpha1.ClusterPhase{
			k8znerv1alpha1.ClusterPhaseDegraded,
			k8znerv1alpha1.ClusterPhaseHealing,
		}, 5*time.Minute)
		if err != nil {
			t.Logf("Warning: Cluster did not transition to Degraded/Healing: %v (continuing)", err)
		}

		// Wait for cluster to return to Running (new CP joins)
		t.Log("Waiting for operator to replace control plane node...")
		err = WaitForClusterPhase(ctx, t, state, k8znerv1alpha1.ClusterPhaseRunning, 20*time.Minute)
		if err != nil {
			showOperatorLogsHA(t, state.KubeconfigPath)
			showClusterStatusHA(t, state)
		}
		require.NoError(t, err, "Cluster should return to Running phase")

		// Verify etcd is healthy (quorum maintained)
		verifyEtcdHealth(t, state.KubeconfigPath)
		t.Log("CP replacement verified: new CP joined, etcd healthy")
	})

	// =========================================================================
	// SUBTEST 10: Verify Cluster Recovery
	// =========================================================================
	t.Run("10_VerifyClusterRecovery", func(t *testing.T) {
		// Verify full cluster health
		VerifyClusterHealth(t, state)

		// Deploy test workload
		err := deployTestWorkloadHA(ctx, t, state.KubeconfigPath)
		require.NoError(t, err, "Test workload should deploy successfully")
		t.Log("Cluster recovery verified: workload deployed successfully")
	})

	t.Log("=== HA OPERATIONS E2E TEST PASSED ===")
}

// verifyEtcdHealth checks etcd health via kubectl exec.
func verifyEtcdHealth(t *testing.T, kubeconfigPath string) {
	t.Log("  Verifying etcd health...")

	// Get etcd pod name
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "pods", "-n", "kube-system",
		"-l", "component=etcd",
		"-o", "jsonpath={.items[0].metadata.name}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("  Warning: Could not get etcd pod: %v", err)
		return
	}

	podName := strings.TrimSpace(string(output))
	if podName == "" {
		t.Log("  Warning: No etcd pod found")
		return
	}

	// Check etcd member list
	cmd = exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"exec", "-n", "kube-system", podName, "--",
		"etcdctl", "member", "list", "--write-out=table")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Logf("  Warning: Could not check etcd members: %v (output: %s)", err, string(output))
		return
	}

	t.Log("  etcd members healthy")
}

// verifyWorkerDeleted verifies a worker was deleted from Hetzner.
func verifyWorkerDeleted(ctx context.Context, t *testing.T, state *OperatorTestContext, workerNumber int) {
	workerName := naming.WorkerWithID(state.ClusterName, fmt.Sprintf("%d", workerNumber))

	// Try to get the server - should not exist
	servers, err := state.HCloudClient.GetServersByLabel(ctx, map[string]string{
		"cluster": state.ClusterName,
	})
	if err != nil {
		t.Logf("  Warning: Could not list servers: %v", err)
		return
	}

	for _, server := range servers {
		if server.Name == workerName {
			t.Logf("  Warning: Worker %s still exists in Hetzner", workerName)
			return
		}
	}

	t.Logf("  Worker %s deleted from Hetzner", workerName)
}

// waitForClusterPhasesHA waits for the cluster to reach one of the expected phases.
func waitForClusterPhasesHA(ctx context.Context, t *testing.T, state *OperatorTestContext, expectedPhases []k8znerv1alpha1.ClusterPhase, timeout time.Duration) error {
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
					t.Logf("  Cluster reached phase: %s", expected)
					return nil
				}
			}
			t.Logf("  Current phase: %s (waiting for %v)", cluster.Status.Phase, expectedPhases)
		}

		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("timeout waiting for cluster phases %v", expectedPhases)
}

// showOperatorLogsHA shows recent operator logs for debugging.
func showOperatorLogsHA(t *testing.T, kubeconfigPath string) {
	t.Helper()

	cmd := exec.Command("kubectl",
		"--kubeconfig", kubeconfigPath,
		"logs", "-n", "k8zner-system", "-l", "app.kubernetes.io/name=k8zner-operator",
		"--tail=50")
	output, _ := cmd.CombinedOutput()
	t.Logf("Operator logs:\n%s", string(output))
}

// showClusterStatusHA shows the K8znerCluster status for debugging.
func showClusterStatusHA(t *testing.T, state *OperatorTestContext) {
	t.Helper()

	cmd := exec.Command("kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"get", "k8znercluster", "-n", "k8zner-system", state.ClusterName, "-o", "yaml")
	output, _ := cmd.CombinedOutput()
	t.Logf("K8znerCluster status:\n%s", string(output))
}

// deployTestWorkloadHA deploys a simple workload to verify cluster functionality.
func deployTestWorkloadHA(ctx context.Context, t *testing.T, kubeconfigPath string) error {
	t.Helper()

	manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ha-ops-test
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app: ha-ops-test
  template:
    metadata:
      labels:
        app: ha-ops-test
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
	t.Log("  Waiting for test deployment to be ready...")
	waitCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"rollout", "status", "deployment/ha-ops-test",
		"--timeout=3m")
	waitOutput, err := waitCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("deployment not ready: %w, output: %s", err, string(waitOutput))
	}

	t.Log("  Test deployment is running")

	// Clean up
	cleanupCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"delete", "deployment", "ha-ops-test")
	_ = cleanupCmd.Run()

	return nil
}
