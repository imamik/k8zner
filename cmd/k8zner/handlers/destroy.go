package handlers

import (
	"context"
	"fmt"
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

	// Find buckets matching cluster naming convention
	prefix := clusterName + "-"
	for _, bucket := range result.Buckets {
		if bucket.Name != nil && strings.HasPrefix(*bucket.Name, prefix) {
			log.Printf("Deleting S3 bucket: %s", *bucket.Name)
			if err := deleteS3Bucket(ctx, s3Client, *bucket.Name); err != nil {
				log.Printf("Warning: failed to delete bucket %s: %v", *bucket.Name, err)
			}
		}
	}

	return nil
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
