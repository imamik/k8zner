package compute

import (
	"context"
	"fmt"

	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/labels"
	"github.com/imamik/k8zner/internal/util/retry"
)

// ServerSpec defines the configuration for creating a server.
// All fields are self-documenting - no need to remember parameter order.
type ServerSpec struct {
	Name             string
	Type             string
	Location         string
	Image            string // empty or "talos" = auto-detect
	Role             string // "control-plane" or "worker"
	Pool             string
	ExtraLabels      map[string]string
	UserData         string
	PlacementGroup   *int64
	PrivateIP        string
	RDNSIPv4         string // RDNS template for IPv4
	RDNSIPv6         string // RDNS template for IPv6
	EnablePublicIPv4 bool   // Enable public IPv4 (default: true for backwards compatibility)
	EnablePublicIPv6 bool   // Enable public IPv6 (default: true)
}

// ServerInfo holds the result of server creation/lookup.
type ServerInfo struct {
	IP       string
	ServerID int64
}

// ensureServer ensures a server exists and returns its IP and server ID.
func (p *Provisioner) ensureServer(ctx *provisioning.Context, spec ServerSpec) (ServerInfo, error) {
	// Check if exists
	serverIDStr, err := ctx.Infra.GetServerID(ctx, spec.Name)
	if err != nil {
		return ServerInfo{}, err
	}

	if serverIDStr != "" {
		// Server exists, get IP and parse server ID
		ip, err := ctx.Infra.GetServerIP(ctx, spec.Name)
		if err != nil {
			return ServerInfo{}, err
		}
		var serverID int64
		if _, err := fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
			return ServerInfo{}, fmt.Errorf("failed to parse server ID: %w", err)
		}
		return ServerInfo{IP: ip, ServerID: serverID}, nil
	}

	// Create
	ctx.Logger.Printf("[%s] Creating %s server %s...", phase, spec.Role, spec.Name)

	// Labels
	serverLabels := labels.NewLabelBuilder(ctx.Config.ClusterName).
		WithRole(spec.Role).
		WithPool(spec.Pool).
		WithTestIDIfSet(ctx.Config.TestID).
		Merge(spec.ExtraLabels).
		Build()

	// Image defaulting - if empty or "talos", ensure the versioned image exists
	image := spec.Image
	if image == "" || image == "talos" {
		image, err = p.ensureImage(ctx, spec.Type, spec.Location)
		if err != nil {
			return ServerInfo{}, fmt.Errorf("failed to ensure Talos image: %w", err)
		}
		ctx.Logger.Printf("[%s] Using Talos image: %s", phase, image)
	}

	// Get Network ID
	if ctx.State == nil || ctx.State.Network == nil {
		return ServerInfo{}, fmt.Errorf("network not initialized in provisioning state")
	}
	networkID := ctx.State.Network.ID

	// Default to dual-stack if not specified (for backwards compatibility)
	enableIPv4 := spec.EnablePublicIPv4
	enableIPv6 := spec.EnablePublicIPv6
	// If neither is explicitly set, default to dual-stack
	if !enableIPv4 && !enableIPv6 {
		enableIPv4 = true
		enableIPv6 = true
	}

	_, err = ctx.Infra.CreateServer(
		ctx,
		spec.Name,
		image,
		spec.Type,
		spec.Location,
		ctx.Config.SSHKeys,
		serverLabels,
		spec.UserData,
		spec.PlacementGroup,
		networkID,
		spec.PrivateIP,
		enableIPv4,
		enableIPv6,
	)
	if err != nil {
		return ServerInfo{}, fmt.Errorf("failed to create server %s: %w", spec.Name, err)
	}

	// Get IP after creation with retry logic and configurable timeout
	ipCtx, cancel := context.WithTimeout(ctx, ctx.Timeouts.ServerIP)
	defer cancel()

	var ip string
	err = retry.WithExponentialBackoff(ipCtx, func() error {
		var getErr error
		ip, getErr = ctx.Infra.GetServerIP(ctx, spec.Name)
		if getErr != nil {
			return getErr
		}
		if ip == "" {
			return fmt.Errorf("server IP not yet assigned")
		}
		return nil
	}, retry.WithMaxRetries(ctx.Timeouts.RetryMaxAttempts), retry.WithInitialDelay(ctx.Timeouts.RetryInitialDelay))

	if err != nil {
		return ServerInfo{}, fmt.Errorf("failed to get server IP for %s: %w", spec.Name, err)
	}

	// Get server ID (reuse serverIDStr from earlier check)
	serverIDStr, err = ctx.Infra.GetServerID(ctx, spec.Name)
	if err != nil {
		return ServerInfo{}, fmt.Errorf("failed to get server ID: %w", err)
	}
	var serverID int64
	if _, err = fmt.Sscanf(serverIDStr, "%d", &serverID); err != nil {
		return ServerInfo{}, fmt.Errorf("failed to parse server ID: %w", err)
	}

	// Apply RDNS if configured
	if spec.RDNSIPv4 != "" || spec.RDNSIPv6 != "" {
		if err := p.applyServerRDNSSimple(ctx, serverID, spec.Name, ip, spec.RDNSIPv4, spec.RDNSIPv6, spec.Role, spec.Pool); err != nil {
			// Log error but don't fail server creation
			ctx.Logger.Printf("[%s] Warning: Failed to set RDNS for %s: %v", phase, spec.Name, err)
		}
	}

	return ServerInfo{IP: ip, ServerID: serverID}, nil
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
	snapshotLabels := map[string]string{
		"os":            "talos",
		"talos-version": talosVersion,
		"k8s-version":   k8sVersion,
		"arch":          arch,
	}

	snapshot, err := ctx.Infra.GetSnapshotByLabels(ctx, snapshotLabels)
	if err != nil {
		return "", fmt.Errorf("failed to check for existing snapshot: %w", err)
	}

	if snapshot != nil {
		snapshotID := fmt.Sprintf("%d", snapshot.ID)
		ctx.Logger.Printf("[%s] Found existing Talos snapshot: %s (ID: %s)", phase, snapshot.Description, snapshotID)
		return snapshotID, nil
	}

	// Snapshot doesn't exist - this shouldn't happen if EnsureAllImages was called first
	return "", fmt.Errorf("talos snapshot not found for %s/%s/%s (should have been pre-built)", talosVersion, k8sVersion, arch)
}
