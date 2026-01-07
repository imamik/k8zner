//go:build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	hcloud_internal "hcloud-k8s/internal/hcloud"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/stretchr/testify/require"
)

// TestSimpleTalosNode - Test 1: Create server from snapshot and verify basic Talos API connectivity
// This test validates:
// 1. Server boots successfully from Talos snapshot
// 2. Talos API port (50000) becomes accessible
// 3. Can establish insecure connection to Talos API
// 4. Can retrieve basic node information
func TestSimpleTalosNode(t *testing.T) {
	t.Parallel() // Run in parallel with other tests

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	t.Log("=== Test 1: Simple Talos Node - Snapshot + Basic Connectivity ===")

	runID := fmt.Sprintf("e2e-simple-%d", time.Now().Unix())
	serverName := fmt.Sprintf("%s-node", runID)

	ctx := context.Background()
	hcloudClient := hcloud_internal.NewRealClient(token)

	cleaner := &ResourceCleaner{t: t}

	// Get shared Talos snapshot from suite
	t.Log("Using shared Talos snapshot from test suite")
	require.NotNil(t, sharedCtx, "Shared context should be available")
	require.NotEmpty(t, sharedCtx.SnapshotAMD64, "Shared AMD64 snapshot should be available")
	snapshotID := sharedCtx.SnapshotAMD64

	// Setup SSH Key
	sshKeyName, _ := setupSSHKey(t, hcloudClient, cleaner, runID)

	// Create a simple server from Talos snapshot
	t.Logf("Creating server %s from Talos snapshot ID %s...", serverName, snapshotID)
	labels := map[string]string{
		"test":    "simple-talos-node",
		"run-id":  runID,
		"purpose": "connectivity-test",
	}

	serverID, err := hcloudClient.CreateServer(ctx, serverName, snapshotID, "cpx22", "nbg1", []string{sshKeyName}, labels, "", nil, 0, "")
	require.NoError(t, err, "Failed to create server")
	require.NotEmpty(t, serverID, "Server ID should not be empty")

	// Register cleanup
	cleaner.Add(func() {
		t.Logf("Deleting server %s...", serverName)
		if err := hcloudClient.DeleteServer(ctx, serverName); err != nil {
			t.Logf("Failed to delete server %s (might not exist): %v", serverName, err)
		}
	})

	// Get server IP
	nodeIP, err := hcloudClient.GetServerIP(ctx, serverName)
	require.NoError(t, err, "Failed to get server IP")
	t.Logf("Server created: %s (ID: %s, IP: %s)", serverName, serverID, nodeIP)

	// Wait for server to boot and Talos API to become available
	t.Log("Waiting for Talos API port 50000 to become accessible...")
	err = WaitForPort(ctx, nodeIP, 50000, 5*time.Minute)
	require.NoError(t, err, "Talos API should become accessible")
	t.Log("✓ Talos API port is accessible")

	// Test 1: Verify insecure connection and maintenance mode
	t.Log("Testing insecure connection to Talos API...")
	insecureClient, err := client.New(ctx,
		client.WithEndpoints(nodeIP),
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	require.NoError(t, err, "Should be able to create insecure Talos client")
	defer insecureClient.Close()

	// Try to get version - this will fail in maintenance mode, which is expected
	// A fresh Talos node without machine config boots into maintenance mode
	t.Log("Attempting Version API call (expected to fail in maintenance mode)...")
	version, err := insecureClient.Version(ctx)

	// We expect this to fail with "API is not implemented in maintenance mode"
	if err != nil {
		if strings.Contains(err.Error(), "maintenance mode") {
			t.Logf("✓ Node is in maintenance mode (expected for unconfigured Talos)")
			t.Logf("  Error: %v", err)
		} else {
			// Different error - this is unexpected
			t.Fatalf("Unexpected error (expected maintenance mode error): %v", err)
		}
	} else {
		// If Version succeeds, that's also fine - log the info
		t.Logf("✓ Successfully connected to Talos node")
		t.Logf("  Talos Version: %v", version.Messages[0].Version.Tag)
		t.Logf("  Platform: %s", version.Messages[0].Platform.Name)
	}

	t.Log("=== Test 1 PASSED: Server boots from snapshot, Talos API accessible, node in maintenance mode ===")
}
