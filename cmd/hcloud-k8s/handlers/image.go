package handlers

import (
	"context"
	"fmt"
	"log"
	"os"

	"hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning"
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
	token := os.Getenv("HCLOUD_TOKEN")
	client := hcloud.NewRealClient(token)
	builder := provisioning.NewBuilder(client, nil) // use default SSH communicator

	log.Printf("Building image %s (Talos %s, Arch %s) in location %s...", imageName, talosVersion, arch, location)

	snapshotID, err := builder.Build(ctx, imageName, talosVersion, arch, location, nil)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Printf("Image built successfully! Snapshot ID: %s\n", snapshotID)
	return nil
}
