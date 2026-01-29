//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/config"
	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/orchestration"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/platform/talos"
	"github.com/imamik/k8zner/internal/util/keygen"
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

	// Verify firewall is applied to servers
	t.Log("Verifying firewall is applied to servers...")
	verifyFirewallApplied(ctx, t, state)

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
// Uses the simplified v2 config format and expands it to full internal config.
func createInitialClusterConfig(state *E2EState) *config.Config {
	// Create simplified v2 config (dev mode = 1 CP, 1 worker)
	v2Cfg := &v2.Config{
		Name:   state.ClusterName,
		Region: v2.RegionNuremberg,
		Mode:   v2.ModeDev,
		Workers: v2.Worker{
			Count: 1,
			Size:  v2.SizeCX22,
		},
	}

	// Expand to internal config
	cfg, err := v2.Expand(v2Cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to expand v2 config: %v", err))
	}

	// Set e2e-specific fields
	cfg.TestID = state.TestID
	cfg.SSHKeys = []string{state.SSHKeyName}

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

// verifyFirewallApplied verifies that the firewall is applied to all cluster servers.
func verifyFirewallApplied(ctx context.Context, t *testing.T, state *E2EState) {
	// Get the firewall
	fw, err := state.Client.GetFirewall(ctx, state.ClusterName)
	if err != nil {
		t.Fatalf("Failed to get firewall: %v", err)
	}
	if fw == nil {
		t.Fatal("Firewall not found")
	}

	t.Logf("Firewall %s (ID: %d) found", fw.Name, fw.ID)

	// Check that the firewall has a label selector applied
	expectedSelector := fmt.Sprintf("cluster=%s", state.ClusterName)
	hasLabelSelector := false
	for _, applied := range fw.AppliedTo {
		if applied.Type == "label_selector" && applied.LabelSelector != nil {
			t.Logf("  Applied to label selector: %s", applied.LabelSelector.Selector)
			if applied.LabelSelector.Selector == expectedSelector {
				hasLabelSelector = true
			}
		}
		if applied.Type == "server" && applied.Server != nil {
			t.Logf("  Applied to server ID: %d", applied.Server.ID)
		}
	}

	if !hasLabelSelector {
		t.Errorf("Firewall not applied to label selector %q", expectedSelector)
	} else {
		t.Logf("✓ Firewall applied to label selector: %s", expectedSelector)
	}

	// Verify servers have the firewall applied
	servers, err := state.Client.GetServersByLabel(ctx, map[string]string{"cluster": state.ClusterName})
	if err != nil {
		t.Fatalf("Failed to get servers: %v", err)
	}

	if len(servers) == 0 {
		t.Fatal("No servers found with cluster label")
	}

	for _, server := range servers {
		serverHasFirewall := false
		for _, serverFw := range server.PublicNet.Firewalls {
			if serverFw.Firewall.ID == fw.ID {
				serverHasFirewall = true
				break
			}
		}
		if !serverHasFirewall {
			t.Errorf("Server %s (ID: %d) does not have firewall applied", server.Name, server.ID)
		} else {
			t.Logf("✓ Server %s has firewall applied", server.Name)
		}
	}
}
