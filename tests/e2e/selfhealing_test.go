//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/platform/hcloud"
)

// TestE2ESelfHealingHA is a dedicated E2E test for self-healing on HA clusters.
// This test requires an HA cluster (3 CPs, 2+ workers) and tests:
// - Worker node failure and automatic replacement
// - (Future) Control plane node failure and replacement with quorum preservation
//
// Run with: HCLOUD_TOKEN=xxx go test -v -timeout=60m -tags=e2e -run TestE2ESelfHealingHA ./tests/e2e/...
func TestE2ESelfHealingHA(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping self-healing test in short mode")
	}

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	clusterName := fmt.Sprintf("e2e-selfheal-%d", time.Now().Unix())
	t.Logf("=== Starting Self-Healing HA E2E Test: %s ===", clusterName)

	client := hcloud.NewRealClient(token)
	state := NewE2EState(clusterName, client)
	defer cleanupE2ECluster(t, state)

	// === PHASE 1: DEPLOY HA CLUSTER ===
	t.Log("=== DEPLOYMENT PHASE ===")

	// Get snapshot
	deployGetSnapshot(t, state)

	// Deploy HA cluster
	cfg := deployHACluster(t, state, token)

	// Install addons
	deployAllAddons(t, state, cfg, token)

	t.Log("=== DEPLOYMENT COMPLETE ===")

	// === PHASE 2: SELF-HEALING TEST ===
	t.Log("=== SELF-HEALING TEST PHASE ===")

	phaseSelfHealing(t, state)

	t.Log("=== SELF-HEALING HA E2E TEST PASSED ===")
}
