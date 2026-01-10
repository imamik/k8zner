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

	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/orchestration"
	hcloud_client "hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/platform/talos"

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
	secrets, err := talos.GetOrGenerateSecrets("/tmp/talos-secrets-"+clusterName+".json", cfg.Talos.Version)
	require.NoError(t, err, "Failed to get secrets")
	defer os.Remove("/tmp/talos-secrets-" + clusterName + ".json")

	talosGen := talos.NewGenerator(clusterName, cfg.Kubernetes.Version, cfg.Talos.Version, "", secrets)

	reconciler := orchestration.NewReconciler(hClient, talosGen, cfg)

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

	// Get server IP for diagnostics
	serverIP, err := hClient.GetServerIP(context.Background(), clusterName+"-control-plane-1")
	require.NoError(t, err, "Should be able to get server IP")
	t.Logf("Control plane node IP: %s", serverIP)

	// Get load balancer IP
	lb, err := hClient.GetLoadBalancer(context.Background(), clusterName+"-kube-api")
	require.NoError(t, err, "Should be able to get load balancer")
	lbIP := lb.PublicNet.IPv4.IP.String()
	t.Logf("Load balancer IP: %s", lbIP)

	// Save talosconfig for diagnostics
	clientCfg, err := talosGen.GetClientConfig()
	require.NoError(t, err, "Should be able to get talos client config")
	talosconfigPath := fmt.Sprintf("/tmp/talosconfig-%s", clusterName)
	err = os.WriteFile(talosconfigPath, clientCfg, 0600)
	require.NoError(t, err, "Should be able to write talosconfig")
	defer os.Remove(talosconfigPath)
	t.Logf("Talosconfig saved to: %s", talosconfigPath)

	// === DIAGNOSTIC PHASE: Validate cluster health via Talos ===
	t.Log("=== Starting diagnostic phase ===")

	// 1. Check Talos API connectivity
	t.Log("Step 1: Checking Talos API connectivity...")
	talosHealthCmd := exec.Command("talosctl",
		"--talosconfig", talosconfigPath,
		"--nodes", serverIP,
		"--endpoints", serverIP,
		"version")
	talosOutput, err := talosHealthCmd.CombinedOutput()
	if err != nil {
		t.Logf("  ⚠️  Talos API check: %v", err)
		t.Logf("  Output: %s", string(talosOutput))
	} else {
		t.Log("  ✓ Talos API is accessible")
	}

	// 2. Check etcd health via Talos
	t.Log("Step 2: Checking etcd health...")
	etcdCmd := exec.Command("talosctl",
		"--talosconfig", talosconfigPath,
		"--nodes", serverIP,
		"--endpoints", serverIP,
		"service", "etcd", "status")
	etcdOutput, err := etcdCmd.CombinedOutput()
	if err != nil {
		t.Logf("  ⚠️  etcd check: %v", err)
	} else {
		t.Log("  ✓ etcd service status:")
		t.Logf("    %s", string(etcdOutput))
	}

	// 3. Check Kubernetes services via Talos
	t.Log("Step 3: Checking Kubernetes services on Talos...")
	services := []string{"kubelet", "kube-apiserver", "kube-controller-manager", "kube-scheduler"}
	for _, svc := range services {
		cmd := exec.Command("talosctl",
			"--talosconfig", talosconfigPath,
			"--nodes", serverIP,
			"--endpoints", serverIP,
			"service", svc)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("  ⚠️  %s: %v", svc, err)
		} else {
			// Parse service status
			outStr := string(output)
			if len(outStr) > 200 {
				outStr = outStr[:200] + "..."
			}
			t.Logf("  %s: %s", svc, outStr)
		}
	}

	// 4. Check load balancer health
	t.Log("Step 4: Checking load balancer health...")
	lbHealthCmd := exec.Command("curl",
		"-k", // ignore self-signed cert
		"-s",
		"-o", "/dev/null",
		"-w", "%{http_code}",
		fmt.Sprintf("https://%s:6443/version", lbIP))
	lbHealthOutput, err := lbHealthCmd.CombinedOutput()
	if err != nil {
		t.Logf("  ⚠️  LB health check: %v", err)
	} else {
		httpCode := string(lbHealthOutput)
		if httpCode == "401" || httpCode == "200" {
			t.Logf("  ✓ Load balancer is routing to API (HTTP %s)", httpCode)
		} else {
			t.Logf("  ⚠️  Load balancer returned HTTP %s", httpCode)
		}
	}

	t.Log("=== Diagnostic phase complete ===")

	// === KUBECTL VALIDATION PHASE ===
	t.Log("=== Starting kubectl validation phase ===")
	t.Log("Waiting for Kubernetes cluster to be fully ready...")
	t.Log("This may take 10-15 minutes for a fresh orchestration...")

	kubectlCtx, kubectlCancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer kubectlCancel()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	nodeCheckSuccess := false
	var lastError error
	attemptCount := 0

	for {
		select {
		case <-kubectlCtx.Done():
			if !nodeCheckSuccess {
				t.Errorf("Timeout waiting for kubectl to work after 20 minutes. Last error: %v", lastError)
				t.Log("=== Final diagnostic dump ===")

				// Final diagnostics
				dumpCmd := exec.Command("talosctl",
					"--talosconfig", talosconfigPath,
					"--nodes", serverIP,
					"--endpoints", serverIP,
					"dmesg", "--tail", "50")
				if dumpOut, err := dumpCmd.CombinedOutput(); err == nil {
					t.Logf("Last 50 kernel messages:\n%s", string(dumpOut))
				}
			}
			goto endKubectlCheck

		case <-ticker.C:
			attemptCount++
			t.Logf("Attempt %d: Testing kubectl get nodes...", attemptCount)

			cmd := exec.Command("kubectl",
				"--kubeconfig", kubeconfigPath,
				"--request-timeout", "30s",
				"get", "nodes",
				"-o", "json")

			output, err := cmd.CombinedOutput()
			if err != nil {
				lastError = fmt.Errorf("kubectl failed: %v", err)
				// Log truncated error for readability
				errStr := string(output)
				if len(errStr) > 300 {
					errStr = errStr[:300] + "..."
				}
				t.Logf("  ⚠️  kubectl error: %s", errStr)

				// Every 4 attempts (~1 minute), check Talos services
				if attemptCount%4 == 0 {
					t.Logf("  Checking API server status via Talos...")
					statusCmd := exec.Command("talosctl",
						"--talosconfig", talosconfigPath,
						"--nodes", serverIP,
						"--endpoints", serverIP,
						"service", "kube-apiserver")
					if statusOut, statusErr := statusCmd.CombinedOutput(); statusErr == nil {
						t.Logf("    API server status: %s", string(statusOut)[:min(200, len(statusOut))])
					}
				}
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
				t.Logf("  ⚠️  Failed to parse output: %v (will retry)", err)
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
	t.Log("✓ Cluster is fully operational and accessible via kubectl!")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
