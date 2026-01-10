package compute

import (
	"context"
	"fmt"
	"log"

	"hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/labels"
	"hcloud-k8s/internal/util/retry"
)

// ensureServer ensures a server exists and returns its IP.
func (p *Provisioner) ensureServer(
	ctx *provisioning.Context,
	serverName string,
	serverType string,
	location string,
	image string,
	role string,
	poolName string,
	extraLabels map[string]string,
	userData string,
	pgID *int64,
	privateIP string,
) (string, error) {
	// Check if exists
	serverID, err := ctx.Infra.GetServerID(ctx, serverName)
	if err != nil {
		return "", err
	}

	if serverID != "" {
		// Server exists, get IP
		ip, err := ctx.Infra.GetServerIP(ctx, serverName)
		if err != nil {
			return "", err
		}
		return ip, nil
	}

	// Create
	log.Printf("[Compute:Server] Creating %s Server %s...", role, serverName)

	// Labels
	labels := labels.NewLabelBuilder(ctx.Config.ClusterName).
		WithRole(role).
		WithPool(poolName).
		Merge(extraLabels).
		Build()

	// Image defaulting - if empty or "talos", ensure the versioned image exists
	if image == "" || image == "talos" {
		var err error
		image, err = p.ensureImage(ctx, serverType, location)
		if err != nil {
			return "", fmt.Errorf("failed to ensure Talos image: %w", err)
		}
		log.Printf("[Compute:Server] Using Talos image: %s", image)
	}

	// Get Network ID
	if ctx.State == nil || ctx.State.Network == nil {
		return "", fmt.Errorf("network not initialized in provisioning state")
	}
	networkID := ctx.State.Network.ID

	_, err = ctx.Infra.CreateServer(
		ctx,
		serverName,
		image,
		serverType,
		location,
		ctx.Config.SSHKeys,
		labels,
		userData,
		pgID,
		networkID,
		privateIP,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create server %s: %w", serverName, err)
	}

	// Get IP after creation with retry logic and configurable timeout
	ipCtx, cancel := context.WithTimeout(ctx, ctx.Timeouts.ServerIP)
	defer cancel()

	var ip string
	err = retry.WithExponentialBackoff(ipCtx, func() error {
		var getErr error
		ip, getErr = ctx.Infra.GetServerIP(ctx, serverName)
		if getErr != nil {
			return getErr
		}
		if ip == "" {
			return fmt.Errorf("server IP not yet assigned")
		}
		return nil
	}, retry.WithMaxRetries(ctx.Timeouts.RetryMaxAttempts), retry.WithInitialDelay(ctx.Timeouts.RetryInitialDelay))

	if err != nil {
		return "", fmt.Errorf("failed to get server IP for %s: %w", serverName, err)
	}

	return ip, nil
}

// ensureImage ensures the required Talos image exists and returns its ID.
// It checks for an existing snapshot and builds it if necessary.
func (p *Provisioner) ensureImage(ctx *provisioning.Context, serverType, _ string) (string, error) {
	// Determine architecture from server type
	arch := string(hcloud.DetectArchitecture(serverType))

	// Get versions from config
	talosVersion := ctx.Config.Talos.Version
	k8sVersion := ctx.Config.Kubernetes.Version

	// Set defaults if not configured
	if talosVersion == "" {
		talosVersion = "v1.8.3"
	}
	if k8sVersion == "" {
		k8sVersion = "v1.31.0"
	}

	// Check if snapshot already exists
	labels := map[string]string{
		"os":            "talos",
		"talos-version": talosVersion,
		"k8s-version":   k8sVersion,
		"arch":          arch,
	}

	snapshot, err := ctx.Infra.GetSnapshotByLabels(ctx, labels)
	if err != nil {
		return "", fmt.Errorf("failed to check for existing snapshot: %w", err)
	}

	if snapshot != nil {
		snapshotID := fmt.Sprintf("%d", snapshot.ID)
		log.Printf("[Compute:Image] Found existing Talos snapshot: %s (ID: %s)", snapshot.Description, snapshotID)
		return snapshotID, nil
	}

	// Snapshot doesn't exist - this shouldn't happen if EnsureAllImages was called first
	return "", fmt.Errorf("Talos snapshot not found for %s/%s/%s (should have been pre-built)", talosVersion, k8sVersion, arch)
}
