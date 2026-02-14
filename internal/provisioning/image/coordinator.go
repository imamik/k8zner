package image

import (
	"context"
	"fmt"
	"strings"

	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/async"
)

const phase = "image"

// EnsureAllImages pre-builds all required Talos images in parallel.
// This is called early in reconciliation to avoid sequential image building during server creation.
func (p *Provisioner) EnsureAllImages(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Pre-building all required Talos images...", phase)

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
		ctx.Logger.Printf("[%s] No Talos images needed (all pools use custom images)", phase)
		return nil
	}

	// Determine unique architectures needed
	architectures := make(map[string]bool)
	for serverType := range serverTypes {
		arch := hcloud.DetectArchitecture(serverType)
		architectures[string(arch)] = true
	}

	ctx.Logger.Printf("[%s] Building images for architectures: %v", phase, getKeys(architectures))

	// Get versions from config
	talosVersion := ctx.Config.Talos.Version
	k8sVersion := ctx.Config.Kubernetes.Version
	if talosVersion == "" {
		talosVersion = "v1.8.3"
	}
	if k8sVersion == "" {
		k8sVersion = "v1.31.0"
	}

	// Build images in parallel using async.RunParallel
	archList := getKeys(architectures)
	tasks := make([]async.Task, len(archList))

	for i, arch := range archList {
		arch := arch // capture loop variable
		tasks[i] = async.Task{
			Name: fmt.Sprintf("image-%s", arch),
			Func: func(_ context.Context) error {
				// Get architecture-specific server type and location from config
				serverType, location := p.getImageBuilderConfig(ctx, arch)
				return p.ensureImageForArch(ctx, arch, talosVersion, k8sVersion, serverType, location)
			},
		}
	}

	if err := async.RunParallel(ctx, tasks, true); err != nil {
		return err
	}

	ctx.Logger.Printf("[%s] All required Talos images are ready", phase)
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

// getImageBuilderConfig returns the server type and location for building images of the given architecture.
// Uses config values if set, otherwise returns defaults (architecture-appropriate server type, nbg1 location).
func (p *Provisioner) getImageBuilderConfig(ctx *provisioning.Context, arch string) (serverType, location string) {
	// Get location from first control plane node as default
	location = "nbg1"
	if len(ctx.Config.ControlPlane.NodePools) > 0 && ctx.Config.ControlPlane.NodePools[0].Location != "" {
		location = ctx.Config.ControlPlane.NodePools[0].Location
	}

	// Get architecture-specific config (serverType defaults to empty, meaning auto-detect)
	switch arch {
	case "amd64":
		if ctx.Config.Talos.ImageBuilder.AMD64.ServerType != "" {
			serverType = ctx.Config.Talos.ImageBuilder.AMD64.ServerType
		}
		if ctx.Config.Talos.ImageBuilder.AMD64.ServerLocation != "" {
			location = ctx.Config.Talos.ImageBuilder.AMD64.ServerLocation
		}
	case "arm64":
		if ctx.Config.Talos.ImageBuilder.ARM64.ServerType != "" {
			serverType = ctx.Config.Talos.ImageBuilder.ARM64.ServerType
		}
		if ctx.Config.Talos.ImageBuilder.ARM64.ServerLocation != "" {
			location = ctx.Config.Talos.ImageBuilder.ARM64.ServerLocation
		}
	}

	return serverType, location
}

// ensureImageForArch ensures a Talos image exists for the given architecture.
func (p *Provisioner) ensureImageForArch(ctx *provisioning.Context, arch, talosVersion, k8sVersion, serverType, location string) error {
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
		ctx.Logger.Printf("[%s] Found existing Talos snapshot for %s: %s (ID: %d)", phase, arch, snapshot.Description, snapshot.ID)
		return nil
	}

	// Build image, trying smallest/default server type and cross-region fallback.
	builder := p.createImageBuilder(ctx)
	if builder == nil {
		return fmt.Errorf("image builder not available")
	}

	snapshotID, buildLocation, err := p.buildImageWithFallback(ctx, builder, arch, talosVersion, k8sVersion, serverType, location, labels)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	ctx.Logger.Printf("[%s] Successfully built Talos snapshot for %s in %s: ID %s", phase, arch, buildLocation, snapshotID)
	return nil
}

func (p *Provisioner) buildImageWithFallback(ctx *provisioning.Context, builder *Builder, arch, talosVersion, k8sVersion, serverType, preferredLocation string, labels map[string]string) (snapshotID, location string, err error) {
	// Always prefer the smallest default build machine when not explicitly configured.
	buildServerType := serverType
	if buildServerType == "" {
		buildServerType = hcloud.GetDefaultServerType(hcloud.Architecture(arch))
	}

	for _, candidateLocation := range buildLocations(preferredLocation) {
		ctx.Logger.Printf("[%s] Building Talos image for %s/%s/%s with %s in %s...", phase, talosVersion, k8sVersion, arch, buildServerType, candidateLocation)
		snapshotID, err = builder.Build(ctx, talosVersion, k8sVersion, arch, buildServerType, candidateLocation, labels)
		if err == nil {
			return snapshotID, candidateLocation, nil
		}
		if !isRegionCapacityOrAvailabilityError(err) {
			return "", "", err
		}
		ctx.Logger.Printf("[%s] Build attempt in %s failed (%v), trying next region...", phase, candidateLocation, err)
	}

	return "", "", err
}

func buildLocations(preferred string) []string {
	base := []string{"nbg1", "fsn1", "hel1"}
	if preferred == "" {
		return base
	}

	locations := []string{preferred}
	for _, loc := range base {
		if loc != preferred {
			locations = append(locations, loc)
		}
	}
	return locations
}

func isRegionCapacityOrAvailabilityError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "resource unavailable") ||
		strings.Contains(msg, "out of stock") ||
		strings.Contains(msg, "no server available") ||
		strings.Contains(msg, "capacity")
}

// createImageBuilder creates an image builder instance.
func (p *Provisioner) createImageBuilder(ctx *provisioning.Context) *Builder {
	// Pass nil for communicator factory - the builder will use its internal
	// SSH key generation and create its own SSH client with those keys
	return NewBuilder(ctx.Infra)
}
