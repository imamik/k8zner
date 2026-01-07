//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"hcloud-k8s/internal/hcloud"
)

// TestImageBuildLifecycle verifies that servers can boot from Talos snapshots
// and that the Talos API is accessible.
//
// NOTE: This test uses CACHED snapshots from TestMain for speed.
// For testing the snapshot BUILD process itself, see TestSnapshotCreation.
func TestImageBuildLifecycle(t *testing.T) {
	t.Parallel() // Run in parallel with other tests

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	client := hcloud.NewRealClient(token)

	tests := []struct {
		name string
		arch string
	}{
		{name: "amd64", arch: "amd64"},
		{name: "arm64", arch: "arm64"},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // Run amd64 and arm64 tests in parallel

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			cleaner := &ResourceCleaner{t: t}

			// Use shared snapshot from TestMain (required)
			var snapshotID string
			if sharedCtx == nil {
				t.Fatal("Shared context not initialized")
			}

			if tc.arch == "amd64" {
				snapshotID = sharedCtx.SnapshotAMD64
				t.Logf("Using shared amd64 snapshot: %s", snapshotID)
			} else if tc.arch == "arm64" {
				snapshotID = sharedCtx.SnapshotARM64
				t.Logf("Using shared arm64 snapshot: %s", snapshotID)
			}

			if snapshotID == "" {
				t.Fatalf("No shared snapshot available for %s", tc.arch)
			}

			// Verify Server Creation from snapshot
			verifyServerName := fmt.Sprintf("verify-%s-%s", tc.arch, time.Now().Format("20060102-150405"))

			// Register cleanup for server immediately, so even if CreateServer fails partially or we crash, we try to delete it.
			cleaner.Add(func() {
				t.Logf("Deleting verification server %s...", verifyServerName)
				if err := client.DeleteServer(context.Background(), verifyServerName); err != nil {
					// It's okay if it doesn't exist.
					t.Logf("Failed to delete verification server (might not exist): %v", err)
				}
			})

			t.Logf("Creating verification server %s...", verifyServerName)

			serverType := "cpx22" // 80 GB disk, enough for Talos snapshot
			if tc.arch == "arm64" {
				serverType = "cax11"
			}

			verifyLabels := map[string]string{
				"type":       "e2e-verify",
				"created_by": "hcloud-k8s-e2e",
				"test_name":  "TestImageBuildLifecycle",
				"arch":       tc.arch,
			}

			// Setup SSH Key for verification
			verifyKeyName, _ := setupSSHKey(t, client, cleaner, verifyServerName)

			// We pass the ssh key to prevent password emails
			_, err := client.CreateServer(ctx, verifyServerName, snapshotID, serverType, "", []string{verifyKeyName}, verifyLabels, "", nil, 0, "")
			if err != nil {
				t.Fatalf("Failed to create verification server: %v", err)
			}

			ip, err := client.GetServerIP(ctx, verifyServerName)
			if err != nil {
				t.Fatalf("Failed to get verification server IP: %v", err)
			}
			t.Logf("Verification server IP: %s", ip)

			// 4. Verify Talos is running (Port 50000)
			t.Logf("Waiting for Talos API (port 50000) on %s...", ip)
			err = WaitForPort(ctx, ip, 50000, 5*time.Minute)
			if err != nil {
				t.Fatalf("Failed to verify Talos API: %v", err)
			}
			t.Logf("Successfully established connection with Talos API on %s:50000", ip)
		})
	}
}
