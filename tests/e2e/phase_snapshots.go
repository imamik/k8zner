//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/provisioning/image"
	"github.com/imamik/k8zner/internal/util/keygen"
)

const (
	talosVersion = "v1.8.3"
	k8sVersion   = "v1.31.0"
)

// phaseSnapshots builds and verifies Talos snapshots for both architectures.
// This is Phase 1 of the E2E lifecycle and must complete before other phases.
func phaseSnapshots(t *testing.T, state *E2EState) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Check for existing snapshots if reuse is enabled
	if os.Getenv("E2E_KEEP_SNAPSHOTS") == "true" {
		if tryReuseSnapshots(ctx, t, state) {
			t.Log("✓ Phase 1: Snapshots (reused existing)")
			return
		}
	}

	t.Log("Building fresh snapshots for amd64 and arm64...")

	// Build both snapshots in parallel for speed
	type result struct {
		arch       string
		snapshotID string
		err        error
	}

	resultChan := make(chan result, 2)
	builder := image.NewBuilder(state.Client)

	// Build amd64
	go func() {
		snapshotID, err := buildSnapshot(ctx, builder, "amd64", state.ClusterName, state.TestID)
		resultChan <- result{arch: "amd64", snapshotID: snapshotID, err: err}
	}()

	// Build arm64
	go func() {
		snapshotID, err := buildSnapshot(ctx, builder, "arm64", state.ClusterName, state.TestID)
		resultChan <- result{arch: "arm64", snapshotID: snapshotID, err: err}
	}()

	// Collect results
	for i := 0; i < 2; i++ {
		res := <-resultChan
		if res.err != nil {
			t.Fatalf("Failed to build %s snapshot: %v", res.arch, res.err)
		}
		if res.arch == "amd64" {
			state.SnapshotAMD64 = res.snapshotID
		} else {
			state.SnapshotARM64 = res.snapshotID
		}
		t.Logf("✓ Built %s snapshot: %s", res.arch, res.snapshotID)
	}

	// Create temporary SSH key for verification servers
	verifyKeyName, cleanupKey, err := createVerificationSSHKey(ctx, t, state)
	if err != nil {
		t.Fatalf("Failed to create verification SSH key: %v", err)
	}
	defer cleanupKey()

	// Verify both snapshots boot correctly
	t.Log("Verifying snapshots boot correctly...")
	verifySnapshot(ctx, t, state, "amd64", state.SnapshotAMD64, verifyKeyName)
	verifySnapshot(ctx, t, state, "arm64", state.SnapshotARM64, verifyKeyName)

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

	serverType := "cpx22"
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
	// E2E tests need public IPv4 for SSH verification
	_, err := state.Client.CreateServer(ctx, serverName, snapshotID, serverType, "", []string{sshKeyName}, labels, "", nil, 0, "", true, true)
	if err != nil {
		t.Fatalf("Failed to create verification server: %v", err)
	}

	// Schedule cleanup
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		state.Client.DeleteServer(cleanupCtx, serverName)
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

// tryReuseSnapshots attempts to find and reuse existing snapshots.
func tryReuseSnapshots(ctx context.Context, t *testing.T, state *E2EState) bool {
	labelsAMD64 := map[string]string{
		"os":            "talos",
		"talos-version": talosVersion,
		"k8s-version":   k8sVersion,
		"arch":          "amd64",
	}

	labelsARM64 := map[string]string{
		"os":            "talos",
		"talos-version": talosVersion,
		"k8s-version":   k8sVersion,
		"arch":          "arm64",
	}

	amd64Snapshot, err := state.Client.GetSnapshotByLabels(ctx, labelsAMD64)
	if err != nil || amd64Snapshot == nil {
		return false
	}

	arm64Snapshot, err := state.Client.GetSnapshotByLabels(ctx, labelsARM64)
	if err != nil || arm64Snapshot == nil {
		return false
	}

	state.SnapshotAMD64 = fmt.Sprintf("%d", amd64Snapshot.ID)
	state.SnapshotARM64 = fmt.Sprintf("%d", arm64Snapshot.ID)

	t.Logf("Reusing existing snapshots: amd64=%s, arm64=%s", state.SnapshotAMD64, state.SnapshotARM64)
	return true
}
