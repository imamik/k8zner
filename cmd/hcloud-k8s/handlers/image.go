package handlers

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/image"
)

func Build(ctx context.Context, imageName, talosVersion, arch, location string) error {
	token := os.Getenv("HCLOUD_TOKEN")
	client := hcloud.NewRealClient(token)
	builder := image.NewBuilder(client, nil) // use default SSH communicator

	log.Printf("Building image %s (Talos %s, Arch %s) in location %s...", imageName, talosVersion, arch, location)

	snapshotID, err := builder.Build(ctx, imageName, talosVersion, arch, location, nil)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Printf("Image built successfully! Snapshot ID: %s\n", snapshotID)
	return nil
}
