package hcloud

import (
	"strings"
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

