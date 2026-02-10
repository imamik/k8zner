//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning/image"
	"github.com/imamik/k8zner/internal/util/keygen"
)

// Get versions from the v2 default version matrix
// Note: Kubernetes version is used WITHOUT 'v' prefix to match provisioning code labels
var (
	versionMatrix = v2.DefaultVersionMatrix()
	talosVersion  = versionMatrix.Talos
	k8sVersion    = versionMatrix.Kubernetes // NO 'v' prefix - must match provisioning labels
)

// phaseSnapshots builds and verifies Talos snapshot for AMD64 (ARM64 not used).
// This is Phase 1 of the E2E lifecycle and must complete before other phases.
func phaseSnapshots(t *testing.T, state *E2EState) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// First, check if sharedCtx has snapshot from TestMain (avoids duplicate building)
	if sharedCtx != nil && sharedCtx.SnapshotAMD64 != "" {
		t.Logf("Using pre-built snapshot from TestMain: amd64=%s", sharedCtx.SnapshotAMD64)
		state.SnapshotAMD64 = sharedCtx.SnapshotAMD64
		t.Log("✓ Phase 1: Snapshots (using shared context)")
		return
	}

	// Check for existing snapshots if reuse is enabled
	if os.Getenv("E2E_KEEP_SNAPSHOTS") == "true" {
		if tryReuseSnapshots(ctx, t, state) {
			t.Log("✓ Phase 1: Snapshots (reused existing)")
			return
		}
	}

	t.Log("Building fresh snapshot for amd64...")

	builder := image.NewBuilder(state.Client)
	snapshotID, err := buildSnapshot(ctx, builder, "amd64", state.ClusterName, state.TestID)
	if err != nil {
		t.Fatalf("Failed to build amd64 snapshot: %v", err)
	}
	state.SnapshotAMD64 = snapshotID
	t.Logf("✓ Built amd64 snapshot: %s", snapshotID)

	// Create temporary SSH key for verification server
	verifyKeyName, cleanupKey, err := createVerificationSSHKey(ctx, t, state)
	if err != nil {
		t.Fatalf("Failed to create verification SSH key: %v", err)
	}
	defer cleanupKey()

	// Verify snapshot boots correctly
	t.Log("Verifying snapshot boots correctly...")
	verifySnapshot(ctx, t, state, "amd64", state.SnapshotAMD64, verifyKeyName)

	t.Log("✓ Phase 1: Snapshots (built and verified)")
}

// buildSnapshot builds a Talos snapshot for the given architecture.
func buildSnapshot(ctx context.Context, builder *image.Builder, arch, clusterName string, testID string) (string, error) {
	labels := map[string]string{
		"os":            "talos",
		"talos-version": talosVersion,
		"k8s-version":   k8sVersion,
		"arch":          arch,
		"e2e-cluster":   clusterName,
		"test-id":       testID,
	}

	startTime := time.Now()
	snapshotID, err := builder.Build(ctx, talosVersion, k8sVersion, arch, "", "nbg1", labels)
	if err != nil {
		return "", fmt.Errorf("snapshot build failed: %w", err)
	}

	duration := time.Since(startTime)
	fmt.Printf("[%s] Snapshot built in %v\n", arch, duration)
	return snapshotID, nil
}

// createVerificationSSHKey creates a temporary SSH key for snapshot verification.
// Returns the key name and a cleanup function to delete the key.
func createVerificationSSHKey(ctx context.Context, t *testing.T, state *E2EState) (string, func(), error) {
	keyName := fmt.Sprintf("%s-verify-key-%d", state.ClusterName, time.Now().UnixNano())

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	labels := map[string]string{
		"cluster": state.ClusterName,
		"test-id": state.TestID,
		"type":    "e2e-verify-key",
	}

	_, err = state.Client.CreateSSHKey(ctx, keyName, string(keyPair.PublicKey), labels)
	if err != nil {
		return "", nil, fmt.Errorf("failed to upload SSH key: %w", err)
	}

	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancel()
		if err := state.Client.DeleteSSHKey(cleanupCtx, keyName); err != nil {
			t.Logf("Warning: failed to delete verification SSH key %s: %v", keyName, err)
		}
	}

	return keyName, cleanup, nil
}

// verifySnapshot creates a test server from the snapshot and verifies Talos boots.
func verifySnapshot(ctx context.Context, t *testing.T, state *E2EState, arch, snapshotID, sshKeyName string) {
	serverName := fmt.Sprintf("%s-verify-%s-%d", state.ClusterName, arch, time.Now().Unix())

	serverType := "cpx22" // Must match disk size of image builder server - better availability than cx23
	if arch == "arm64" {
		serverType = "cax11"
	}

	labels := map[string]string{
		"type":        "e2e-snapshot-verify",
		"arch":        arch,
		"e2e-cluster": state.ClusterName,
		"test-id":     state.TestID,
	}

	t.Logf("Creating verification server %s from snapshot...", serverName)
	// Enable public IPv4 so we can access the Talos API for verification
	_, err := state.Client.CreateServer(ctx, hcloud.ServerCreateOpts{
		Name:             serverName,
		ImageType:        snapshotID,
		ServerType:       serverType,
		SSHKeys:          []string{sshKeyName},
		Labels:           labels,
		EnablePublicIPv4: true,
		EnablePublicIPv6: true,
	})
	if err != nil {
		t.Fatalf("Failed to create verification server: %v", err)
	}

	// Schedule cleanup with error logging
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := state.Client.DeleteServer(cleanupCtx, serverName); err != nil {
			t.Logf("Warning: failed to delete verification server %s: %v", serverName, err)
		}
	}()

	ip, err := state.Client.GetServerIP(ctx, serverName)
	if err != nil {
		t.Fatalf("Failed to get verification server IP: %v", err)
	}

	// Verify Talos API is accessible
	t.Logf("Waiting for Talos API on %s:50000...", ip)
	if err := WaitForPort(ctx, ip, 50000, 5*time.Minute); err != nil {
		t.Fatalf("Talos API not accessible: %v", err)
	}

	t.Logf("✓ %s snapshot verified (Talos API responding)", arch)
}

// tryReuseSnapshots attempts to find and reuse existing AMD64 snapshot.
func tryReuseSnapshots(ctx context.Context, t *testing.T, state *E2EState) bool {
	labelsAMD64 := map[string]string{
		"os":            "talos",
		"talos-version": talosVersion,
		"k8s-version":   k8sVersion,
		"arch":          "amd64",
	}

	amd64Snapshot, err := state.Client.GetSnapshotByLabels(ctx, labelsAMD64)
	if err != nil || amd64Snapshot == nil {
		return false
	}

	state.SnapshotAMD64 = fmt.Sprintf("%d", amd64Snapshot.ID)
	t.Logf("Reusing existing snapshot: amd64=%s", state.SnapshotAMD64)
	return true
}
