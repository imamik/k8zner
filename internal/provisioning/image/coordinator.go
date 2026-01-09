package image

import (
	"context"
	"fmt"
	"log"

	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
)

// EnsureAllImages pre-builds all required Talos images in parallel.
// This is called early in reconciliation to avoid sequential image building during server creation.
func (p *Provisioner) EnsureAllImages(ctx context.Context) error {
	log.Println("Pre-building all required Talos images...")

	// Collect all unique server types from control plane and worker pools
	serverTypes := make(map[string]bool)

	// Control plane server types
	for _, pool := range p.config.ControlPlane.NodePools {
		if pool.Image == "" || pool.Image == "talos" {
			serverTypes[pool.ServerType] = true
		}
	}

	// Worker server types
	for _, pool := range p.config.Workers {
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
		arch := hcloud_internal.DetectArchitecture(serverType)
		architectures[string(arch)] = true
	}

	log.Printf("Building images for architectures: %v", getKeys(architectures))

	// Get versions from config
	talosVersion := p.config.Talos.Version
	k8sVersion := p.config.Kubernetes.Version
	if talosVersion == "" {
		talosVersion = "v1.8.3"
	}
	if k8sVersion == "" {
		k8sVersion = "v1.31.0"
	}

	// Get location from first control plane node, or default to nbg1
	location := "nbg1"
	if len(p.config.ControlPlane.NodePools) > 0 && p.config.ControlPlane.NodePools[0].Location != "" {
		location = p.config.ControlPlane.NodePools[0].Location
	}

	// Build images in parallel
	type buildResult struct {
		arch string
		err  error
	}

	resultChan := make(chan buildResult, len(architectures))

	for arch := range architectures {
		arch := arch // capture loop variable
		go func() {
			labels := map[string]string{
				"os":            "talos",
				"talos-version": talosVersion,
				"k8s-version":   k8sVersion,
				"arch":          arch,
			}

			// Check if snapshot already exists
			snapshot, err := p.snapshotManager.GetSnapshotByLabels(ctx, labels)
			if err != nil {
				resultChan <- buildResult{arch: arch, err: fmt.Errorf("failed to check for existing snapshot: %w", err)}
				return
			}

			if snapshot != nil {
				log.Printf("Found existing Talos snapshot for %s: %s (ID: %d)", arch, snapshot.Description, snapshot.ID)
				resultChan <- buildResult{arch: arch, err: nil}
				return
			}

			// Build image
			log.Printf("Building Talos image for %s/%s/%s in location %s...", talosVersion, k8sVersion, arch, location)
			builder := p.createImageBuilder()
			if builder == nil {
				resultChan <- buildResult{arch: arch, err: fmt.Errorf("image builder not available")}
				return
			}

			snapshotID, err := builder.Build(ctx, talosVersion, k8sVersion, arch, location, labels)
			if err != nil {
				resultChan <- buildResult{arch: arch, err: fmt.Errorf("failed to build image: %w", err)}
				return
			}

			log.Printf("Successfully built Talos snapshot for %s: ID %s", arch, snapshotID)
			resultChan <- buildResult{arch: arch, err: nil}
		}()
	}

	// Wait for all builds to complete
	var errors []error
	for i := 0; i < len(architectures); i++ {
		result := <-resultChan
		if result.err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", result.arch, result.err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to build some images: %v", errors)
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

// createImageBuilder creates an image builder instance.
func (p *Provisioner) createImageBuilder() *Builder {
	// Pass nil for communicator factory - the builder will use its internal
	// SSH key generation and create its own SSH client with those keys
	return NewBuilder(p.infra)
}
