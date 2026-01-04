//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	hcloud_client "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/image"
)

// SharedTestContext holds resources shared across E2E tests.
type SharedTestContext struct {
	SnapshotAMD64 string // Snapshot ID for amd64
	SnapshotARM64 string // Snapshot ID for arm64
	Client        *hcloud_client.RealClient
}

var sharedCtx *SharedTestContext

// TestMain orchestrates E2E tests to run sequentially and manage shared resources.
func TestMain(m *testing.M) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		log.Println("HCLOUD_TOKEN not set, skipping E2E tests")
		os.Exit(0)
	}

	client := hcloud_client.NewRealClient(token)
	sharedCtx = &SharedTestContext{
		Client: client,
	}

	// Build Talos snapshots before running tests
	log.Println("=== Building Talos snapshots for E2E tests ===")
	if err := buildSharedSnapshots(client); err != nil {
		log.Printf("Failed to build snapshots: %v", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup snapshots after all tests
	log.Println("=== Cleaning up shared snapshots ===")
	cleanupSharedSnapshots(client)

	os.Exit(code)
}

// buildSharedSnapshots builds Talos snapshots for both amd64 and arm64.
func buildSharedSnapshots(client *hcloud_client.RealClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	builder := image.NewBuilder(client, nil)

	// Check if snapshots already exist
	labelsAMD64 := map[string]string{
		"os":            "talos",
		"talos-version": "v1.8.3",
		"k8s-version":   "v1.31.0",
		"arch":          "amd64",
		"e2e-shared":    "true",
	}

	labelsARM64 := map[string]string{
		"os":            "talos",
		"talos-version": "v1.8.3",
		"k8s-version":   "v1.31.0",
		"arch":          "arm64",
		"e2e-shared":    "true",
	}

	// Check for existing amd64 snapshot
	existingAMD64, err := client.GetSnapshotByLabels(ctx, labelsAMD64)
	if err != nil {
		return fmt.Errorf("failed to check for existing amd64 snapshot: %w", err)
	}

	if existingAMD64 != nil {
		log.Printf("Found existing amd64 snapshot: %s (ID: %d)", existingAMD64.Description, existingAMD64.ID)
		sharedCtx.SnapshotAMD64 = fmt.Sprintf("%d", existingAMD64.ID)
	} else {
		log.Println("Building amd64 snapshot...")
		snapshotID, err := builder.Build(ctx, "v1.8.3", "v1.31.0", "amd64", labelsAMD64)
		if err != nil {
			return fmt.Errorf("failed to build amd64 snapshot: %w", err)
		}
		sharedCtx.SnapshotAMD64 = snapshotID
		log.Printf("Built amd64 snapshot: %s", snapshotID)
	}

	// Check for existing arm64 snapshot
	existingARM64, err := client.GetSnapshotByLabels(ctx, labelsARM64)
	if err != nil {
		return fmt.Errorf("failed to check for existing arm64 snapshot: %w", err)
	}

	if existingARM64 != nil {
		log.Printf("Found existing arm64 snapshot: %s (ID: %d)", existingARM64.Description, existingARM64.ID)
		sharedCtx.SnapshotARM64 = fmt.Sprintf("%d", existingARM64.ID)
	} else {
		log.Println("Building arm64 snapshot...")
		snapshotID, err := builder.Build(ctx, "v1.8.3", "v1.31.0", "arm64", labelsARM64)
		if err != nil {
			return fmt.Errorf("failed to build arm64 snapshot: %w", err)
		}
		sharedCtx.SnapshotARM64 = snapshotID
		log.Printf("Built arm64 snapshot: %s", snapshotID)
	}

	return nil
}

// cleanupSharedSnapshots removes the shared snapshots after all tests complete.
func cleanupSharedSnapshots(client *hcloud_client.RealClient) {
	ctx := context.Background()

	if sharedCtx.SnapshotAMD64 != "" {
		log.Printf("Deleting amd64 snapshot %s...", sharedCtx.SnapshotAMD64)
		if err := client.DeleteImage(ctx, sharedCtx.SnapshotAMD64); err != nil {
			log.Printf("Failed to delete amd64 snapshot: %v", err)
		}
	}

	if sharedCtx.SnapshotARM64 != "" {
		log.Printf("Deleting arm64 snapshot %s...", sharedCtx.SnapshotARM64)
		if err := client.DeleteImage(ctx, sharedCtx.SnapshotARM64); err != nil {
			log.Printf("Failed to delete arm64 snapshot: %v", err)
		}
	}
}
