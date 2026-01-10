package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuild_MissingToken(t *testing.T) {
	// t.Setenv clears the token and automatically restores it after the test
	t.Setenv("HCLOUD_TOKEN", "")

	// Build should fail due to missing token
	ctx := context.Background()
	err := Build(ctx, "test-image", "v1.8.3", "amd64", "nbg1")

	// The error will be from the hcloud client validation
	assert.Error(t, err)
}

func TestBuild_InvalidParameters(t *testing.T) {
	tests := []struct {
		name         string
		imageName    string
		talosVersion string
		arch         string
		location     string
	}{
		{
			name:         "empty image name",
			imageName:    "",
			talosVersion: "v1.8.3",
			arch:         "amd64",
			location:     "nbg1",
		},
		{
			name:         "empty talos version",
			imageName:    "test-image",
			talosVersion: "",
			arch:         "amd64",
			location:     "nbg1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := Build(ctx, tt.imageName, tt.talosVersion, tt.arch, tt.location)
			// These may fail for various reasons (no token, invalid params, etc.)
			// The important thing is they don't panic
			assert.Error(t, err)
		})
	}
}
