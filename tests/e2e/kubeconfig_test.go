//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/cluster"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_client "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/talos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKubeconfigRetrieval is a minimal E2E test focused on validating kubeconfig retrieval.
// It provisions a minimal cluster (1 control plane only) and verifies kubectl works.
func TestKubeconfigRetrieval(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	// Use a unique cluster name
	clusterName := fmt.Sprintf("kubeconfig-test-%d", time.Now().Unix())
	t.Logf("Creating test cluster: %s", clusterName)

	// Minimal config - single control plane, no workers
	cfg := &config.Config{
		ClusterName: clusterName,
		HCloudToken: token,
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		Firewall: config.FirewallConfig{
			UseCurrentIPv4: true,
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					ServerType: "cpx22", // Same as other E2E tests
					Location:   "nbg1",
					Count:      1, // Just one control plane
					Image:      "talos",
				},
			},
		},
		Workers: []config.WorkerNodePool{}, // No workers to save cost
		Talos: config.TalosConfig{
			Version: "v1.8.3",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.31.0",
		},
	}

	// Initialize clients
	hClient := hcloud_client.NewRealClient(token)
	cleaner := &ResourceCleaner{t: t}

	// Setup SSH Key
	sshKeyName, _ := setupSSHKey(t, hClient, cleaner, clusterName)
	cfg.SSHKeys = []string{sshKeyName}

	// Cleanup function
	cleanup := func() {
		t.Log("Cleaning up resources...")
		ctx := context.Background()

		// Delete in reverse order
		hClient.DeleteServer(ctx, clusterName+"-control-plane-1")
		time.Sleep(2 * time.Second)

		hClient.DeleteLoadBalancer(ctx, clusterName+"-kube-api")
		time.Sleep(2 * time.Second)

		hClient.DeleteFirewall(ctx, clusterName)
		time.Sleep(2 * time.Second)

		hClient.DeleteNetwork(ctx, clusterName)
		time.Sleep(1 * time.Second)

		hClient.DeletePlacementGroup(ctx, clusterName+"-control-plane")
		hClient.DeleteCertificate(ctx, clusterName+"-state")

		t.Log("Cleanup complete")
	}
	defer cleanup()

	// Initialize Talos Generator
	talosGen, err := talos.NewConfigGenerator(
		clusterName,
		cfg.Kubernetes.Version,
		cfg.Talos.Version,
		"",
		"",
	)
	require.NoError(t, err, "Failed to create Talos generator")

	reconciler := cluster.NewReconciler(hClient, talosGen, cfg)

	// Run Reconcile with generous timeout
	t.Log("Starting reconciliation (this will take 15-20 minutes)...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	startTime := time.Now()
	kubeconfig, err := reconciler.Reconcile(ctx)
	duration := time.Since(startTime)

	t.Logf("Reconciliation completed in %s", duration)
	require.NoError(t, err, "Reconciliation should succeed")
	require.NotEmpty(t, kubeconfig, "Kubeconfig should be returned")

	t.Logf("✓ Kubeconfig retrieved successfully (%d bytes)", len(kubeconfig))

	// Save kubeconfig to temporary file
	kubeconfigPath := fmt.Sprintf("/tmp/kubeconfig-%s", clusterName)
	err = os.WriteFile(kubeconfigPath, kubeconfig, 0600)
	require.NoError(t, err, "Should be able to write kubeconfig")
	defer os.Remove(kubeconfigPath)

	t.Logf("Kubeconfig saved to: %s", kubeconfigPath)

	// Verify kubeconfig content looks valid
	assert.Contains(t, string(kubeconfig), "apiVersion", "Kubeconfig should contain apiVersion")
	assert.Contains(t, string(kubeconfig), "clusters", "Kubeconfig should contain clusters")
	assert.Contains(t, string(kubeconfig), clusterName, "Kubeconfig should contain cluster name")

	// Test kubectl get nodes with retries
	t.Log("Testing kubectl get nodes...")
	kubectlCtx, kubectlCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer kubectlCancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	nodeCheckSuccess := false
	var lastError error

	for {
		select {
		case <-kubectlCtx.Done():
			if !nodeCheckSuccess {
				t.Errorf("Timeout waiting for kubectl to work. Last error: %v", lastError)
			}
			goto endKubectlCheck

		case <-ticker.C:
			t.Log("Attempting kubectl get nodes...")

			cmd := exec.Command("kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "nodes",
				"-o", "json")

			output, err := cmd.CombinedOutput()
			if err != nil {
				lastError = fmt.Errorf("kubectl failed: %v, output: %s", err, string(output))
				t.Logf("  kubectl not ready yet: %v (will retry)", err)
				continue
			}

			// Parse JSON output
			var nodeList struct {
				Items []struct {
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
					Status struct {
						Conditions []struct {
							Type   string `json:"type"`
							Status string `json:"status"`
						} `json:"conditions"`
					} `json:"status"`
				} `json:"items"`
			}

			if err := json.Unmarshal(output, &nodeList); err != nil {
				lastError = fmt.Errorf("failed to parse kubectl output: %v", err)
				t.Logf("  Failed to parse output: %v (will retry)", err)
				continue
			}

			if len(nodeList.Items) >= 1 {
				t.Logf("✓ kubectl get nodes succeeded!")
				t.Logf("  Found %d node(s):", len(nodeList.Items))
				for _, node := range nodeList.Items {
					ready := "NotReady"
					for _, cond := range node.Status.Conditions {
						if cond.Type == "Ready" && cond.Status == "True" {
							ready = "Ready"
							break
						}
					}
					t.Logf("    - %s [%s]", node.Metadata.Name, ready)
				}
				nodeCheckSuccess = true
				goto endKubectlCheck
			}

			t.Logf("  Found %d nodes, expecting at least 1 (will retry)", len(nodeList.Items))
		}
	}

endKubectlCheck:
	require.True(t, nodeCheckSuccess, "kubectl get nodes should succeed")

	t.Log("✓ All kubeconfig validation checks passed!")
}
