// Package s3 provides a client for Hetzner Object Storage (S3-compatible).
package s3

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

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
