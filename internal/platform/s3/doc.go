// Package s3 provides a client for Hetzner Object Storage (S3-compatible).
//
// It handles bucket creation, object upload, and lifecycle management for
// etcd backup storage. The client auto-detects the S3 endpoint from the
// cluster region.
package s3
