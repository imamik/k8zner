package hcloud

import (
	"errors"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// isResourceLocked checks if an error indicates a resource is locked.
// Locked resources typically occur during snapshot creation or other
// long-running operations. These errors are retryable.
func isResourceLocked(err error) bool {
	return isHCloudErrorCode(err,
		hcloud.ErrorCodeLocked,           // Item is locked (action running)
		hcloud.ErrorCodeConflict,         // Resource changed during request
		hcloud.ErrorCodeResourceLocked,   // Resource locked (contact support)
		hcloud.ErrorCodeResourceUnavailable,
	)
}

// isInvalidParameter checks if an error indicates invalid parameters.
// These errors are fatal and should not be retried.
func isInvalidParameter(err error) bool {
	return isHCloudErrorCode(err,
		hcloud.ErrorCodeNotFound,
		hcloud.ErrorCodeInvalidInput,
		hcloud.ErrorCodeInvalidServerType,
	)
}

// isHCloudErrorCode checks if the error is an hcloud API error with one of the given codes.
func isHCloudErrorCode(err error, codes ...hcloud.ErrorCode) bool {
	if err == nil {
		return false
	}

	var hcloudErr hcloud.Error
	if errors.As(err, &hcloudErr) {
		for _, code := range codes {
			if hcloudErr.Code == code {
				return true
			}
		}
	}
	return false
}

// IsNotFound checks if an error indicates a resource was not found.
func IsNotFound(err error) bool {
	return isHCloudErrorCode(err, hcloud.ErrorCodeNotFound)
}

// IsConflict checks if an error indicates a conflict occurred.
func IsConflict(err error) bool {
	return isHCloudErrorCode(err, hcloud.ErrorCodeConflict)
}

// IsRateLimited checks if an error indicates rate limiting.
func IsRateLimited(err error) bool {
	return isHCloudErrorCode(err, hcloud.ErrorCodeRateLimitExceeded)
}
