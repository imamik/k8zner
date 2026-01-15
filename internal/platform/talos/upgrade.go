package talos

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/client"
)

// GetNodeVersion retrieves the current Talos version from a node.
func (g *Generator) GetNodeVersion(ctx context.Context, endpoint string) (string, error) {
	talosClient, err := g.createClient(ctx, endpoint)
	if err != nil {
		return "", fmt.Errorf("failed to create Talos client: %w", err)
	}
	defer talosClient.Close()

	version, err := talosClient.Version(ctx)
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
	defer talosClient.Close()

	version, err := talosClient.Version(ctx)
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
	defer talosClient.Close()

	// Perform upgrade
	// Note: This will cause the node to reboot
	if err := talosClient.Upgrade(ctx, imageURL); err != nil {
		return fmt.Errorf("failed to initiate upgrade: %w", err)
	}

	return nil
}

// UpgradeKubernetes upgrades the Kubernetes control plane.
func (g *Generator) UpgradeKubernetes(ctx context.Context, endpoint, targetVersion string) error {
	talosClient, err := g.createClient(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("failed to create Talos client: %w", err)
	}
	defer talosClient.Close()

	// Strip 'v' prefix if present (K8s version format)
	targetVersion = strings.TrimPrefix(targetVersion, "v")

	// Upgrade Kubernetes
	if err := talosClient.UpgradeK8s(ctx, targetVersion); err != nil {
		return fmt.Errorf("failed to upgrade Kubernetes: %w", err)
	}

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

		// Try to get version as a health check
		_, err = talosClient.Version(ctx)
		talosClient.Close()

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
	defer talosClient.Close()

	// Check that we can communicate with Talos API
	version, err := talosClient.Version(ctx)
	if err != nil {
		return fmt.Errorf("Talos API not responding: %w", err)
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
	talosconfig, err := g.GetClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get talos config: %w", err)
	}

	// Create client with endpoint
	c, err := client.New(ctx,
		client.WithEndpoints(endpoint),
		client.WithConfig(talosconfig),
		client.WithTLSConfig(&tls.Config{
			InsecureSkipVerify: false,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return c, nil
}
