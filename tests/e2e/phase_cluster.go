//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"k8zner/internal/config"
	"k8zner/internal/orchestration"
	"k8zner/internal/platform/hcloud"
	"k8zner/internal/platform/talos"
	"k8zner/internal/util/keygen"
)

// phaseCluster provisions the complete cluster infrastructure and bootstraps Kubernetes.
// This is Phase 2 of the E2E lifecycle and includes:
// - Network, Firewall, Load Balancer, Placement Groups
// - Control Plane node(s)
// - Worker node(s)
// - Kubernetes bootstrap
func phaseCluster(t *testing.T, state *E2EState) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	t.Log("Provisioning full cluster (infrastructure + nodes + bootstrap)...")

	// Setup SSH key
	if err := setupSSHKeyForCluster(ctx, t, state); err != nil {
		t.Fatalf("Failed to setup SSH key: %v", err)
	}

	// Create config
	cfg := createInitialClusterConfig(state)

	// Generate Talos secrets
	secrets, err := talos.GetOrGenerateSecrets("/tmp/talos-secrets-"+state.ClusterName+".json", talosVersion)
	if err != nil {
		t.Fatalf("Failed to generate Talos secrets: %v", err)
	}
	state.TalosSecretsPath = "/tmp/talos-secrets-" + state.ClusterName + ".json"

	talosGen := talos.NewGenerator(state.ClusterName, k8sVersion, talosVersion, "", secrets)

	// Get Talos config for diagnostics
	state.TalosConfig, err = talosGen.GetClientConfig()
	if err != nil {
		t.Logf("Warning: Could not get Talos config: %v", err)
	}

	// Create reconciler and provision cluster
	reconciler := orchestration.NewReconciler(state.Client, talosGen, cfg)

	t.Log("Starting cluster reconciliation...")
	startTime := time.Now()
	kubeconfig, err := reconciler.Reconcile(ctx)
	duration := time.Since(startTime)

	if err != nil {
		// Run diagnostics before failing
		t.Logf("Cluster provisioning failed after %v: %v", duration, err)
		runClusterDiagnostics(ctx, t, state)
		t.Fatalf("Cluster provisioning failed: %v", err)
	}

	state.Kubeconfig = kubeconfig
	t.Logf("✓ Cluster provisioned in %v", duration)

	// Save kubeconfig to file
	kubeconfigPath := fmt.Sprintf("/tmp/kubeconfig-%s", state.ClusterName)
	if err := os.WriteFile(kubeconfigPath, kubeconfig, 0600); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}
	state.KubeconfigPath = kubeconfigPath

	// Get cluster IPs for state
	cpIP, err := state.Client.GetServerIP(ctx, state.ClusterName+"-control-plane-1")
	if err == nil {
		state.ControlPlaneIPs = append(state.ControlPlaneIPs, cpIP)
	}

	workerIP, err := state.Client.GetServerIP(ctx, state.ClusterName+"-worker-1-1")
	if err == nil {
		state.WorkerIPs = append(state.WorkerIPs, workerIP)
	}

	lb, err := state.Client.GetLoadBalancer(ctx, state.ClusterName+"-kube-api")
	if err == nil && lb != nil {
		state.LoadBalancerIP = hcloud.LoadBalancerIPv4(lb)
	}

	// Verify cluster is accessible
	t.Log("Verifying cluster API accessibility...")
	verifyClusterAPI(ctx, t, state)

	// Wait for nodes to be ready
	t.Log("Waiting for nodes to be ready...")
	waitForNodesReady(t, state.KubeconfigPath, 2, 5*time.Minute)

	// Verify RDNS configuration
	t.Log("Verifying RDNS configuration...")
	verifyServerRDNS(ctx, t, state)
	verifyLoadBalancerRDNS(ctx, t, state)

	t.Log("✓ Phase 2: Cluster (provisioned and ready)")
}

// setupSSHKeyForCluster generates and uploads an SSH key.
func setupSSHKeyForCluster(ctx context.Context, t *testing.T, state *E2EState) error {
	keyName := fmt.Sprintf("%s-key-%d", state.ClusterName, time.Now().UnixNano())

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	labels := map[string]string{
		"cluster": state.ClusterName,
		"test-id": state.TestID,
	}

	_, err = state.Client.CreateSSHKey(ctx, keyName, string(keyPair.PublicKey), labels)
	if err != nil {
		return fmt.Errorf("failed to upload SSH key: %w", err)
	}

	state.SSHKeyName = keyName
	state.SSHPrivateKey = keyPair.PrivateKey
	return nil
}

// createInitialClusterConfig creates the initial cluster config with 1 CP + 1 worker.
func createInitialClusterConfig(state *E2EState) *config.Config {
	cfg := &config.Config{
		ClusterName: state.ClusterName,
		TestID:      state.TestID,
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
				Name:       "worker-1",
				ServerType: "cpx22",
				Location:   "nbg1",
				Count:      1,
				Image:      "talos",
			},
		},
		Talos: config.TalosConfig{
			Version: talosVersion,
		},
		Kubernetes: config.KubernetesConfig{
			Version: k8sVersion,
		},
		SSHKeys: []string{state.SSHKeyName},
	}

	cfg.CalculateSubnets()
	return cfg
}

// verifyClusterAPI verifies Talos and Kubernetes APIs are accessible.
func verifyClusterAPI(ctx context.Context, t *testing.T, state *E2EState) {
	if len(state.ControlPlaneIPs) > 0 {
		cpIP := state.ControlPlaneIPs[0]
		if err := WaitForPort(ctx, cpIP, 50000, 2*time.Minute); err != nil {
			t.Errorf("Talos API not reachable on %s:50000: %v", cpIP, err)
		} else {
			t.Logf("✓ Talos API reachable on %s:50000", cpIP)
		}
	}

	if state.LoadBalancerIP != "" {
		if err := WaitForPort(ctx, state.LoadBalancerIP, 6443, 10*time.Minute); err != nil {
			t.Errorf("Kubernetes API not reachable on %s:6443: %v", state.LoadBalancerIP, err)
		} else {
			t.Logf("✓ Kubernetes API reachable on %s:6443", state.LoadBalancerIP)
		}
	}
}

// waitForNodesReady waits for kubectl to report all nodes as ready.
func waitForNodesReady(t *testing.T, kubeconfigPath string, expectedCount int, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("Timeout waiting for nodes to be ready")
		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "nodes", "--no-headers")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("kubectl not ready yet: %v (will retry)", err)
				continue
			}

			// Count lines in output
			lines := 0
			if len(output) > 0 {
				for _, b := range output {
					if b == '\n' {
						lines++
					}
				}
			}

			if lines >= expectedCount {
				t.Logf("✓ %d nodes ready", lines)
				return
			}
			t.Logf("Waiting for nodes... (found %d, expecting %d)", lines, expectedCount)
		}
	}
}

// runClusterDiagnostics runs diagnostic checks when cluster provisioning fails.
func runClusterDiagnostics(ctx context.Context, t *testing.T, state *E2EState) {
	if len(state.ControlPlaneIPs) == 0 {
		t.Log("No control plane IPs available for diagnostics")
		return
	}

	diag := NewClusterDiagnostics(t, state.ControlPlaneIPs[0], state.LoadBalancerIP, state.TalosConfig)
	diag.RunFullDiagnostics(ctx)
}
