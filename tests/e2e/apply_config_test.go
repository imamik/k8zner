//go:build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"testing"
	"time"

	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
	talos_config "github.com/sak-d/hcloud-k8s/internal/talos"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/stretchr/testify/require"
)

// TestApplyConfig - Test 2: Apply machine configuration to a single Talos node
// This test validates:
// 1. Generating machine config for a single control plane node
// 2. Applying the config to a node in maintenance mode
// 3. Node reboots and comes back online
// 4. Can establish authenticated connection after config application
func TestApplyConfig(t *testing.T) {
	t.Parallel() // Run in parallel with other tests

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	t.Log("=== Test 2: Apply Machine Configuration ===")

	runID := fmt.Sprintf("e2e-config-%d", time.Now().Unix())
	serverName := fmt.Sprintf("%s-cp1", runID)
	clusterName := fmt.Sprintf("%s-cluster", runID)

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

	// Create a server from Talos snapshot
	t.Logf("Creating server %s from Talos snapshot ID %s...", serverName, snapshotID)
	labels := map[string]string{
		"test":    "apply-config",
		"run-id":  runID,
		"purpose": "config-application-test",
	}

	serverID, err := hcloudClient.CreateServer(ctx, serverName, snapshotID, "cpx22", "nbg1", []string{sshKeyName}, labels, "", nil, 0, "", nil)
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

	// Wait for Talos API to become available
	t.Log("Waiting for Talos API port 50000 to become accessible...")
	err = WaitForPort(ctx, nodeIP, 50000, 5*time.Minute)
	require.NoError(t, err, "Talos API should become accessible")
	t.Log("✓ Talos API port is accessible")

	// Generate machine configuration
	t.Log("Generating machine configuration...")
	// Format endpoint as https://IP:6443 (standard Kubernetes API port)
	endpoint := fmt.Sprintf("https://%s:6443", nodeIP)

	configGenerator, err := talos_config.NewConfigGenerator(
		clusterName,
		"v1.31.0", // Kubernetes version
		"v1.8.3",  // Talos version
		endpoint,  // Control plane endpoint with port
		"none",    // CNI type
		nil,       // Registry mirrors
		"",        // No existing secrets file, will generate new
	)
	require.NoError(t, err, "Should be able to create config generator")

	// Generate control plane config
	machineConfigBytes, err := configGenerator.GenerateControlPlaneConfig([]string{nodeIP})
	require.NoError(t, err, "Should be able to generate control plane config")
	require.NotEmpty(t, machineConfigBytes, "Machine config should not be empty")

	t.Logf("Generated machine config (%d bytes)", len(machineConfigBytes))

	// Get client config (talosconfig) for authenticated connection
	clientConfigBytes, err := configGenerator.GetClientConfig()
	require.NoError(t, err, "Should be able to generate client config")

	// Apply machine configuration using insecure connection (node is in maintenance mode)
	t.Log("Applying machine configuration to node (insecure connection to maintenance mode)...")
	insecureClient, err := client.New(ctx,
		client.WithEndpoints(nodeIP),
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	require.NoError(t, err, "Should be able to create insecure Talos client")
	defer insecureClient.Close()

	// Apply the configuration
	applyReq := &machine.ApplyConfigurationRequest{
		Data: machineConfigBytes,
		Mode: machine.ApplyConfigurationRequest_REBOOT,
	}
	applyResp, err := insecureClient.ApplyConfiguration(ctx, applyReq)
	require.NoError(t, err, "Should be able to apply configuration")
	require.NotNil(t, applyResp, "Apply response should not be nil")

	t.Log("✓ Configuration applied successfully")
	t.Log("Node will now reboot to apply configuration...")

	// Wait for node to reboot and come back
	t.Log("Waiting for node to reboot (60 seconds)...")
	time.Sleep(60 * time.Second)

	t.Log("Waiting for Talos API to become accessible again after reboot...")
	err = WaitForPort(ctx, nodeIP, 50000, 5*time.Minute)
	require.NoError(t, err, "Talos API should become accessible after reboot")
	t.Log("✓ Node is back online")

	// Now try authenticated connection
	t.Log("Testing authenticated connection with applied configuration...")

	// Parse client config
	cfg, err := config.FromString(string(clientConfigBytes))
	require.NoError(t, err, "Should be able to parse client config")

	authClient, err := client.New(ctx,
		client.WithConfig(cfg),
		client.WithEndpoints(nodeIP),
	)
	require.NoError(t, err, "Should be able to create authenticated Talos client")
	defer authClient.Close()

	// Try to get version with authenticated connection
	t.Log("Attempting Version API call with authentication...")
	version, err := authClient.Version(ctx)

	if err != nil {
		t.Logf("❌ Authenticated connection failed: %v", err)
		t.Logf("This is the same error we see in the bootstrap process!")

		// Try to get more details about the error
		t.Log("Attempting to diagnose the issue...")

		// Check if we can still connect insecurely
		t.Log("Trying insecure connection to check node state...")
		testInsecureClient, err := client.New(ctx,
			client.WithEndpoints(nodeIP),
			client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
		)
		if err != nil {
			t.Logf("Cannot create insecure client: %v", err)
		} else {
			defer testInsecureClient.Close()
			testVersion, testErr := testInsecureClient.Version(ctx)
			if testErr != nil {
				t.Logf("Insecure connection also fails: %v", testErr)
			} else {
				t.Logf("Insecure connection works! Version: %s", testVersion.Messages[0].Version.Tag)
				t.Log("This suggests the issue is with certificate/authentication, not the node itself")
			}
		}

		require.Fail(t, "Authenticated connection failed after config application", "Error: %v", err)
	}

	t.Logf("✓ Authenticated connection successful!")
	t.Logf("  Talos Version: %v", version.Messages[0].Version.Tag)
	t.Logf("  Platform: %s", version.Messages[0].Platform.Name)

	t.Log("=== Test 2 PASSED: Machine config applied and authenticated connection works ===")
}
