package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/provisioning/destroy"
)

const (
	// s3MetadataFile is the name of the metadata file used to verify bucket ownership.
	// This should match s3.MetadataFileName.
	s3MetadataFile = "k8zner_metadata.json"
)

// Provisioner interface for testing - matches provisioning.Phase.
type Provisioner interface {
	Provision(ctx *provisioning.Context) error
}

// Factory function variables for destroy - can be replaced in tests.
var (
	// newDestroyProvisioner creates a new destroy provisioner.
	newDestroyProvisioner = func() Provisioner {
		return destroy.NewProvisioner()
	}

	// newProvisioningContext creates a new provisioning context.
	newProvisioningContext = provisioning.NewContext
)

// Destroy handles the destroy command.
//
// It loads the cluster configuration and deletes all associated resources
// from Hetzner Cloud. Resources are deleted in dependency order.
// Also cleans up S3 buckets matching the cluster naming convention.
func Destroy(ctx context.Context, configPath string) error {
	// Load configuration using v2 loader
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	log.Printf("Destroying cluster: %s", cfg.ClusterName)

	// Initialize Hetzner Cloud client using environment variable
	token := os.Getenv("HCLOUD_TOKEN")
	infraClient := newInfraClient(token)

	// Create provisioning context (no Talos generator needed for destroy)
	pCtx := newProvisioningContext(ctx, cfg, infraClient, nil)

	// Create destroy provisioner
	destroyer := newDestroyProvisioner()

	// Execute destroy
	if err := destroyer.Provision(pCtx); err != nil {
		return fmt.Errorf("destroy failed: %w", err)
	}

	// Clean up S3 buckets if talos-backup was configured
	if cfg.Addons.TalosBackup.Enabled {
		log.Println("Cleaning up S3 buckets...")
		if err := cleanupS3Buckets(ctx, cfg.ClusterName, cfg.Addons.TalosBackup); err != nil {
			log.Printf("Warning: S3 cleanup failed: %v", err)
			// Don't fail the destroy for S3 cleanup issues
		}
	}

	log.Printf("Cluster %s destroyed successfully", cfg.ClusterName)
	return nil
}

// cleanupS3Buckets removes S3 buckets matching the cluster naming convention.
// Buckets are named: {cluster}-* (e.g., my-cluster-etcd-backup)
func cleanupS3Buckets(ctx context.Context, clusterName string, backupCfg config.TalosBackupConfig) error {
	endpoint := backupCfg.S3Endpoint
	accessKey := backupCfg.S3AccessKey
	secretKey := backupCfg.S3SecretKey
	region := backupCfg.S3Region

	if endpoint == "" || accessKey == "" || secretKey == "" {
		log.Println("S3 credentials not configured, skipping bucket cleanup")
		return nil
	}

	// Default region if not specified
	if region == "" {
		region = "us-east-1"
	}

	// Create S3 client with custom endpoint
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return fmt.Errorf("failed to create S3 config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	// List all buckets
	result, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return fmt.Errorf("failed to list S3 buckets: %w", err)
	}

	// Find buckets matching cluster naming convention and verify ownership
	prefix := clusterName + "-"
	for _, bucket := range result.Buckets {
		if bucket.Name == nil || !strings.HasPrefix(*bucket.Name, prefix) {
			continue
		}

		bucketName := *bucket.Name

		// Verify bucket ownership via metadata file
		owned, err := verifyBucketOwnership(ctx, s3Client, bucketName, clusterName)
		if err != nil {
			log.Printf("Warning: failed to verify ownership of bucket %s: %v", bucketName, err)
			continue
		}

		if !owned {
			log.Printf("Skipping bucket %s: ownership not verified (no valid metadata)", bucketName)
			continue
		}

		log.Printf("Deleting S3 bucket: %s", bucketName)
		if err := deleteS3Bucket(ctx, s3Client, bucketName); err != nil {
			log.Printf("Warning: failed to delete bucket %s: %v", bucketName, err)
		}
	}

	return nil
}

// s3BucketMetadata represents the metadata stored in k8zner-managed buckets.
type s3BucketMetadata struct {
	ClusterName string `json:"clusterName"`
	ManagedBy   string `json:"managedBy"`
	CreatedAt   string `json:"createdAt,omitempty"`
}

// verifyBucketOwnership checks if a bucket is owned by the specified cluster
// by reading the k8zner_metadata.json file and verifying the cluster name.
func verifyBucketOwnership(ctx context.Context, s3Client *s3.Client, bucketName, expectedCluster string) (bool, error) {
	// Try to get the metadata file
	result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(s3MetadataFile),
	})
	if err != nil {
		// If the metadata file doesn't exist, check if bucket name matches exactly
		// This provides backwards compatibility for buckets created before metadata was added
		errStr := err.Error()
		if strings.Contains(errStr, "NoSuchKey") || strings.Contains(errStr, "not found") || strings.Contains(errStr, "404") {
			// Fall back to strict prefix matching only for known k8zner bucket patterns
			knownPatterns := []string{
				expectedCluster + "-etcd-backup",
				expectedCluster + "-talos-backup",
			}
			for _, pattern := range knownPatterns {
				if bucketName == pattern {
					log.Printf("Bucket %s has no metadata file but matches known k8zner pattern, allowing deletion", bucketName)
					return true, nil
				}
			}
			return false, nil
		}
		return false, fmt.Errorf("failed to get metadata: %w", err)
	}
	defer func() { _ = result.Body.Close() }()

	// Read and parse the metadata
	data, err := io.ReadAll(result.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata s3BucketMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return false, fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Verify cluster name matches
	if metadata.ClusterName != expectedCluster {
		log.Printf("Bucket %s belongs to cluster %s, not %s", bucketName, metadata.ClusterName, expectedCluster)
		return false, nil
	}

	// Verify this is a k8zner-managed bucket
	if metadata.ManagedBy != "k8zner" {
		log.Printf("Bucket %s is not managed by k8zner (managedBy: %s)", bucketName, metadata.ManagedBy)
		return false, nil
	}

	return true, nil
}

// deleteS3Bucket empties and deletes an S3 bucket.
func deleteS3Bucket(ctx context.Context, client *s3.Client, bucketName string) error {
	// First, delete all objects in the bucket
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	}

	paginator := s3.NewListObjectsV2Paginator(client, listInput)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucketName),
				Key:    obj.Key,
			})
			if err != nil {
				log.Printf("Warning: failed to delete object %s: %v", *obj.Key, err)
			}
		}
	}

	// Now delete the bucket
	_, err := client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return fmt.Errorf("failed to delete bucket: %w", err)
	}

	return nil
}
