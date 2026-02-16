//go:build e2e

package e2e

import (
	"context"
	"fmt"
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

	// Verify cleanup - count remaining resources using raw hcloud client
	hc := newHCloudAPIClient()
	labelString := buildLabelSelectorString(labelSelector)
	remaining := countResources(ctx, hc, labelString)

	// If resources remain, try a second cleanup pass
	if remaining > 0 {
		t.Logf("[Cleanup] Warning: Found %d remaining resources after first cleanup", remaining)
		t.Log("[Cleanup] Attempting second cleanup pass...")

		// Wait longer before second pass - resources may still be in transition
		time.Sleep(30 * time.Second)

		if err := state.Client.CleanupByLabel(ctx, labelSelector); err != nil {
			t.Logf("[Cleanup] Warning: Second cleanup pass encountered errors: %v", err)
		}

		// Wait and re-verify
		time.Sleep(10 * time.Second)
		remaining = countResources(ctx, hc, labelString)
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
	if remaining > 0 {
		t.Errorf("[Cleanup] CRITICAL: %d resources remain after cleanup. These will incur costs!", remaining)
		t.Log("[Cleanup] Remaining resources details:")
		logRemainingResources(ctx, t, hc, labelString)
	} else {
		t.Log("[Cleanup] Cleanup verification passed - all resources deleted")
	}
}

// newHCloudAPIClient creates a raw hcloud client from the HCLOUD_TOKEN env var.
func newHCloudAPIClient() *hcloud.Client {
	return hcloud.NewClient(hcloud.WithToken(os.Getenv("HCLOUD_TOKEN")))
}

// countResources counts all resources matching the given label selector.
func countResources(ctx context.Context, hc *hcloud.Client, labelString string) int {
	opts := hcloud.ListOpts{LabelSelector: labelString}
	total := 0

	servers, _ := hc.Server.AllWithOpts(ctx, hcloud.ServerListOpts{ListOpts: opts})
	total += len(servers)
	volumes, _ := hc.Volume.AllWithOpts(ctx, hcloud.VolumeListOpts{ListOpts: opts})
	total += len(volumes)
	lbs, _ := hc.LoadBalancer.AllWithOpts(ctx, hcloud.LoadBalancerListOpts{ListOpts: opts})
	total += len(lbs)
	fws, _ := hc.Firewall.AllWithOpts(ctx, hcloud.FirewallListOpts{ListOpts: opts})
	total += len(fws)
	nets, _ := hc.Network.AllWithOpts(ctx, hcloud.NetworkListOpts{ListOpts: opts})
	total += len(nets)
	pgs, _ := hc.PlacementGroup.AllWithOpts(ctx, hcloud.PlacementGroupListOpts{ListOpts: opts})
	total += len(pgs)
	keys, _ := hc.SSHKey.AllWithOpts(ctx, hcloud.SSHKeyListOpts{ListOpts: opts})
	total += len(keys)
	certs, _ := hc.Certificate.AllWithOpts(ctx, hcloud.CertificateListOpts{ListOpts: opts})
	total += len(certs)

	return total
}

// logRemainingResources logs detailed information about remaining resources for debugging.
func logRemainingResources(ctx context.Context, t *testing.T, hc *hcloud.Client, labelString string) {
	opts := hcloud.ListOpts{LabelSelector: labelString}

	servers, _ := hc.Server.AllWithOpts(ctx, hcloud.ServerListOpts{ListOpts: opts})
	for _, s := range servers {
		t.Logf("  - Server: %s (ID: %d, Status: %s)", s.Name, s.ID, s.Status)
	}

	volumes, _ := hc.Volume.AllWithOpts(ctx, hcloud.VolumeListOpts{ListOpts: opts})
	for _, v := range volumes {
		serverName := "detached"
		if v.Server != nil {
			serverName = v.Server.Name
		}
		t.Logf("  - Volume: %s (ID: %d, Size: %dGB, Server: %s)", v.Name, v.ID, v.Size, serverName)
	}

	lbs, _ := hc.LoadBalancer.AllWithOpts(ctx, hcloud.LoadBalancerListOpts{ListOpts: opts})
	for _, lb := range lbs {
		t.Logf("  - LoadBalancer: %s (ID: %d)", lb.Name, lb.ID)
	}

	keys, _ := hc.SSHKey.AllWithOpts(ctx, hcloud.SSHKeyListOpts{ListOpts: opts})
	for _, k := range keys {
		t.Logf("  - SSHKey: %s (ID: %d)", k.Name, k.ID)
	}

	certs, _ := hc.Certificate.AllWithOpts(ctx, hcloud.CertificateListOpts{ListOpts: opts})
	for _, c := range certs {
		t.Logf("  - Certificate: %s (ID: %d)", c.Name, c.ID)
	}

	fws, _ := hc.Firewall.AllWithOpts(ctx, hcloud.FirewallListOpts{ListOpts: opts})
	for _, fw := range fws {
		t.Logf("  - Firewall: %s (ID: %d)", fw.Name, fw.ID)
	}

	nets, _ := hc.Network.AllWithOpts(ctx, hcloud.NetworkListOpts{ListOpts: opts})
	for _, n := range nets {
		t.Logf("  - Network: %s (ID: %d)", n.Name, n.ID)
	}

	pgs, _ := hc.PlacementGroup.AllWithOpts(ctx, hcloud.PlacementGroupListOpts{ListOpts: opts})
	for _, pg := range pgs {
		t.Logf("  - PlacementGroup: %s (ID: %d)", pg.Name, pg.ID)
	}
}

// buildLabelSelectorString converts a map of labels to a Hetzner Cloud label selector string.
func buildLabelSelectorString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	selector := parts[0]
	for _, p := range parts[1:] {
		selector += "," + p
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
