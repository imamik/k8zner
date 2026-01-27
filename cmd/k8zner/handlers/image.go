package handlers

import (
	"context"
	"fmt"
	"log"

	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning/image"
)

// ImageBuilder interface for testing - matches image.Builder.
type ImageBuilder interface {
	Build(ctx context.Context, talosVersion, k8sVersion, architecture, serverType, location string, labels map[string]string) (string, error)
}

// Factory function variables for image - can be replaced in tests.
var (
	// newImageBuilder creates a new image builder.
	newImageBuilder = func(client hcloud.InfrastructureManager) ImageBuilder {
		return image.NewBuilder(client)
	}
)

// Build creates a custom Talos Linux snapshot on Hetzner Cloud.
//
// This function provisions a temporary build server, installs Talos Linux
// with the specified version and architecture, creates a snapshot, and
// cleans up the build resources.
//
// The resulting snapshot can be used as the base image for cluster nodes
// when provisioning with the apply command.
//
// The function expects HCLOUD_TOKEN to be set in the environment and will
// delegate validation to the Hetzner Cloud client.
func Build(ctx context.Context, imageName, talosVersion, arch, location string) error {
	client := initializeClient()
	builder := newImageBuilder(client)

	log.Printf("Building image %s (Talos %s, Arch %s) in location %s...", imageName, talosVersion, arch, location)

	// serverType is empty to use auto-detection based on architecture
	snapshotID, err := builder.Build(ctx, imageName, talosVersion, arch, "", location, nil)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Printf("Image built successfully! Snapshot ID: %s\n", snapshotID)
	return nil
}
