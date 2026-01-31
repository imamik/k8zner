//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	realhcloud "github.com/imamik/k8zner/internal/platform/hcloud"
)

// cleanupE2ECluster removes all resources created during the E2E test using label-based cleanup.
// This function:
// 1. Performs label-based cleanup of all Hetzner Cloud resources
// 2. Verifies that no resources remain
// 3. Attempts a second cleanup if resources are found
// 4. Fails the test if resources remain after cleanup (to alert about cost leakage)
func cleanupE2ECluster(t *testing.T, state *E2EState) {
	if state == nil || state.Client == nil {
		t.Log("[Cleanup] No state to clean up")
		return
	}

	t.Logf("[Cleanup] Starting label-based cleanup for test-id: %s", state.TestID)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Use label-based cleanup to delete all resources with the test-id label
	labelSelector := map[string]string{
		"test-id": state.TestID,
	}

	// First cleanup pass
	if err := state.Client.CleanupByLabel(ctx, labelSelector); err != nil {
		t.Logf("[Cleanup] Warning: First cleanup pass encountered errors: %v", err)
	}

	// Wait a moment for resources to be fully deleted
	time.Sleep(5 * time.Second)

	// Verify cleanup - count remaining resources
	remaining, err := state.Client.CountResourcesByLabel(ctx, labelSelector)
	if err != nil {
		t.Logf("[Cleanup] Warning: Failed to count remaining resources: %v", err)
	}

	// If resources remain, try a second cleanup pass
	if remaining.Total() > 0 {
		t.Logf("[Cleanup] Warning: Found %d remaining resources after first cleanup: %s", remaining.Total(), remaining.String())
		t.Log("[Cleanup] Attempting second cleanup pass...")

		// Wait longer before second pass - resources may still be in transition
		time.Sleep(30 * time.Second)

		if err := state.Client.CleanupByLabel(ctx, labelSelector); err != nil {
			t.Logf("[Cleanup] Warning: Second cleanup pass encountered errors: %v", err)
		}

		// Wait and re-verify
		time.Sleep(10 * time.Second)
		remaining, err = state.Client.CountResourcesByLabel(ctx, labelSelector)
		if err != nil {
			t.Logf("[Cleanup] Warning: Failed to count remaining resources: %v", err)
		}
	}

	// Cleanup snapshot separately if needed (it may not have labels in older runs)
	// Skip if snapshot came from sharedCtx (TestMain handles cleanup)
	if os.Getenv("E2E_KEEP_SNAPSHOTS") != "true" {
		// Only delete snapshot if it was built by this test (not from sharedCtx)
		snapshotFromSharedCtx := sharedCtx != nil && state.SnapshotAMD64 == sharedCtx.SnapshotAMD64

		if snapshotFromSharedCtx {
			t.Log("[Cleanup] Snapshot from sharedCtx - TestMain will clean it up")
		} else if state.SnapshotAMD64 != "" {
			t.Log("[Cleanup] Deleting snapshot...")
			deleteSnapshot(ctx, t, state.Client, state.SnapshotAMD64)
		}
	} else {
		t.Log("[Cleanup] Keeping snapshot (E2E_KEEP_SNAPSHOTS=true)")
	}

	// Cleanup temporary files
	if state.KubeconfigPath != "" {
		os.Remove(state.KubeconfigPath)
	}
	if state.TalosSecretsPath != "" {
		os.Remove(state.TalosSecretsPath)
	}

	// Final verification - FAIL the test if resources remain
	// This is critical to prevent cost leakage from orphaned resources
	if remaining.Total() > 0 {
		t.Errorf("[Cleanup] CRITICAL: %d resources remain after cleanup: %s. These will incur costs!", remaining.Total(), remaining.String())
		t.Log("[Cleanup] Remaining resources details:")
		logRemainingResources(ctx, t, state.Client, labelSelector)
	} else {
		t.Log("[Cleanup] Cleanup verification passed - all resources deleted")
	}
}

// logRemainingResources logs detailed information about remaining resources for debugging.
func logRemainingResources(ctx context.Context, t *testing.T, client *realhcloud.RealClient, labels map[string]string) {
	hc := client.HCloudClient()
	labelString := buildLabelSelectorString(labels)

	// Log remaining servers
	servers, _ := hc.Server.AllWithOpts(ctx, hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelString},
	})
	for _, s := range servers {
		t.Logf("  - Server: %s (ID: %d, Status: %s)", s.Name, s.ID, s.Status)
	}

	// Log remaining volumes
	volumes, _ := hc.Volume.AllWithOpts(ctx, hcloud.VolumeListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelString},
	})
	for _, v := range volumes {
		serverName := "detached"
		if v.Server != nil {
			serverName = v.Server.Name
		}
		t.Logf("  - Volume: %s (ID: %d, Size: %dGB, Server: %s)", v.Name, v.ID, v.Size, serverName)
	}

	// Log remaining load balancers
	lbs, _ := hc.LoadBalancer.AllWithOpts(ctx, hcloud.LoadBalancerListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelString},
	})
	for _, lb := range lbs {
		t.Logf("  - LoadBalancer: %s (ID: %d)", lb.Name, lb.ID)
	}

	// Log remaining SSH keys
	keys, _ := hc.SSHKey.AllWithOpts(ctx, hcloud.SSHKeyListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelString},
	})
	for _, k := range keys {
		t.Logf("  - SSHKey: %s (ID: %d)", k.Name, k.ID)
	}

	// Log remaining certificates
	certs, _ := hc.Certificate.AllWithOpts(ctx, hcloud.CertificateListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelString},
	})
	for _, c := range certs {
		t.Logf("  - Certificate: %s (ID: %d)", c.Name, c.ID)
	}

	// Log remaining firewalls
	fws, _ := hc.Firewall.AllWithOpts(ctx, hcloud.FirewallListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelString},
	})
	for _, fw := range fws {
		t.Logf("  - Firewall: %s (ID: %d)", fw.Name, fw.ID)
	}

	// Log remaining networks
	networks, _ := hc.Network.AllWithOpts(ctx, hcloud.NetworkListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelString},
	})
	for _, n := range networks {
		t.Logf("  - Network: %s (ID: %d)", n.Name, n.ID)
	}

	// Log remaining placement groups
	pgs, _ := hc.PlacementGroup.AllWithOpts(ctx, hcloud.PlacementGroupListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: labelString},
	})
	for _, pg := range pgs {
		t.Logf("  - PlacementGroup: %s (ID: %d)", pg.Name, pg.ID)
	}
}

// buildLabelSelectorString converts a map of labels to a Hetzner Cloud label selector string.
func buildLabelSelectorString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	selector := ""
	for k, v := range labels {
		if selector != "" {
			selector += ","
		}
		selector += k + "=" + v
	}
	return selector
}

// Helper functions for cleanup - log errors but don't fail the test

func deleteServer(ctx context.Context, t *testing.T, client *realhcloud.RealClient, name string) {
	if err := client.DeleteServer(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete server %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted server: %s", name)
	}
}

func deleteLoadBalancer(ctx context.Context, t *testing.T, client *realhcloud.RealClient, name string) {
	if err := client.DeleteLoadBalancer(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete load balancer %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted load balancer: %s", name)
	}
}

func deleteFirewall(ctx context.Context, t *testing.T, client *realhcloud.RealClient, name string) {
	if err := client.DeleteFirewall(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete firewall %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted firewall: %s", name)
	}
}

func deleteNetwork(ctx context.Context, t *testing.T, client *realhcloud.RealClient, name string) {
	if err := client.DeleteNetwork(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete network %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted network: %s", name)
	}
}

func deletePlacementGroup(ctx context.Context, t *testing.T, client *realhcloud.RealClient, name string) {
	if err := client.DeletePlacementGroup(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete placement group %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted placement group: %s", name)
	}
}

func deleteCertificate(ctx context.Context, t *testing.T, client *realhcloud.RealClient, name string) {
	if err := client.DeleteCertificate(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete certificate %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted certificate: %s", name)
	}
}

func deleteSSHKey(ctx context.Context, t *testing.T, client *realhcloud.RealClient, name string) {
	if err := client.DeleteSSHKey(ctx, name); err != nil {
		t.Logf("  [Cleanup] Failed to delete SSH key %s (may not exist): %v", name, err)
	} else {
		t.Logf("  [Cleanup] Deleted SSH key: %s", name)
	}
}

func deleteSnapshot(ctx context.Context, t *testing.T, client *realhcloud.RealClient, id string) {
	if err := client.DeleteImage(ctx, id); err != nil {
		t.Logf("  [Cleanup] Failed to delete snapshot %s (may not exist): %v", id, err)
	} else {
		t.Logf("  [Cleanup] Deleted snapshot: %s", id)
	}
}
