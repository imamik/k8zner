//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/config"
	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/orchestration"
	"github.com/imamik/k8zner/internal/platform/talos"
)

// phaseScale tests scaling the cluster by adding more control plane and worker nodes.
// This is Phase 4 of the E2E lifecycle.
func phaseScale(t *testing.T, state *E2EState) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	t.Log("Testing cluster scale-out...")

	// Load Talos secrets
	secrets, err := talos.GetOrGenerateSecrets(state.TalosSecretsPath, talosVersion)
	if err != nil {
		t.Fatalf("Failed to load Talos secrets: %v", err)
	}

	talosGen := talos.NewGenerator(state.ClusterName, k8sVersion, talosVersion, "", secrets)

	// Create scaled config (3 CP nodes, 2 worker pools)
	cfg := createScaledClusterConfig(state)

	// Run reconciler to scale cluster
	reconciler := orchestration.NewReconciler(state.Client, talosGen, cfg)

	t.Log("Scaling cluster to 3 control plane nodes and 2 worker pools...")
	startTime := time.Now()
	_, err = reconciler.Reconcile(ctx)
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("Cluster scale-out failed after %v: %v", duration, err)
	}

	t.Logf("✓ Cluster scaled in %v", duration)

	// Verify scaled resources
	verifyScaledCluster(t, state)

	state.ScaledOut = true
	t.Log("✓ Phase 4: Scale (cluster scaled successfully)")
}

// createScaledClusterConfig creates config with 3 CP nodes (HA mode) and 3 workers.
// Uses the simplified v2 config format and expands it to full internal config.
func createScaledClusterConfig(state *E2EState) *config.Config {
	// Create simplified v2 config (HA mode = 3 CP)
	v2Cfg := &v2.Config{
		Name:   state.ClusterName,
		Region: v2.RegionNuremberg,
		Mode:   v2.ModeHA, // Scale to HA mode (3 control planes)
		Workers: v2.Worker{
			Count: 3, // Scale workers to 3
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

// verifyScaledCluster verifies the scaled cluster has expected resources.
// In HA mode: 3 control planes + 3 workers = 6 total nodes
// Naming convention: {cluster}-{poolName}-{index}
func verifyScaledCluster(t *testing.T, state *E2EState) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Verify control plane nodes exist (HA mode = 3)
	// Pool name: "control-plane" (from v2 config)
	for i := 1; i <= 3; i++ {
		serverName := fmt.Sprintf("%s-control-plane-%d", state.ClusterName, i)
		if _, err := state.Client.GetServerIP(ctx, serverName); err != nil {
			t.Errorf("Control plane node %d not found: %v", i, err)
		} else {
			t.Logf("  ✓ Control plane node %d exists", i)
		}
	}

	// Verify worker nodes exist (scaled to 3)
	// Pool name: "workers" (from v2 config)
	for i := 1; i <= 3; i++ {
		serverName := fmt.Sprintf("%s-workers-%d", state.ClusterName, i)
		if _, err := state.Client.GetServerIP(ctx, serverName); err != nil {
			t.Errorf("Worker node %d not found: %v", i, err)
		} else {
			t.Logf("  ✓ Worker node %d exists", i)
		}
	}

	// Verify kubectl shows all nodes (3 CP + 3 workers = 6 total)
	t.Log("Verifying all nodes are visible in Kubernetes...")
	verifyNodeCount(t, state.KubeconfigPath, 6, 5*time.Minute)

	// Verify etcd cluster health (3 members)
	t.Log("Verifying etcd cluster has 3 members...")
	// This would require Talos API access - simplified for now
	t.Log("  ✓ etcd verification skipped (would require Talos API)")

	t.Log("✓ Scaled cluster verified")
}

// verifyNodeCount waits for kubectl to show expected number of nodes.
func verifyNodeCount(t *testing.T, kubeconfigPath string, expectedCount int, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for %d nodes", expectedCount)
		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "nodes", "--no-headers")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("  kubectl not ready yet: %v (will retry)", err)
				continue
			}

			lines := 0
			if len(output) > 0 {
				for _, b := range output {
					if b == '\n' {
						lines++
					}
				}
			}

			if lines >= expectedCount {
				t.Logf("  ✓ %d nodes found", lines)
				return
			}
			t.Logf("  Waiting for nodes... (found %d, expecting %d)", lines, expectedCount)
		}
	}
}
