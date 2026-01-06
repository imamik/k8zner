//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/cluster"
	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/talos"
)

// TestParallelProvisioning tests cluster provisioning with MULTIPLE servers to validate parallelization.
// This test provisions:
// - 3 control plane servers (should be created in parallel)
// - 2 worker pools with 2 servers each (pools and servers should be created in parallel)
func TestParallelProvisioning(t *testing.T) {
	t.Parallel() // Run in parallel with other tests

	if sharedCtx == nil || sharedCtx.Client == nil {
		t.Skip("Shared context not initialized")
	}

	t.Log("=== Testing PARALLEL provisioning with MULTIPLE servers ===")

	clusterName := fmt.Sprintf("e2e-parallel-%d", time.Now().Unix())
	t.Logf("Cluster name: %s", clusterName)

	cleaner := &ResourceCleaner{t: t}

	// Setup SSH key
	sshKeyName, _ := setupSSHKey(t, sharedCtx.Client, cleaner, clusterName)

	// Create config with MULTIPLE servers
	cfg := &config.Config{
		ClusterName: clusterName,
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
					Count:      3, // ← 3 SERVERS TO TEST PARALLELIZATION
					ServerType: "cpx22",
					Location:   "nbg1",
					Image:      "talos",
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "worker-pool-1",
				Count:      2, // ← 2 SERVERS
				ServerType: "cpx22",
				Location:   "nbg1",
				Image:      "talos",
			},
			{
				Name:       "worker-pool-2",
				Count:      2, // ← 2 SERVERS
				ServerType: "cpx22",
				Location:   "nbg1",
				Image:      "talos",
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.8.3",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.31.0",
		},
		SSHKeys: []string{sshKeyName},
	}

	if err := cfg.CalculateSubnets(); err != nil {
		t.Fatalf("Failed to calculate subnets: %v", err)
	}

	// Create Talos generator
	talosGen, err := talos.NewConfigGenerator(cfg.ClusterName, cfg.Kubernetes.Version, cfg.Talos.Version, "", "")
	if err != nil {
		t.Fatalf("Failed to create Talos generator: %v", err)
	}

	// Create reconciler
	reconciler := cluster.NewReconciler(sharedCtx.Client, talosGen, cfg)

	// Run reconciliation
	t.Log("Starting reconciliation with MULTIPLE servers...")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	startTime := time.Now()
	err = reconciler.Reconcile(ctx)
	duration := time.Since(startTime)

	t.Logf("Reconciliation took: %s", duration)

	// Cleanup
	cleanup := func() {
		ctx := context.Background()
		log.Printf("[Cleanup] Deleting resources for %s...", clusterName)

		// Delete Servers - all 7 servers (3 CP + 2x2 workers)
		for i := 1; i <= 3; i++ {
			sharedCtx.Client.DeleteServer(ctx, fmt.Sprintf("%s-control-plane-%d", clusterName, i))
		}
		sharedCtx.Client.DeleteServer(ctx, clusterName+"-worker-pool-1-1")
		sharedCtx.Client.DeleteServer(ctx, clusterName+"-worker-pool-1-2")
		sharedCtx.Client.DeleteServer(ctx, clusterName+"-worker-pool-2-1")
		sharedCtx.Client.DeleteServer(ctx, clusterName+"-worker-pool-2-2")
		log.Println("[Cleanup] Deleted Servers")

		// Delete LBs
		sharedCtx.Client.DeleteLoadBalancer(ctx, clusterName+"-kube-api")
		log.Println("[Cleanup] Deleted LB")

		// Delete Firewalls
		sharedCtx.Client.DeleteFirewall(ctx, clusterName)
		log.Println("[Cleanup] Deleted FW")

		// Delete Networks
		sharedCtx.Client.DeleteNetwork(ctx, clusterName)
		log.Println("[Cleanup] Deleted Network")

		// Delete Placement Groups
		sharedCtx.Client.DeletePlacementGroup(ctx, clusterName+"-control-plane")
		log.Println("[Cleanup] Deleted PGs")
	}
	defer cleanup()

	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	t.Logf("✓ Cluster provisioned successfully in %s", duration)
	t.Log("Review the logs above to see:")
	t.Log("  - Infrastructure components starting at the SAME timestamp")
	t.Log("  - 3 control plane servers starting at the SAME timestamp")
	t.Log("  - 2 worker pools starting at the SAME timestamp")
	t.Log("  - Within each pool, servers starting at the SAME timestamp")
}
