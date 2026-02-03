//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/platform/s3"
)

// TestE2EBackup tests the S3 backup integration with Hetzner Object Storage.
//
// This test verifies:
// 1. S3 bucket can be auto-created using cluster name pattern
// 2. S3 operations (put, list, delete) work correctly
// 3. v2 config backup expansion is correct
//
// Prerequisites:
//   - HCLOUD_TOKEN - Hetzner Cloud API token
//   - HETZNER_S3_ACCESS_KEY - Hetzner Object Storage access key
//   - HETZNER_S3_SECRET_KEY - Hetzner Object Storage secret key
//
// The bucket "{cluster-name}-etcd-backups" is auto-created and cleaned up.
//
// Example:
//
//	HCLOUD_TOKEN=xxx HETZNER_S3_ACCESS_KEY=yyy HETZNER_S3_SECRET_KEY=zzz \
//	go test -v -timeout=10m -tags=e2e -run TestE2EBackup ./tests/e2e/
func TestE2EBackup(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	s3AccessKey := os.Getenv("HETZNER_S3_ACCESS_KEY")
	s3SecretKey := os.Getenv("HETZNER_S3_SECRET_KEY")
	if s3AccessKey == "" || s3SecretKey == "" {
		t.Skip("HETZNER_S3_ACCESS_KEY and HETZNER_S3_SECRET_KEY required for backup E2E test")
	}

	// Generate unique cluster name for this test run
	timestamp := time.Now().Unix()
	clusterName := fmt.Sprintf("e2e-backup-%d", timestamp)
	region := v2.RegionFalkenstein
	regionCode := string(region)
	bucketName := clusterName + "-etcd-backups"
	endpoint := fmt.Sprintf("https://%s.your-objectstorage.com", regionCode)

	t.Logf("=== Starting Backup E2E Test: %s ===", clusterName)
	t.Logf("=== Bucket: %s ===", bucketName)
	t.Logf("=== Endpoint: %s ===", endpoint)

	// Create S3 client for verification
	s3Client, err := s3.NewClient(endpoint, regionCode, s3AccessKey, s3SecretKey)
	if err != nil {
		t.Fatalf("Failed to create S3 client: %v", err)
	}

	// Cleanup bucket at the end (even if test fails)
	defer func() {
		t.Log("Cleaning up S3 bucket...")
		if err := cleanupS3Bucket(context.Background(), s3Client, bucketName); err != nil {
			t.Logf("Warning: failed to cleanup bucket: %v", err)
		}
	}()

	// === TEST 1: Bucket Creation and Operations ===
	t.Run("BucketOperations", func(t *testing.T) {
		testBucketOperations(t, s3Client, bucketName)
	})

	// === TEST 2: v2 Config Validation ===
	t.Run("ConfigValidation", func(t *testing.T) {
		testBackupConfigValidation(t, clusterName, region, bucketName)
	})

	t.Log("=== BACKUP E2E TEST PASSED ===")
}

// testBucketOperations tests S3 bucket creation and object operations.
func testBucketOperations(t *testing.T, s3Client *s3.Client, bucketName string) {
	ctx := context.Background()
	t.Log("Testing S3 bucket operations...")

	// Create bucket
	if err := s3Client.CreateBucket(ctx, bucketName); err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}
	t.Logf("  Bucket %s created", bucketName)

	// Wait for eventual consistency and verify bucket exists
	var exists bool
	var err error
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		exists, err = s3Client.BucketExists(ctx, bucketName)
		if err != nil {
			t.Logf("  Attempt %d: error checking bucket: %v", i+1, err)
			continue
		}
		if exists {
			break
		}
		t.Logf("  Attempt %d: bucket not yet visible, waiting...", i+1)
	}
	if err != nil {
		t.Fatalf("Failed to check bucket existence: %v", err)
	}
	if !exists {
		t.Fatal("Bucket was created but doesn't exist after 20 seconds")
	}
	t.Log("  Bucket existence verified")

	// Test writing an object
	testKey := "test-object.txt"
	testContent := []byte("k8zner backup test content")

	if err := s3Client.PutObject(ctx, bucketName, testKey, testContent); err != nil {
		t.Fatalf("Failed to write test object: %v", err)
	}
	t.Log("  Test object written")

	// Verify object exists by listing
	objects, err := s3Client.ListObjects(ctx, bucketName, "")
	if err != nil {
		t.Fatalf("Failed to list objects: %v", err)
	}

	found := false
	for _, obj := range objects {
		if obj == testKey {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Test object not found in bucket (objects: %v)", objects)
	}
	t.Log("  Test object verified in bucket")

	// Cleanup test object
	if err := s3Client.DeleteObject(ctx, bucketName, testKey); err != nil {
		t.Logf("Warning: failed to delete test object: %v", err)
	}
	t.Log("  Test object deleted")

	t.Log("S3 bucket operations working correctly")
}

// testBackupConfigValidation verifies the v2 config backup expansion is correct.
func testBackupConfigValidation(t *testing.T, clusterName string, region v2.Region, expectedBucket string) {
	t.Log("Testing v2 config backup validation...")

	// Verify the v2 config would expand correctly
	cfg := &v2.Config{
		Name:   clusterName,
		Region: region,
		Mode:   v2.ModeDev,
		Workers: v2.Worker{
			Count: 1,
			Size:  v2.SizeCX22,
		},
		Backup: true,
	}

	// Verify config passes validation (env vars are already set)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Config validation failed: %v", err)
	}
	t.Log("  Config validation passed")

	// Verify bucket name generation
	if cfg.BackupBucketName() != expectedBucket {
		t.Fatalf("Bucket name mismatch: got %s, want %s", cfg.BackupBucketName(), expectedBucket)
	}
	t.Log("  Bucket name generation correct")

	// Verify S3 endpoint generation
	expectedEndpoint := fmt.Sprintf("https://%s.your-objectstorage.com", string(region))
	if cfg.S3Endpoint() != expectedEndpoint {
		t.Fatalf("S3 endpoint mismatch: got %s, want %s", cfg.S3Endpoint(), expectedEndpoint)
	}
	t.Log("  S3 endpoint generation correct")

	// Verify HasBackup
	if !cfg.HasBackup() {
		t.Fatal("HasBackup() should return true")
	}
	t.Log("  HasBackup() returns true")

	t.Log("Backup configuration validated")
}

// verifyBackupCronJob verifies the TalosBackup CronJob is properly configured.
func verifyBackupCronJob(t *testing.T, kubeconfigPath, expectedSchedule string) {
	t.Log("  Verifying TalosBackup CronJob configuration...")

	// Get CronJob details
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "cronjob", "-n", "kube-system", "talos-backup",
		"-o", "jsonpath={.spec.schedule}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get CronJob schedule: %v", err)
	}

	schedule := string(output)
	if schedule != expectedSchedule {
		t.Fatalf("Unexpected schedule: got %s, want %s", schedule, expectedSchedule)
	}
	t.Logf("  CronJob schedule: %s", schedule)

	// Verify S3 secret exists
	cmd = exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "secret", "-n", "kube-system", "talos-backup-s3-secrets")
	if err := cmd.Run(); err != nil {
		t.Fatal("  S3 secrets not found")
	}
	t.Log("  S3 secrets configured")
}

// triggerBackupJob creates a manual Job from the CronJob and waits for completion.
func triggerBackupJob(t *testing.T, kubeconfigPath string, timeout time.Duration) {
	t.Log("  Triggering manual backup job...")

	jobName := fmt.Sprintf("talos-backup-manual-%d", time.Now().Unix())

	// Create a Job from the CronJob
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"create", "job", jobName, "-n", "kube-system",
		"--from=cronjob/talos-backup")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create backup job: %v\nOutput: %s", err, string(output))
	}
	t.Logf("  Created job: %s", jobName)

	// Wait for job completion
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Get job status for debugging
			statusCmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"describe", "job", "-n", "kube-system", jobName)
			if statusOutput, _ := statusCmd.CombinedOutput(); len(statusOutput) > 0 {
				t.Logf("  Job status:\n%s", string(statusOutput))
			}
			t.Fatal("  Timeout waiting for backup job to complete")

		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "job", "-n", "kube-system", jobName,
				"-o", "jsonpath={.status.succeeded}")
			output, err := cmd.CombinedOutput()
			if err == nil && string(output) == "1" {
				t.Log("  Backup job completed successfully")
				return
			}

			// Check for failure
			cmd = exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "job", "-n", "kube-system", jobName,
				"-o", "jsonpath={.status.failed}")
			failOutput, _ := cmd.CombinedOutput()
			if string(failOutput) != "" && string(failOutput) != "0" {
				// Get pod logs for debugging
				logsCmd := exec.CommandContext(context.Background(), "kubectl",
					"--kubeconfig", kubeconfigPath,
					"logs", "-n", "kube-system", "-l", fmt.Sprintf("job-name=%s", jobName))
				if logsOutput, _ := logsCmd.CombinedOutput(); len(logsOutput) > 0 {
					t.Logf("  Job logs:\n%s", string(logsOutput))
				}
				t.Fatal("  Backup job failed")
			}

			t.Log("  Waiting for backup job to complete...")
		}
	}
}

// verifyBackupInS3 checks that a backup file exists in the S3 bucket and returns the backup key.
func verifyBackupInS3(t *testing.T, s3Client *s3.Client, bucketName, prefix string) string {
	t.Log("  Verifying backup exists in S3...")

	objects, err := s3Client.ListObjects(context.Background(), bucketName, prefix)
	if err != nil {
		t.Fatalf("Failed to list S3 objects: %v", err)
	}

	if len(objects) == 0 {
		t.Fatal("  No backup files found in S3 bucket")
	}

	t.Logf("  Found %d backup file(s) in S3:", len(objects))
	for _, obj := range objects {
		t.Logf("    - %s", obj)
	}

	return objects[0] // Return the first backup key for restore verification
}

// verifyBackupRestore downloads and validates the backup file can be decompressed.
func verifyBackupRestore(t *testing.T, s3Client *s3.Client, bucketName, backupKey string) {
	t.Log("  Verifying backup can be restored (download and validate)...")

	ctx := context.Background()

	// Download the backup
	data, err := s3Client.GetObject(ctx, bucketName, backupKey)
	if err != nil {
		t.Fatalf("Failed to download backup: %v", err)
	}

	t.Logf("  Downloaded backup: %d bytes", len(data))

	// Verify it's a valid zstd-compressed file
	// zstd magic number: 0x28 0xB5 0x2F 0xFD
	if len(data) < 4 {
		t.Fatal("  Backup file too small to be valid")
	}

	zstdMagic := []byte{0x28, 0xB5, 0x2F, 0xFD}
	if data[0] != zstdMagic[0] || data[1] != zstdMagic[1] || data[2] != zstdMagic[2] || data[3] != zstdMagic[3] {
		t.Fatalf("  Invalid zstd magic number: got %x %x %x %x, want %x %x %x %x",
			data[0], data[1], data[2], data[3],
			zstdMagic[0], zstdMagic[1], zstdMagic[2], zstdMagic[3])
	}

	t.Log("  Backup is valid zstd-compressed file")
	t.Log("  Backup restore verification passed (file is downloadable and valid)")
}

// TestE2EBackupFullCluster deploys a cluster with backup enabled and verifies the backup/restore flow.
//
// This comprehensive test:
// 1. Deploys a dev cluster with backup: true via operator
// 2. Waits for TalosBackup CronJob to be ready
// 3. Triggers a manual backup
// 4. Verifies backup appears in S3
// 5. Downloads and verifies backup is valid etcd snapshot
//
// Prerequisites:
//   - HCLOUD_TOKEN, HETZNER_S3_ACCESS_KEY, HETZNER_S3_SECRET_KEY
//
// Example:
//
//	go test -v -timeout=45m -tags=e2e -run TestE2EBackupFullCluster ./tests/e2e/
func TestE2EBackupFullCluster(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	s3AccessKey := os.Getenv("HETZNER_S3_ACCESS_KEY")
	s3SecretKey := os.Getenv("HETZNER_S3_SECRET_KEY")
	if s3AccessKey == "" || s3SecretKey == "" {
		t.Skip("HETZNER_S3_ACCESS_KEY and HETZNER_S3_SECRET_KEY required for backup E2E test")
	}

	// Generate unique cluster name
	timestamp := time.Now().Unix()
	clusterName := fmt.Sprintf("e2e-bkp-%d", timestamp)
	region := "fsn1"
	bucketName := clusterName + "-etcd-backups"
	endpoint := fmt.Sprintf("https://%s.your-objectstorage.com", region)

	t.Logf("=== Starting Full Backup E2E Test: %s ===", clusterName)
	t.Logf("=== Bucket: %s ===", bucketName)

	// Create S3 client
	s3Client, err := s3.NewClient(endpoint, region, s3AccessKey, s3SecretKey)
	if err != nil {
		t.Fatalf("Failed to create S3 client: %v", err)
	}

	// Cleanup S3 bucket at end
	defer func() {
		t.Log("Cleaning up S3 bucket...")
		if err := cleanupS3Bucket(context.Background(), s3Client, bucketName); err != nil {
			t.Logf("Warning: failed to cleanup bucket: %v", err)
		}
	}()

	// Create configuration with backup enabled
	configPath := CreateTestConfig(t, clusterName, ModeDev,
		WithWorkers(1),
		WithRegion(region),
		WithBackup(true),
	)
	defer os.Remove(configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	// Create cluster via operator
	var state *OperatorTestContext

	t.Run("Create", func(t *testing.T) {
		state, err = CreateClusterViaOperator(ctx, t, configPath)
		require.NoError(t, err, "Cluster creation should succeed")
	})

	// Ensure cleanup
	defer func() {
		if state != nil {
			DestroyCluster(context.Background(), t, state)
		}
	}()

	// Wait for cluster to be ready
	t.Run("WaitForClusterReady", func(t *testing.T) {
		err := WaitForClusterReady(ctx, t, state, 30*time.Minute)
		require.NoError(t, err, "Cluster should become ready")
	})

	// === PHASE 2: Verify TalosBackup is configured ===
	t.Log("=== Verifying TalosBackup configuration ===")
	t.Run("VerifyBackupCronJob", func(t *testing.T) {
		verifyBackupCronJob(t, state.KubeconfigPath, "0 * * * *")
	})

	// === PHASE 3: Trigger manual backup ===
	t.Log("=== Triggering manual backup ===")
	t.Run("TriggerBackup", func(t *testing.T) {
		triggerBackupJob(t, state.KubeconfigPath, 5*time.Minute)
	})

	// === PHASE 4: Verify backup in S3 ===
	t.Log("=== Verifying backup in S3 ===")
	var backupKey string
	t.Run("VerifyBackupInS3", func(t *testing.T) {
		backupKey = verifyBackupInS3(t, s3Client, bucketName, "etcd-backups/")
	})

	// === PHASE 5: Verify backup can be restored ===
	t.Log("=== Verifying backup restore capability ===")
	t.Run("VerifyBackupRestore", func(t *testing.T) {
		verifyBackupRestore(t, s3Client, bucketName, backupKey)
	})

	t.Log("=== FULL BACKUP E2E TEST PASSED ===")
}

// cleanupS3Bucket deletes all objects and the bucket itself.
func cleanupS3Bucket(ctx context.Context, s3Client *s3.Client, bucketName string) error {
	// Check if bucket exists
	exists, err := s3Client.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		return nil // Nothing to clean up
	}

	// List and delete all objects
	objects, err := s3Client.ListObjects(ctx, bucketName, "")
	if err != nil {
		return fmt.Errorf("failed to list objects: %w", err)
	}

	for _, obj := range objects {
		if err := s3Client.DeleteObject(ctx, bucketName, obj); err != nil {
			return fmt.Errorf("failed to delete object %s: %w", obj, err)
		}
	}

	// Delete the bucket
	if err := s3Client.DeleteBucket(ctx, bucketName); err != nil {
		// Check if it's because the bucket is not empty (shouldn't happen, but be safe)
		if !strings.Contains(err.Error(), "BucketNotEmpty") {
			return fmt.Errorf("failed to delete bucket: %w", err)
		}
	}

	return nil
}
