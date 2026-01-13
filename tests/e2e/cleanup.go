//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"hcloud-k8s/internal/platform/hcloud"
)

// cleanupE2ECluster removes all resources created during the E2E test using label-based cleanup.
// This is much more robust than name-based cleanup as it handles all resources with the test-id label.
func cleanupE2ECluster(t *testing.T, state *E2EState) {
	if state == nil || state.Client == nil {
		t.Log("[Cleanup] No state to clean up")
		return
	}

	t.Logf("[Cleanup] Starting label-based cleanup for test-id: %s", state.TestID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Use label-based cleanup to delete all resources with the test-id label
	labelSelector := map[string]string{
		"test-id": state.TestID,
	}

	if err := state.Client.CleanupByLabel(ctx, labelSelector); err != nil {
		t.Logf("[Cleanup] Warning: Label-based cleanup encountered errors: %v", err)
	}

	// Cleanup snapshots separately if needed (they may not have labels in older runs)
	if os.Getenv("E2E_KEEP_SNAPSHOTS") != "true" {
		t.Log("[Cleanup] Deleting snapshots...")
		if state.SnapshotAMD64 != "" {
			deleteSnapshot(ctx, t, state.Client, state.SnapshotAMD64)
		}
		if state.SnapshotARM64 != "" {
			deleteSnapshot(ctx, t, state.Client, state.SnapshotARM64)
		}
	} else {
		t.Log("[Cleanup] Keeping snapshots (E2E_KEEP_SNAPSHOTS=true)")
	}

	// Cleanup temporary files
	if state.KubeconfigPath != "" {
		os.Remove(state.KubeconfigPath)
	}
	if state.TalosSecretsPath != "" {
		os.Remove(state.TalosSecretsPath)
	}

	t.Log("[Cleanup] Cleanup complete")
}

// Helper functions for cleanup - log errors but don't fail the test

func deleteServer(ctx context.Context, t *testing.T, client *hcloud.RealClient, name string) {
	if err := client.DeleteServer(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete server %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted server: %s", name)
	}
}

func deleteLoadBalancer(ctx context.Context, t *testing.T, client *hcloud.RealClient, name string) {
	if err := client.DeleteLoadBalancer(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete load balancer %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted load balancer: %s", name)
	}
}

func deleteFirewall(ctx context.Context, t *testing.T, client *hcloud.RealClient, name string) {
	if err := client.DeleteFirewall(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete firewall %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted firewall: %s", name)
	}
}

func deleteNetwork(ctx context.Context, t *testing.T, client *hcloud.RealClient, name string) {
	if err := client.DeleteNetwork(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete network %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted network: %s", name)
	}
}

func deletePlacementGroup(ctx context.Context, t *testing.T, client *hcloud.RealClient, name string) {
	if err := client.DeletePlacementGroup(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete placement group %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted placement group: %s", name)
	}
}

func deleteCertificate(ctx context.Context, t *testing.T, client *hcloud.RealClient, name string) {
	if err := client.DeleteCertificate(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete certificate %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted certificate: %s", name)
	}
}

func deleteSSHKey(ctx context.Context, t *testing.T, client *hcloud.RealClient, name string) {
	if err := client.DeleteSSHKey(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete SSH key %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted SSH key: %s", name)
	}
}

func deleteSnapshot(ctx context.Context, t *testing.T, client *hcloud.RealClient, id string) {
	if err := client.DeleteImage(ctx, id); err != nil {
		t.Logf("  [Cleanup] Failed to delete snapshot %s (may not exist): %v", id, err)
	} else {
		t.Logf("  [Cleanup] Deleted snapshot: %s", id)
	}
}
