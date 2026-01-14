//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"hcloud-k8s/internal/platform/hcloud"
)

// TestE2ELifecycle runs the complete E2E test lifecycle sequentially.
// This test provisions a full Kubernetes cluster on Hetzner Cloud and validates
// all functionality from snapshots through addons and scaling.
//
// Phases:
//  1. Snapshots - Build and verify Talos snapshots
//  2. Cluster - Provision infrastructure and bootstrap Kubernetes
//  3. Addons - Install and test each addon sequentially
//  4. Scale - Scale cluster and verify operation
//
// Environment variables:
//
//	HCLOUD_TOKEN - Required
//	E2E_KEEP_SNAPSHOTS - Set to "true" to cache snapshots between runs
//	E2E_SKIP_SCALE - Set to "true" to skip scale testing (faster)
func TestE2ELifecycle(t *testing.T) {
	// This test is inherently sequential and long-running
	// Do not mark as parallel

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	// Create unique cluster name
	clusterName := fmt.Sprintf("e2e-seq-%d", time.Now().Unix())
	t.Logf("Starting E2E lifecycle test for cluster: %s", clusterName)

	// Initialize state
	client := hcloud.NewRealClient(token)
	state := NewE2EState(clusterName, client)

	// Register cleanup to run at end (or on failure/timeout)
	defer func() {
		cleanupE2ECluster(t, state)
		// Verify cleanup completed successfully
		verifyCleanupComplete(t, state)
	}()

	// Phase 1: Snapshots
	t.Run("Phase1_Snapshots", func(t *testing.T) {
		phaseSnapshots(t, state)
	})

	// Phase 2: Cluster Provisioning
	t.Run("Phase2_Cluster", func(t *testing.T) {
		phaseCluster(t, state)
	})

	// Phase 3: Addons
	t.Run("Phase3_Addons", func(t *testing.T) {
		phaseAddons(t, state)
	})

	// Phase 4: Scale (optional, can be skipped for faster testing)
	if os.Getenv("E2E_SKIP_SCALE") != "true" {
		t.Run("Phase4_Scale", func(t *testing.T) {
			phaseScale(t, state)
		})
	} else {
		t.Log("Skipping Phase 4: Scale (E2E_SKIP_SCALE=true)")
	}

	t.Log("✓ E2E Lifecycle Complete")
	t.Logf("Cluster: %s", clusterName)
	t.Logf("Snapshots: amd64=%s, arm64=%s", state.SnapshotAMD64, state.SnapshotARM64)
	t.Logf("Load Balancer: %s", state.LoadBalancerIP)
	t.Logf("Addons installed: %v", getInstalledAddonsList(state))
}

// getInstalledAddonsList returns a list of installed addon names.
func getInstalledAddonsList(state *E2EState) []string {
	addons := []string{}
	for name, installed := range state.AddonsInstalled {
		if installed {
			addons = append(addons, name)
		}
	}
	return addons
}
