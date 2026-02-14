// Package upgrade handles Talos OS and Kubernetes upgrades.
package upgrade

import (
	"fmt"
	"time"

	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
)

const (
	phase = "Upgrade"

	// Upgrade health check configuration
	upgradeHealthCheckRetries = 3
	nodeUpgradeTimeout        = 10 * time.Minute
)

// ProvisionerOptions contains options for the upgrade provisioner.
type ProvisionerOptions struct {
	DryRun          bool
	SkipHealthCheck bool
}

// Provisioner handles cluster upgrades.
type Provisioner struct {
	opts ProvisionerOptions
}

// NewProvisioner creates a new upgrade provisioner.
func NewProvisioner(opts ProvisionerOptions) *Provisioner {
	return &Provisioner{
		opts: opts,
	}
}

// Name returns the phase name.
func (p *Provisioner) Name() string {
	return phase
}

// Provision performs the upgrade process.
//
// The upgrade happens in phases:
// 1. Validate configuration and get current cluster state
// 2. Upgrade control plane nodes sequentially (maintains quorum)
// 3. Upgrade worker nodes in parallel (faster, no quorum concerns)
// 4. Upgrade Kubernetes version (if changed)
// 5. Final health check
func (p *Provisioner) Provision(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[%s] Starting cluster upgrade for: %s", phase, ctx.Config.ClusterName)

	// Phase 1: Validate and get current state
	if err := p.validateAndPrepare(ctx); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Phase 2: Upgrade control plane nodes
	if err := p.upgradeControlPlane(ctx); err != nil {
		return fmt.Errorf("control plane upgrade failed: %w", err)
	}

	// Phase 3: Upgrade worker nodes
	if err := p.upgradeWorkers(ctx); err != nil {
		return fmt.Errorf("worker upgrade failed: %w", err)
	}

	// Phase 4: Upgrade Kubernetes (if version changed)
	if err := p.upgradeKubernetes(ctx); err != nil {
		return fmt.Errorf("kubernetes upgrade failed: %w", err)
	}

	// Phase 5: Final health check
	if !p.opts.SkipHealthCheck && !p.opts.DryRun {
		ctx.Observer.Printf("[%s] Performing final health check...", phase)
		if err := p.healthCheck(ctx); err != nil {
			return fmt.Errorf("final health check failed: %w", err)
		}
		ctx.Observer.Printf("[%s] Cluster health check passed", phase)
	}

	ctx.Observer.Printf("[%s] Cluster upgrade completed successfully", phase)
	return nil
}

// validateAndPrepare validates the configuration and retrieves current cluster state.
func (p *Provisioner) validateAndPrepare(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[%s] Validating configuration...", phase)

	// Get all servers in the cluster
	servers, err := p.getClusterServers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster servers: %w", err)
	}

	if len(servers) == 0 {
		return fmt.Errorf("no servers found in cluster %s", ctx.Config.ClusterName)
	}

	ctx.Observer.Printf("[%s] Found %d nodes in cluster", phase, len(servers))

	// If dry run, show what would be upgraded
	if p.opts.DryRun {
		return p.dryRunReport(ctx, servers)
	}

	return nil
}

// upgradeControlPlane upgrades control plane nodes sequentially.
func (p *Provisioner) upgradeControlPlane(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[%s] Upgrading control plane nodes...", phase)

	// Get control plane node IPs
	cpIPs, err := p.getControlPlaneIPs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get control plane IPs: %w", err)
	}

	if len(cpIPs) == 0 {
		ctx.Observer.Printf("[%s] No control plane nodes found, skipping", phase)
		return nil
	}

	ctx.Observer.Printf("[%s] Found %d control plane nodes", phase, len(cpIPs))

	// Upgrade each control plane node sequentially
	for i, nodeIP := range cpIPs {
		ctx.Observer.Printf("[%s] Upgrading control plane node %d/%d (%s)...", phase, i+1, len(cpIPs), nodeIP)

		if p.opts.DryRun {
			ctx.Observer.Printf("[%s] [DRY RUN] Would upgrade node %s", phase, nodeIP)
			continue
		}

		// Check current version
		currentVersion, err := ctx.Talos.GetNodeVersion(ctx, nodeIP)
		if err != nil {
			return fmt.Errorf("failed to get version for node %s: %w", nodeIP, err)
		}

		targetVersion := ctx.Config.Talos.Version
		if currentVersion == targetVersion {
			ctx.Observer.Printf("[%s] Node %s already at version %s, skipping", phase, nodeIP, targetVersion)
			continue
		}

		ctx.Observer.Printf("[%s] Node %s: %s → %s", phase, nodeIP, currentVersion, targetVersion)

		// Perform upgrade
		if err := p.upgradeNode(ctx, nodeIP, "control-plane"); err != nil {
			return fmt.Errorf("failed to upgrade node %s: %w", nodeIP, err)
		}

		// Health check after each control plane node (critical for quorum)
		if !p.opts.SkipHealthCheck {
			ctx.Observer.Printf("[%s] Checking cluster health after node %s...", phase, nodeIP)
			if err := p.healthCheckWithRetry(ctx, upgradeHealthCheckRetries); err != nil {
				return fmt.Errorf("health check failed after upgrading %s: %w", nodeIP, err)
			}
			ctx.Observer.Printf("[%s] Cluster health check passed", phase)
		}
	}

	ctx.Observer.Printf("[%s] Control plane upgrade completed", phase)
	return nil
}

// upgradeWorkers upgrades worker nodes sequentially.
//
// Note: Worker upgrades are performed sequentially to maintain observability and
// prevent simultaneous drain operations that could impact cluster stability.
// While workers don't have quorum requirements like control planes, sequential
// upgrades provide better control over cluster capacity during the upgrade window.
//
// Future enhancement: Consider adding a MaxConcurrentUpgrades option for large
// clusters where controlled parallelism would reduce total upgrade time.
func (p *Provisioner) upgradeWorkers(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[%s] Upgrading worker nodes...", phase)

	// Get worker node IPs
	workerIPs, err := p.getWorkerIPs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get worker IPs: %w", err)
	}

	if len(workerIPs) == 0 {
		ctx.Observer.Printf("[%s] No worker nodes found, skipping", phase)
		return nil
	}

	ctx.Observer.Printf("[%s] Found %d worker nodes", phase, len(workerIPs))

	// Upgrade each worker node sequentially
	for i, nodeIP := range workerIPs {
		ctx.Observer.Printf("[%s] Upgrading worker node %d/%d (%s)...", phase, i+1, len(workerIPs), nodeIP)

		if p.opts.DryRun {
			ctx.Observer.Printf("[%s] [DRY RUN] Would upgrade node %s", phase, nodeIP)
			continue
		}

		// Check current version
		currentVersion, err := ctx.Talos.GetNodeVersion(ctx, nodeIP)
		if err != nil {
			return fmt.Errorf("failed to get version for node %s: %w", nodeIP, err)
		}

		targetVersion := ctx.Config.Talos.Version
		if currentVersion == targetVersion {
			ctx.Observer.Printf("[%s] Node %s already at version %s, skipping", phase, nodeIP, targetVersion)
			continue
		}

		ctx.Observer.Printf("[%s] Node %s: %s → %s", phase, nodeIP, currentVersion, targetVersion)

		// Perform upgrade
		if err := p.upgradeNode(ctx, nodeIP, "worker"); err != nil {
			return fmt.Errorf("failed to upgrade node %s: %w", nodeIP, err)
		}
	}

	ctx.Observer.Printf("[%s] Worker upgrade completed", phase)
	return nil
}

// upgradeKubernetes upgrades the Kubernetes control plane.
func (p *Provisioner) upgradeKubernetes(ctx *provisioning.Context) error {
	// Get primary control plane endpoint for K8s upgrade
	cpIPs, err := p.getControlPlaneIPs(ctx)
	if err != nil || len(cpIPs) == 0 {
		ctx.Observer.Printf("[%s] No control plane nodes available for K8s upgrade, skipping", phase)
		return nil
	}

	primaryEndpoint := cpIPs[0]

	// Check if K8s version changed (we'd need to track current K8s version)
	// For now, we'll attempt upgrade if config specifies a version
	targetK8sVersion := ctx.Config.Kubernetes.Version
	if targetK8sVersion == "" {
		ctx.Observer.Printf("[%s] No Kubernetes version specified, skipping K8s upgrade", phase)
		return nil
	}

	ctx.Observer.Printf("[%s] Upgrading Kubernetes to version %s...", phase, targetK8sVersion)

	if p.opts.DryRun {
		ctx.Observer.Printf("[%s] [DRY RUN] Would upgrade Kubernetes to %s", phase, targetK8sVersion)
		return nil
	}

	// Perform Kubernetes upgrade
	if err := ctx.Talos.UpgradeKubernetes(ctx, primaryEndpoint, targetK8sVersion); err != nil {
		return fmt.Errorf("failed to upgrade Kubernetes: %w", err)
	}

	ctx.Observer.Printf("[%s] Kubernetes upgrade completed", phase)
	return nil
}

// upgradeNode upgrades a single node.
func (p *Provisioner) upgradeNode(ctx *provisioning.Context, nodeIP, _ string) error {
	// Build upgrade image URL
	var imageURL string
	if ctx.Config.Talos.SchematicID != "" {
		// Use Talos factory with schematic for custom extensions
		imageURL = fmt.Sprintf("factory.talos.dev/installer/%s:%s",
			ctx.Config.Talos.SchematicID,
			ctx.Config.Talos.Version)
	} else {
		// Use official installer without schematic
		imageURL = fmt.Sprintf("ghcr.io/siderolabs/installer:%s",
			ctx.Config.Talos.Version)
	}

	// Build upgrade options from config
	// See: terraform/variables.tf talos_upgrade_* variables
	opts := provisioning.UpgradeOptions{
		Stage: ctx.Config.Talos.Upgrade.Stage,
		Force: ctx.Config.Talos.Upgrade.Force,
	}

	// Perform upgrade
	if err := ctx.Talos.UpgradeNode(ctx, nodeIP, imageURL, opts); err != nil {
		return err
	}

	// Wait for node to come back up
	ctx.Observer.Printf("[%s] Waiting for node %s to reboot...", phase, nodeIP)
	if err := ctx.Talos.WaitForNodeReady(ctx, nodeIP, nodeUpgradeTimeout); err != nil {
		return fmt.Errorf("node failed to become ready: %w", err)
	}

	ctx.Observer.Printf("[%s] Node %s upgraded successfully", phase, nodeIP)
	return nil
}

// healthCheck performs a cluster health check.
func (p *Provisioner) healthCheck(ctx *provisioning.Context) error {
	cpIPs, err := p.getControlPlaneIPs(ctx)
	if err != nil || len(cpIPs) == 0 {
		return fmt.Errorf("no control plane nodes available for health check")
	}

	// Check against primary control plane node
	primaryNode := cpIPs[0]
	return ctx.Talos.HealthCheck(ctx, primaryNode)
}

// healthCheckWithRetry performs health check with retry logic.
func (p *Provisioner) healthCheckWithRetry(ctx *provisioning.Context, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		err := p.healthCheck(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if i < maxRetries-1 {
			ctx.Observer.Printf("[%s] Health check failed (attempt %d/%d), retrying in 10s...", phase, i+1, maxRetries)
			time.Sleep(10 * time.Second)
		}
	}
	return fmt.Errorf("health check failed after %d attempts: %w", maxRetries, lastErr)
}

// Helper methods to get server IPs

func (p *Provisioner) getClusterServers(ctx *provisioning.Context) ([]string, error) {
	servers, err := ctx.Infra.GetServersByLabel(ctx, map[string]string{
		"cluster": ctx.Config.ClusterName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query servers: %w", err)
	}

	var names []string
	for _, server := range servers {
		names = append(names, server.Name)
	}
	return names, nil
}

func (p *Provisioner) getControlPlaneIPs(ctx *provisioning.Context) ([]string, error) {
	servers, err := ctx.Infra.GetServersByLabel(ctx, map[string]string{
		"cluster": ctx.Config.ClusterName,
		"role":    "control-plane",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query control plane servers: %w", err)
	}

	var ips []string
	for _, server := range servers {
		// Use public IP for Talos API access (accessible on port 50000)
		if ip := hcloud.ServerIPv4(server); ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips, nil
}

func (p *Provisioner) getWorkerIPs(ctx *provisioning.Context) ([]string, error) {
	servers, err := ctx.Infra.GetServersByLabel(ctx, map[string]string{
		"cluster": ctx.Config.ClusterName,
		"role":    "worker",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query worker servers: %w", err)
	}

	var ips []string
	for _, server := range servers {
		// Use public IP for Talos API access (accessible on port 50000)
		if ip := hcloud.ServerIPv4(server); ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips, nil
}

func (p *Provisioner) dryRunReport(ctx *provisioning.Context, servers []string) error {
	ctx.Observer.Printf("[%s] === DRY RUN REPORT ===", phase)
	ctx.Observer.Printf("[%s] Target Talos version: %s", phase, ctx.Config.Talos.Version)
	ctx.Observer.Printf("[%s] Target Kubernetes version: %s", phase, ctx.Config.Kubernetes.Version)
	ctx.Observer.Printf("[%s] Nodes to check: %d", phase, len(servers))
	ctx.Observer.Printf("[%s] ", phase)
	ctx.Observer.Printf("[%s] This is a dry run. No changes will be made.", phase)
	ctx.Observer.Printf("[%s] Run without --dry-run to perform the actual upgrade.", phase)
	ctx.Observer.Printf("[%s] === END REPORT ===", phase)
	return nil
}
