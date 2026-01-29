package s3

import (
	"testing"
)

func TestContains(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"empty strings", "", "", true},
		{"empty substr", "hello", "", true},
		{"substr not found", "hello", "world", false},
		{"substr found", "hello world", "world", true},
		{"substr at start", "hello world", "hello", true},
		{"substr at end", "hello world", "world", true},
		{"exact match", "hello", "hello", true},
		{"substr longer than s", "hi", "hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestIsBucketAlreadyOwnedByYou(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBucketAlreadyOwnedByYou(tt.err)
			if got != tt.want {
				t.Errorf("isBucketAlreadyOwnedByYou() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNotFoundError(tt.err)
			if got != tt.want {
				t.Errorf("isNotFoundError() = %v, want %v", got, tt.want)
			}
		})
	}
}
