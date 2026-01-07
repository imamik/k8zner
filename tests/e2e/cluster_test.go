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
)

// TestClusterProvisioning is an end-to-end test that provisions a cluster and verifies resources.
// It requires HCLOUD_TOKEN to be set.
func TestClusterProvisioning(t *testing.T) {
	t.Parallel() // Run in parallel with other tests

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	// Use a unique cluster name to avoid conflicts
	clusterName := fmt.Sprintf("e2e-test-%d", time.Now().Unix())

	// Create a minimal config
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
					ServerType: "cpx22",
					Location:   "nbg1",
					Count:      1,
					Image:      "talos",
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "worker",
				ServerType: "cpx22",
				Location:   "nbg1",
				Count:      1,
				Image:      "talos",
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.8.3",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.31.0",
		},
	}

	// Use shared snapshots if available (built in TestMain)
	if sharedCtx != nil && sharedCtx.SnapshotAMD64 != "" {
		t.Log("Using shared Talos snapshot from test suite")
		// Snapshots will be used automatically via auto-build feature
		// The reconciler will find existing snapshots with matching labels
	}

	// Initialize Real Client
	hClient := hcloud_client.NewRealClient(token)

	cleaner := &ResourceCleaner{t: t}

	// Setup SSH Key
	sshKeyName, _ := setupSSHKey(t, hClient, cleaner, clusterName)
	cfg.SSHKeys = []string{sshKeyName}

	// Clean up before and after
	cleanup := func() {
		ctx := context.Background()
		logger := func(msg string) { fmt.Printf("[Cleanup] %s\n", msg) }

		// Delete Servers
		hClient.DeleteServer(ctx, clusterName+"-control-plane-1")
		hClient.DeleteServer(ctx, clusterName+"-worker-1")
		logger("Deleted Servers")

		// Delete LBs
		hClient.DeleteLoadBalancer(ctx, clusterName+"-kube-api")
		logger("Deleted LB")

		// Delete Firewalls
		hClient.DeleteFirewall(ctx, clusterName)
		logger("Deleted FW")

		// Delete Networks
		hClient.DeleteNetwork(ctx, clusterName)
		logger("Deleted Network")

		// Delete Placement Groups
		hClient.DeletePlacementGroup(ctx, clusterName+"-control-plane")
		hClient.DeletePlacementGroup(ctx, clusterName+"-worker-pg-1")
		logger("Deleted PGs")

		// Delete Certificates
		hClient.DeleteCertificate(ctx, clusterName+"-state")
		logger("Deleted Certificates")
	}
	defer cleanup()

	// Initialize Managers
	// Use real Talos generator for a "real" E2E run
	talosGen, err := talos.NewConfigGenerator(clusterName, cfg.Kubernetes.Version, cfg.Talos.Version, "", "")
	assert.NoError(t, err)

	reconciler := cluster.NewReconciler(hClient, talosGen, cfg)

	// Run Reconcile
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	kubeconfig, err := reconciler.Reconcile(ctx)
	assert.NoError(t, err)
	assert.NotEmpty(t, kubeconfig, "kubeconfig should be returned after bootstrap")

	// Verify APIs are reachable
	// We check for Talos API on one of the servers
	cp1IP, err := hClient.GetServerIP(ctx, clusterName+"-control-plane-1")
	assert.NoError(t, err)
	assert.NotEmpty(t, cp1IP)

	// Kube API through Load Balancer
	lb, err := hClient.GetLoadBalancer(ctx, clusterName+"-kube-api")
	assert.NoError(t, err)
	lbIP := lb.PublicNet.IPv4.IP.String()
	assert.NotEmpty(t, lbIP)

	t.Logf("Verifying APIs: Talos=%s:50000, Kube=%s:6443", cp1IP, lbIP)

	// Connectivity Check: Talos API
	if err := WaitForPort(context.Background(), cp1IP, 50000, 2*time.Minute); err != nil {
		t.Errorf("Talos API not reachable: %v", err)
	} else {
		t.Log("Talos API is reachable!")
	}

	// Connectivity Check: Kube API
	if err := WaitForPort(context.Background(), lbIP, 6443, 10*time.Minute); err != nil {
		t.Errorf("Kube API not reachable: %v", err)
	} else {
		t.Log("Kube API is reachable!")
	}

	// Verify cluster with kubectl
	t.Log("Verifying cluster with kubectl...")
	kubeconfigPath := fmt.Sprintf("/tmp/kubeconfig-%s", clusterName)
	if err := os.WriteFile(kubeconfigPath, kubeconfig, 0600); err != nil {
		t.Errorf("Failed to write kubeconfig: %v", err)
	} else {
		defer os.Remove(kubeconfigPath)

		// Try kubectl get nodes (with retries as cluster might still be initializing)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		nodeCheckSuccess := false
		for {
			select {
			case <-ctx.Done():
				if !nodeCheckSuccess {
					t.Error("Timeout waiting for kubectl get nodes to succeed")
				}
				goto endNodeCheck
			case <-ticker.C:
				// Run kubectl get nodes
				cmd := exec.CommandContext(context.Background(), "kubectl",
					"--kubeconfig", kubeconfigPath,
					"get", "nodes", "-o", "json")
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Logf("kubectl get nodes not ready yet: %v (will retry)", err)
					continue
				}

				// Parse output to count nodes
				var nodeList struct {
					Items []map[string]interface{} `json:"items"`
				}
				if err := json.Unmarshal(output, &nodeList); err != nil {
					t.Logf("Failed to parse kubectl output: %v (will retry)", err)
					continue
				}

				if len(nodeList.Items) >= 2 { // 1 control plane + 1 worker
					t.Logf("✓ kubectl verified: %d nodes found", len(nodeList.Items))
					nodeCheckSuccess = true
					goto endNodeCheck
				}
				t.Logf("Waiting for nodes to appear... (found %d, expecting >= 2)", len(nodeList.Items))
			}
		}
		endNodeCheck:

		// Verify CCM is installed and running
		t.Log("Verifying Hetzner Cloud Controller Manager (CCM) installation...")

		// Check if CCM deployment exists
		cmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "deployment", "-n", "kube-system", "hcloud-cloud-controller-manager", "-o", "json")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("CCM deployment not found: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ CCM deployment exists")
		}

		// Check if hcloud secret exists
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "secret", "-n", "kube-system", "hcloud", "-o", "json")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("hcloud secret not found: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ hcloud secret exists")

			// Verify secret contains required keys
			var secret struct {
				Data map[string]string `json:"data"`
			}
			if err := json.Unmarshal(output, &secret); err != nil {
				t.Errorf("Failed to parse secret: %v", err)
			} else {
				if _, ok := secret.Data["token"]; !ok {
					t.Error("hcloud secret missing 'token' key")
				}
				if _, ok := secret.Data["network"]; !ok {
					t.Error("hcloud secret missing 'network' key")
				}
				if len(secret.Data) >= 2 {
					t.Log("✓ hcloud secret contains required keys")
				}
			}
		}
	}

	// Verify Resources using Interface Getters

	// Check Network
	net, err := hClient.GetNetwork(ctx, clusterName)
	assert.NoError(t, err)
	assert.NotNil(t, net)

	// Check Firewall
	fw, err := hClient.GetFirewall(ctx, clusterName)
	assert.NoError(t, err)
	assert.NotNil(t, fw)

	// Check LB
	lb, err = hClient.GetLoadBalancer(ctx, clusterName+"-kube-api")
	assert.NoError(t, err)
	assert.NotNil(t, lb)

	// Check Servers
	srvID, err := hClient.GetServerID(ctx, clusterName+"-control-plane-1")
	assert.NoError(t, err)
	assert.NotEmpty(t, srvID)
}
