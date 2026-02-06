// Package s3 provides a client for Hetzner Object Storage (S3-compatible).
package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

const (
	// MetadataFileName is the name of the metadata file used to identify k8zner-managed buckets.
	MetadataFileName = "k8zner_metadata.json"
)

// BucketMetadata contains metadata about a k8zner-managed S3 bucket.
// This file is written to each bucket to verify ownership during cleanup.
type BucketMetadata struct {
	ClusterName string `json:"clusterName"`
	ManagedBy   string `json:"managedBy"`
	CreatedAt   string `json:"createdAt"`
}

// Client wraps the S3 client for Hetzner Object Storage.
type Client struct {
	s3     *s3.Client
	region string
}

// NewClient creates a new S3 client for Hetzner Object Storage.
func NewClient(endpoint, region, accessKey, secretKey string) (*Client, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = false // Hetzner uses virtual-hosted style
	})

	return &Client{s3: client, region: region}, nil
}

// CreateBucket creates a new S3 bucket.
// Returns nil if the bucket already exists and is owned by us.
func (c *Client) CreateBucket(ctx context.Context, bucketName string) error {
	_, err := c.s3.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		// Check if bucket already exists (that's okay)
		if isBucketAlreadyOwnedByYou(err) {
			return nil
		}
		return fmt.Errorf("failed to create bucket %s: %w", bucketName, err)
	}
	return nil
}

// CreateBucketWithMetadata creates a new S3 bucket and writes k8zner metadata file.
// This allows safe identification and cleanup of k8zner-managed buckets.
func (c *Client) CreateBucketWithMetadata(ctx context.Context, bucketName, clusterName string) error {
	// Create the bucket
	if err := c.CreateBucket(ctx, bucketName); err != nil {
		return err
	}

	// Write metadata file
	return c.WriteMetadata(ctx, bucketName, clusterName)
}

// WriteMetadata writes the k8zner metadata file to a bucket.
// This can be used to add metadata to existing buckets.
func (c *Client) WriteMetadata(ctx context.Context, bucketName, clusterName string) error {
	metadata := BucketMetadata{
		ClusterName: clusterName,
		ManagedBy:   "k8zner",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	return c.PutObject(ctx, bucketName, MetadataFileName, data)
}

// GetMetadata retrieves the k8zner metadata from a bucket.
// Returns nil if the metadata file doesn't exist.
func (c *Client) GetMetadata(ctx context.Context, bucketName string) (*BucketMetadata, error) {
	data, err := c.GetObject(ctx, bucketName, MetadataFileName)
	if err != nil {
		// Check if it's a not found error
		if isNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}

	var metadata BucketMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &metadata, nil
}

// BucketExists checks if a bucket exists and is accessible.
func (c *Client) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	_, err := c.s3.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check bucket %s: %w", bucketName, err)
	}
	return true, nil
}

// isBucketAlreadyOwnedByYou checks if the error indicates the bucket exists and is owned by us.
func isBucketAlreadyOwnedByYou(err error) bool {
	if err == nil {
		return false
	}

	// Check for typed S3 errors first
	var baoby *types.BucketAlreadyOwnedByYou
	if errors.As(err, &baoby) {
		return true
	}

	var bae *types.BucketAlreadyExists
	if errors.As(err, &bae) {
		return true
	}

	// Fall back to API error code checking for S3-compatible services
	// that may not return the exact SDK error types
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "BucketAlreadyOwnedByYou" || code == "BucketAlreadyExists"
	}

	return false
}

// isNotFoundError checks if the error is a not found error.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Check for typed S3 errors first
	var nsb *types.NoSuchBucket
	if errors.As(err, &nsb) {
		return true
	}

	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}

	// Fall back to API error code checking for S3-compatible services
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "NotFound" || code == "NoSuchBucket" || code == "404"
	}

	return false
}

// ListObjects lists objects in a bucket with an optional prefix filter.
func (c *Client) ListObjects(ctx context.Context, bucketName, prefix string) ([]string, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	result, err := c.s3.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects in bucket %s: %w", bucketName, err)
	}

	var keys []string
	for _, obj := range result.Contents {
		if obj.Key != nil {
			keys = append(keys, *obj.Key)
		}
	}
	return keys, nil
}

// PutObject uploads an object to a bucket.
func (c *Client) PutObject(ctx context.Context, bucketName, key string, data []byte) error {
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucketName),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	})
	if err != nil {
		return fmt.Errorf("failed to put object %s in bucket %s: %w", key, bucketName, err)
	}
	return nil
}

// GetObject downloads an object from a bucket.
func (c *Client) GetObject(ctx context.Context, bucketName, key string) ([]byte, error) {
	result, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object %s from bucket %s: %w", key, bucketName, err)
	}
	defer func() {
		_ = result.Body.Close()
	}()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(result.Body); err != nil {
		return nil, fmt.Errorf("failed to read object body: %w", err)
	}

	return buf.Bytes(), nil
}

// DeleteObject deletes an object from a bucket.
func (c *Client) DeleteObject(ctx context.Context, bucketName, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object %s from bucket %s: %w", key, bucketName, err)
	}
	return nil
}

// DeleteBucket deletes a bucket. The bucket must be empty.
func (c *Client) DeleteBucket(ctx context.Context, bucketName string) error {
	_, err := c.s3.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return fmt.Errorf("failed to delete bucket %s: %w", bucketName, err)
	}
	return nil
}
