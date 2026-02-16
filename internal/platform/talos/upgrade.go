package talos

import (
	"context"
	"fmt"
	"time"

	"github.com/imamik/k8zner/internal/provisioning"

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

// UpgradeNode upgrades a single node to the specified image.
// The opts parameter allows configuring upgrade behavior (stage, force).
func (g *Generator) UpgradeNode(ctx context.Context, endpoint, imageURL string, opts provisioning.UpgradeOptions) error {
	talosClient, err := g.createClient(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("failed to create Talos client: %w", err)
	}
	defer func() { _ = talosClient.Close() }()

	// Wrap context with target node - required for node-specific operations
	// WithNode tells apid which node to target for the upgrade
	nodeCtx := client.WithNode(ctx, endpoint)

	// Perform upgrade
	// Note: This will cause the node to reboot (unless opts.Stage is true)
	// Parameters: ctx, image, stage, force
	_, err = talosClient.Upgrade(nodeCtx, imageURL, opts.Stage, opts.Force)
	if err != nil {
		return fmt.Errorf("failed to initiate upgrade: %w", err)
	}

	return nil
}

// UpgradeKubernetes upgrades the Kubernetes control plane.
//
// Note: In Talos Linux, Kubernetes upgrades happen automatically when nodes are upgraded
// to a new Talos version with a different Kubernetes version. The Talos machinery client
// doesn't expose a direct K8s-only upgrade method because Kubernetes is tightly coupled
// with the Talos OS release.
//
// To upgrade Kubernetes in a Talos cluster:
// 1. Upgrade control plane nodes to a Talos version containing the desired K8s version
// 2. Talos automatically handles the Kubernetes upgrade during the node upgrade
// 3. Upgrade worker nodes to complete the cluster upgrade
//
// This method is a no-op since K8s upgrades are handled by UpgradeNode.
func (g *Generator) UpgradeKubernetes(_ context.Context, _, _ string) error {
	return nil
}

const (
	// nodeRebootInitialWait is the initial wait time before checking node readiness after reboot.
	nodeRebootInitialWait = 30 * time.Second

	// nodeReadyCheckInterval is the interval between node readiness checks.
	nodeReadyCheckInterval = 10 * time.Second
)

// WaitForNodeReady waits for a node to become ready after upgrade/reboot.
func (g *Generator) WaitForNodeReady(ctx context.Context, endpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	// Wait a bit before first attempt (node needs time to start rebooting)
	time.Sleep(nodeRebootInitialWait)

	for time.Now().Before(deadline) {
		// Try to connect and get version (simple liveness check)
		talosClient, err := g.createClient(ctx, endpoint)
		if err != nil {
			time.Sleep(nodeReadyCheckInterval)
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

		time.Sleep(nodeReadyCheckInterval)
	}

	return fmt.Errorf("node did not become ready within %v", timeout)
}

// HealthCheck performs a basic cluster health check.
// It verifies that the Talos API is responsive and returns version information.
//
// Note: This performs a basic connectivity check. For comprehensive health validation
// including etcd cluster health, Kubernetes node status, and control plane components,
// use the operator's health check reconciliation which has access to the Kubernetes API.
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
