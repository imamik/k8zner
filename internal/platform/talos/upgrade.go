package talos

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
)

// GetNodeVersion retrieves the current Talos version from a node.
func (g *Generator) GetNodeVersion(ctx context.Context, endpoint string) (string, error) {
	talosClient, err := g.createClient(ctx, endpoint)
	if err != nil {
		return "", fmt.Errorf("failed to create Talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	// Wrap context with target node - required for node-specific operations
	nodeCtx := client.WithNode(ctx, endpoint)

	version, err := talosClient.Version(nodeCtx)
	if err != nil {
		return "", fmt.Errorf("failed to get version: %w", err)
	}

	// Extract version from the first message
	if len(version.Messages) == 0 {
		return "", fmt.Errorf("no version information returned")
	}

	return version.Messages[0].Version.Tag, nil
}

// GetSchematicID retrieves the current schematic ID from a node.
func (g *Generator) GetSchematicID(ctx context.Context, endpoint string) (string, error) {
	talosClient, err := g.createClient(ctx, endpoint)
	if err != nil {
		return "", fmt.Errorf("failed to create Talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	// Wrap context with target node - required for node-specific operations
	nodeCtx := client.WithNode(ctx, endpoint)

	version, err := talosClient.Version(nodeCtx)
	if err != nil {
		return "", fmt.Errorf("failed to get version: %w", err)
	}

	// Extract schematic from the first message
	if len(version.Messages) == 0 {
		return "", fmt.Errorf("no version information returned")
	}

	// Schematic is in the image reference, e.g.:
	// "factory.talos.dev/installer/abc123:v1.9.0"
	imageRef := version.Messages[0].Version.Tag
	parts := strings.Split(imageRef, "/")
	if len(parts) < 3 {
		// No schematic (using official image)
		return "", nil
	}

	// Extract schematic from "installer/abc123:v1.9.0"
	schematicPart := parts[2]
	schematicID := strings.Split(schematicPart, ":")[0]

	return schematicID, nil
}

// UpgradeNode upgrades a single node to the specified image.
func (g *Generator) UpgradeNode(ctx context.Context, endpoint, imageURL string) error {
	talosClient, err := g.createClient(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("failed to create Talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	// Wrap context with target node - required for node-specific operations
	// WithNode tells apid which node to target for the upgrade
	nodeCtx := client.WithNode(ctx, endpoint)

	// Perform upgrade
	// Note: This will cause the node to reboot
	// Parameters: ctx, image, stage=false (upgrade immediately), force=false (don't force)
	_, err = talosClient.Upgrade(nodeCtx, imageURL, false, false)
	if err != nil {
		return fmt.Errorf("failed to initiate upgrade: %w", err)
	}

	return nil
}

// UpgradeKubernetes upgrades the Kubernetes control plane.
// Note: Kubernetes upgrade in Talos happens automatically when nodes are upgraded
// to a new Talos version with a different Kubernetes version. The Talos machinery
// client doesn't expose a direct K8s upgrade method - upgrades are managed by
// upgrading the Talos OS itself with the new Kubernetes version bundled.
func (g *Generator) UpgradeKubernetes(_ context.Context, _, _ string) error {
	// TODO: Implement Kubernetes-only upgrade if needed
	// For now, K8s upgrades happen via Talos node upgrades
	// The Talos image includes the K8s version, so upgrading Talos nodes
	// automatically upgrades K8s when using images with newer K8s versions

	return nil
}

// WaitForNodeReady waits for a node to become ready after upgrade/reboot.
func (g *Generator) WaitForNodeReady(ctx context.Context, endpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	// Wait a bit before first attempt (node needs time to start rebooting)
	time.Sleep(30 * time.Second)

	for time.Now().Before(deadline) {
		// Try to connect and get version (simple liveness check)
		talosClient, err := g.createClient(ctx, endpoint)
		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}

		// Wrap context with target node - required for node-specific operations
		nodeCtx := client.WithNode(ctx, endpoint)

		// Try to get version as a health check
		_, err = talosClient.Version(nodeCtx)
		_ = talosClient.Close()

		if err == nil {
			// Node is responsive
			return nil
		}

		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("node did not become ready within %v", timeout)
}

// HealthCheck performs a basic cluster health check.
func (g *Generator) HealthCheck(ctx context.Context, endpoint string) error {
	talosClient, err := g.createClient(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("failed to create Talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	// Wrap context with target node - required for node-specific operations
	nodeCtx := client.WithNode(ctx, endpoint)

	// Check that we can communicate with Talos API
	version, err := talosClient.Version(nodeCtx)
	if err != nil {
		return fmt.Errorf("talos API not responding: %w", err)
	}

	if len(version.Messages) == 0 {
		return fmt.Errorf("no response from Talos API")
	}

	// TODO: Add more comprehensive health checks:
	// - Check etcd cluster health
	// - Check node ready status in Kubernetes
	// - Check control plane components

	return nil
}

// createClient creates a Talos client for the specified endpoint.
func (g *Generator) createClient(ctx context.Context, endpoint string) (*client.Client, error) {
	// Get talosconfig for authentication
	talosconfigBytes, err := g.GetClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get talos config: %w", err)
	}

	// Parse config from bytes
	cfg, err := config.FromString(string(talosconfigBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse talos config: %w", err)
	}

	// Create client with endpoint - the config contains CA and client certs for TLS
	c, err := client.New(ctx,
		client.WithEndpoints(endpoint),
		client.WithConfig(cfg),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return c, nil
}
