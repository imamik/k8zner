//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/imagebuilder"
)

// TestSnapshotCreation tests the snapshot build process from scratch.
// This test ALWAYS builds fresh snapshots (ignores cached ones) to ensure
// the image builder is working correctly and can catch regressions.
//
// These test snapshots use unique names (with timestamp suffix) to avoid conflicts
// with the production snapshots cached in TestMain.
//
// Set E2E_SKIP_SNAPSHOT_BUILD_TEST=true to skip this test for faster local dev,
// but it should ALWAYS run in CI to catch snapshot creation issues.
func TestSnapshotCreation(t *testing.T) {
	t.Parallel() // Run in parallel with other tests

	// Allow skipping this test for local dev speed, but warn about it
	if os.Getenv("E2E_SKIP_SNAPSHOT_BUILD_TEST") == "true" {
		t.Skip("Skipping snapshot build test (E2E_SKIP_SNAPSHOT_BUILD_TEST=true) - NOTE: This should NOT be skipped in CI!")
	}

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	client := hcloud.NewRealClient(token)
	builder := image.NewBuilder(client, nil)

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

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()

			cleaner := &ResourceCleaner{t: t}

			// Use unique snapshot name with timestamp to avoid conflicts with production snapshots
			// Production snapshots (in TestMain): talos-v1.8.3-k8s-v1.31.0-{arch}
			// Test snapshots: talos-v1.8.3-k8s-v1.31.0-{arch}-test-{timestamp}
			testTimestamp := time.Now().Unix()
			labels := map[string]string{
				"os":            "talos",
				"talos-version": "v1.8.3",
				"k8s-version":   "v1.31.0",
				"arch":          tc.arch,
				"type":          "e2e-snapshot-build-test",
				"created_by":    "hcloud-k8s-e2e",
				"test_name":     "TestSnapshotCreation",
				"test_run":      fmt.Sprintf("%d", testTimestamp),
			}

			t.Logf("Building FRESH %s test snapshot (timestamp: %d) to validate build process...", tc.arch, testTimestamp)
			startTime := time.Now()

			snapshotID, err := builder.Build(ctx, "v1.8.3", "v1.31.0", tc.arch, "nbg1", labels)

			buildDuration := time.Since(startTime)

			// Always clean up test snapshots (don't cache these)
			if snapshotID != "" {
				cleaner.Add(func() {
					t.Logf("Deleting test snapshot %s...", snapshotID)
					if err := client.DeleteImage(context.Background(), snapshotID); err != nil {
						t.Errorf("Failed to delete test snapshot %s: %v", snapshotID, err)
					}
				})
			}

			if err != nil {
				t.Fatalf("Snapshot build FAILED for %s: %v", tc.arch, err)
			}

			t.Logf("✓ Snapshot build SUCCEEDED for %s in %v (ID: %s)", tc.arch, buildDuration, snapshotID)

			// Verify the snapshot by creating a server from it
			verifyServerName := fmt.Sprintf("snapshot-verify-%s-%s", tc.arch, time.Now().Format("20060102-150405"))

			cleaner.Add(func() {
				t.Logf("Deleting verification server %s...", verifyServerName)
				if err := client.DeleteServer(context.Background(), verifyServerName); err != nil {
					t.Logf("Failed to delete verification server (might not exist): %v", err)
				}
			})

			t.Logf("Creating verification server from fresh snapshot...")

			serverType := "cpx22"
			if tc.arch == "arm64" {
				serverType = "cax11"
			}

			verifyLabels := map[string]string{
				"type":       "e2e-snapshot-verify",
				"created_by": "hcloud-k8s-e2e",
				"test_name":  "TestSnapshotCreation",
				"arch":       tc.arch,
			}

			verifyKeyName, _ := setupSSHKey(t, client, cleaner, verifyServerName)

			_, err = client.CreateServer(ctx, verifyServerName, snapshotID, serverType, "", []string{verifyKeyName}, verifyLabels, "", nil, 0, "")
			if err != nil {
				t.Fatalf("Failed to create verification server: %v", err)
			}

			ip, err := client.GetServerIP(ctx, verifyServerName)
			if err != nil {
				t.Fatalf("Failed to get verification server IP: %v", err)
			}

			// Verify Talos API is accessible
			t.Logf("Verifying Talos API on %s:50000...", ip)
			err = WaitForPort(ctx, ip, 50000, 5*time.Minute)
			if err != nil {
				t.Fatalf("Talos API verification FAILED: %v", err)
			}

			t.Logf("✓ SNAPSHOT BUILD TEST PASSED: %s snapshot builds correctly and boots Talos successfully", tc.arch)
		})
	}
}
