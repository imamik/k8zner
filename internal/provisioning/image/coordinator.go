package image

import (
	"context"
	"fmt"
	"log"

	"hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/async"
)

// EnsureAllImages pre-builds all required Talos images in parallel.
// This is called early in reconciliation to avoid sequential image building during server creation.
func (p *Provisioner) EnsureAllImages(ctx *provisioning.Context) error {
	log.Println("Pre-building all required Talos images...")

	// Collect all unique server types from control plane and worker pools
	serverTypes := make(map[string]bool)

	// Control plane server types
	for _, pool := range ctx.Config.ControlPlane.NodePools {
		if pool.Image == "" || pool.Image == "talos" {
			serverTypes[pool.ServerType] = true
		}
	}

	// Worker server types
	for _, pool := range ctx.Config.Workers {
		if pool.Image == "" || pool.Image == "talos" {
			serverTypes[pool.ServerType] = true
		}
	}

	if len(serverTypes) == 0 {
		log.Println("No Talos images needed (all pools use custom images)")
		return nil
	}

	// Determine unique architectures needed
	architectures := make(map[string]bool)
	for serverType := range serverTypes {
		arch := hcloud.DetectArchitecture(serverType)
		architectures[string(arch)] = true
	}

	log.Printf("Building images for architectures: %v", getKeys(architectures))

	// Get versions from config
	talosVersion := ctx.Config.Talos.Version
	k8sVersion := ctx.Config.Kubernetes.Version
	if talosVersion == "" {
		talosVersion = "v1.8.3"
	}
	if k8sVersion == "" {
		k8sVersion = "v1.31.0"
	}

	// Get location from first control plane node, or default to nbg1
	location := "nbg1"
	if len(ctx.Config.ControlPlane.NodePools) > 0 && ctx.Config.ControlPlane.NodePools[0].Location != "" {
		location = ctx.Config.ControlPlane.NodePools[0].Location
	}

	// Build images in parallel using async.RunParallel
	archList := getKeys(architectures)
	tasks := make([]async.Task, len(archList))

	for i, arch := range archList {
		arch := arch // capture loop variable
		tasks[i] = async.Task{
			Name: fmt.Sprintf("image-%s", arch),
			Func: func(_ context.Context) error {
				return p.ensureImageForArch(ctx, arch, talosVersion, k8sVersion, location)
			},
		}
	}

	if err := async.RunParallel(ctx, tasks); err != nil {
		return err
	}

	log.Println("All required Talos images are ready")
	return nil
}

// getKeys returns the keys of a map as a slice (helper function).
func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ensureImageForArch ensures a Talos image exists for the given architecture.
func (p *Provisioner) ensureImageForArch(ctx *provisioning.Context, arch, talosVersion, k8sVersion, location string) error {
	labels := map[string]string{
		"os":            "talos",
		"talos-version": talosVersion,
		"k8s-version":   k8sVersion,
		"arch":          arch,
	}

	// Check if snapshot already exists
	snapshot, err := ctx.Infra.GetSnapshotByLabels(ctx, labels)
	if err != nil {
		return fmt.Errorf("failed to check for existing snapshot: %w", err)
	}

	if snapshot != nil {
		log.Printf("Found existing Talos snapshot for %s: %s (ID: %d)", arch, snapshot.Description, snapshot.ID)
		return nil
	}

	// Build image
	log.Printf("Building Talos image for %s/%s/%s in location %s...", talosVersion, k8sVersion, arch, location)
	builder := p.createImageBuilder(ctx)
	if builder == nil {
		return fmt.Errorf("image builder not available")
	}

	snapshotID, err := builder.Build(ctx, talosVersion, k8sVersion, arch, location, labels)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	log.Printf("Successfully built Talos snapshot for %s: ID %s", arch, snapshotID)
	return nil
}

// createImageBuilder creates an image builder instance.
func (p *Provisioner) createImageBuilder(ctx *provisioning.Context) *Builder {
	// Pass nil for communicator factory - the builder will use its internal
	// SSH key generation and create its own SSH client with those keys
	return NewBuilder(ctx.Infra)
}
