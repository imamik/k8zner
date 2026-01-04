package hcloud

import (
	"strings"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// isResourceLocked checks if an error indicates a resource is locked.
// Locked resources typically occur during snapshot creation or other
// long-running operations. These errors are retryable.
func isResourceLocked(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "locked") ||
		strings.Contains(errStr, "conflict") ||
		strings.Contains(errStr, "is busy")
}

// isInvalidParameter checks if an error indicates invalid parameters.
// These errors are fatal and should not be retried.
func isInvalidParameter(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "invalid") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "does not exist")
}

// isRateLimitError checks if an error is due to rate limiting.
// Rate limit errors are retryable.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	// Check for hcloud.Error with specific code
	var hcloudErr hcloud.Error
	if err, ok := err.(hcloud.Error); ok {
		hcloudErr = err
		// Rate limit is typically HTTP 429
		return hcloudErr.Code == "rate_limit_exceeded" || strings.Contains(hcloudErr.Message, "rate limit")
	}

	errStr := err.Error()
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "too many requests")
}

// isTemporaryError checks if an error is temporary and retryable.
func isTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "temporary") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "503") || // Service Unavailable
		strings.Contains(errStr, "502")    // Bad Gateway
}

// isRetryable checks if an error should trigger a retry.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	return isResourceLocked(err) ||
		isRateLimitError(err) ||
		isTemporaryError(err)
}
