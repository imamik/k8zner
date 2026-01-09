//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	hcloud_client "hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning/image"
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

// buildSharedSnapshots builds Talos snapshots for both amd64 and arm64 IN PARALLEL.
func buildSharedSnapshots(client *hcloud_client.RealClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	builder := image.NewBuilder(client)

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

	// Check for existing snapshots FIRST
	log.Println("Checking for existing snapshots...")
	existingAMD64, err := client.GetSnapshotByLabels(ctx, labelsAMD64)
	if err != nil {
		return fmt.Errorf("failed to check for existing amd64 snapshot: %w", err)
	}

	existingARM64, err := client.GetSnapshotByLabels(ctx, labelsARM64)
	if err != nil {
		return fmt.Errorf("failed to check for existing arm64 snapshot: %w", err)
	}

	// Track what needs to be built
	needAMD64 := existingAMD64 == nil
	needARM64 := existingARM64 == nil

	// Report existing snapshots
	if existingAMD64 != nil {
		log.Printf("Found existing amd64 snapshot: %s (ID: %d)", existingAMD64.Description, existingAMD64.ID)
		sharedCtx.SnapshotAMD64 = fmt.Sprintf("%d", existingAMD64.ID)
	}
	if existingARM64 != nil {
		log.Printf("Found existing arm64 snapshot: %s (ID: %d)", existingARM64.Description, existingARM64.ID)
		sharedCtx.SnapshotARM64 = fmt.Sprintf("%d", existingARM64.ID)
	}

	// If nothing needs building, we're done
	if !needAMD64 && !needARM64 {
		log.Println("All snapshots already exist, skipping build")
		return nil
	}

	// Build missing snapshots IN PARALLEL
	log.Printf("=== BUILDING SNAPSHOTS IN PARALLEL (amd64=%v, arm64=%v) ===", needAMD64, needARM64)

	type buildResult struct {
		arch       string
		snapshotID string
		err        error
	}

	buildCount := 0
	if needAMD64 {
		buildCount++
	}
	if needARM64 {
		buildCount++
	}

	resultChan := make(chan buildResult, buildCount)

	// Start AMD64 build if needed
	if needAMD64 {
		go func() {
			log.Printf("[amd64] Starting build at %s", time.Now().Format("15:04:05"))
			snapshotID, err := builder.Build(ctx, "v1.8.3", "v1.31.0", "amd64", "nbg1", labelsAMD64)
			if err != nil {
				log.Printf("[amd64] Build failed at %s: %v", time.Now().Format("15:04:05"), err)
				resultChan <- buildResult{arch: "amd64", err: err}
			} else {
				log.Printf("[amd64] Build completed at %s: %s", time.Now().Format("15:04:05"), snapshotID)
				resultChan <- buildResult{arch: "amd64", snapshotID: snapshotID}
			}
		}()
	}

	// Start ARM64 build if needed
	if needARM64 {
		go func() {
			log.Printf("[arm64] Starting build at %s", time.Now().Format("15:04:05"))
			snapshotID, err := builder.Build(ctx, "v1.8.3", "v1.31.0", "arm64", "nbg1", labelsARM64)
			if err != nil {
				log.Printf("[arm64] Build failed at %s: %v", time.Now().Format("15:04:05"), err)
				resultChan <- buildResult{arch: "arm64", err: err}
			} else {
				log.Printf("[arm64] Build completed at %s: %s", time.Now().Format("15:04:05"), snapshotID)
				resultChan <- buildResult{arch: "arm64", snapshotID: snapshotID}
			}
		}()
	}

	// Wait for all builds to complete
	var buildErrors []error
	for i := 0; i < buildCount; i++ {
		result := <-resultChan
		if result.err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("%s: %w", result.arch, result.err))
		} else {
			if result.arch == "amd64" {
				sharedCtx.SnapshotAMD64 = result.snapshotID
			} else {
				sharedCtx.SnapshotARM64 = result.snapshotID
			}
		}
	}

	if len(buildErrors) > 0 {
		return fmt.Errorf("failed to build snapshots: %v", buildErrors)
	}

	log.Println("=== ALL SNAPSHOTS BUILT SUCCESSFULLY ===")
	return nil
}

// cleanupSharedSnapshots removes the shared snapshots after all tests complete.
// Set E2E_KEEP_SNAPSHOTS=true to skip cleanup for faster local development.
func cleanupSharedSnapshots(client *hcloud_client.RealClient) {
	// Check if snapshots should be kept for faster subsequent runs
	if os.Getenv("E2E_KEEP_SNAPSHOTS") == "true" {
		log.Println("=== Skipping snapshot cleanup (E2E_KEEP_SNAPSHOTS=true) ===")
		log.Printf("Keeping snapshots: amd64=%s, arm64=%s", sharedCtx.SnapshotAMD64, sharedCtx.SnapshotARM64)
		log.Println("Note: Snapshots will be reused in subsequent test runs")
		return
	}

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
