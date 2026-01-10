package hcloud

import (
	"errors"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func TestIsResourceLocked(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("something went wrong"),
			expected: false,
		},
		{
			name:     "hcloud locked error",
			err:      hcloud.Error{Code: hcloud.ErrorCodeLocked, Message: "resource is locked"},
			expected: true,
		},
		{
			name:     "hcloud conflict error",
			err:      hcloud.Error{Code: hcloud.ErrorCodeConflict, Message: "conflict occurred"},
			expected: true,
		},
		{
			name:     "hcloud resource locked error",
			err:      hcloud.Error{Code: hcloud.ErrorCodeResourceLocked, Message: "resource locked"},
			expected: true,
		},
		{
			name:     "hcloud resource unavailable error",
			err:      hcloud.Error{Code: hcloud.ErrorCodeResourceUnavailable, Message: "unavailable"},
			expected: true,
		},
		{
			name:     "hcloud not found error (not locked)",
			err:      hcloud.Error{Code: hcloud.ErrorCodeNotFound, Message: "not found"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isResourceLocked(tt.err)
			if result != tt.expected {
				t.Errorf("isResourceLocked(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsInvalidParameter(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("something went wrong"),
			expected: false,
		},
		{
			name:     "hcloud not found error",
			err:      hcloud.Error{Code: hcloud.ErrorCodeNotFound, Message: "not found"},
			expected: true,
		},
		{
			name:     "hcloud invalid input error",
			err:      hcloud.Error{Code: hcloud.ErrorCodeInvalidInput, Message: "invalid input"},
			expected: true,
		},
		{
			name:     "hcloud invalid server type error",
			err:      hcloud.Error{Code: hcloud.ErrorCodeInvalidServerType, Message: "invalid server type"},
			expected: true,
		},
		{
			name:     "hcloud locked error (not invalid)",
			err:      hcloud.Error{Code: hcloud.ErrorCodeLocked, Message: "locked"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInvalidParameter(tt.err)
			if result != tt.expected {
				t.Errorf("isInvalidParameter(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "hcloud not found",
			err:      hcloud.Error{Code: hcloud.ErrorCodeNotFound, Message: "not found"},
			expected: true,
		},
		{
			name:     "hcloud other error",
			err:      hcloud.Error{Code: hcloud.ErrorCodeLocked, Message: "locked"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotFound(tt.err)
			if result != tt.expected {
				t.Errorf("IsNotFound(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsConflict(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "hcloud conflict",
			err:      hcloud.Error{Code: hcloud.ErrorCodeConflict, Message: "conflict"},
			expected: true,
		},
		{
			name:     "hcloud other error",
			err:      hcloud.Error{Code: hcloud.ErrorCodeNotFound, Message: "not found"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsConflict(tt.err)
			if result != tt.expected {
				t.Errorf("IsConflict(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "hcloud rate limited",
			err:      hcloud.Error{Code: hcloud.ErrorCodeRateLimitExceeded, Message: "rate limited"},
			expected: true,
		},
		{
			name:     "hcloud other error",
			err:      hcloud.Error{Code: hcloud.ErrorCodeNotFound, Message: "not found"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRateLimited(tt.err)
			if result != tt.expected {
				t.Errorf("IsRateLimited(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
