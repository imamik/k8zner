// Package cluster provides the reconciliation logic for provisioning and managing Hetzner Cloud resources.
package cluster

import (
	"context"
	"fmt"
	"log"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
)

// Reconciler is responsible for reconciling the state of the cluster.
type TalosConfigProducer interface {
	GenerateControlPlaneConfig(san []string) ([]byte, error)
	GenerateWorkerConfig() ([]byte, error)
}

// Reconciler is responsible for reconciling the state of the cluster.
type Reconciler struct {
	serverProvisioner   hcloud_internal.ServerProvisioner
	networkManager      hcloud_internal.NetworkManager
	firewallManager     hcloud_internal.FirewallManager
	lbManager           hcloud_internal.LoadBalancerManager
	pgManager           hcloud_internal.PlacementGroupManager
	fipManager          hcloud_internal.FloatingIPManager
	talosGenerator      TalosConfigProducer
	config              *config.Config

	// State
	network  *hcloud.Network
	firewall *hcloud.Firewall
}

// NewReconciler creates a new Reconciler.
func NewReconciler(
	infra hcloud_internal.InfrastructureManager,
	talosGenerator TalosConfigProducer,
	cfg *config.Config,
) *Reconciler {
	return &Reconciler{
		serverProvisioner:   infra,
		networkManager:      infra,
		firewallManager:     infra,
		lbManager:           infra,
		pgManager:           infra,
		fipManager:          infra,
		talosGenerator:      talosGenerator,
		config:              cfg,
	}
}

// Reconcile ensures that the desired state matches the actual state.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	log.Println("Starting reconciliation...")

	// 0. Calculate Subnets
	if err := r.config.CalculateSubnets(); err != nil {
		return fmt.Errorf("failed to calculate subnets: %w", err)
	}

	// 1. Network
	if err := r.reconcileNetwork(ctx); err != nil {
		return fmt.Errorf("failed to reconcile network: %w", err)
	}

	// Fetch Public IP if needed (simplified check)
	var publicIP string
	// Ideally check config.Firewall.UseCurrentIPv4 etc.
	// For now, always fetch to support the logic.
	// We need to cast serverProvisioner to RealClient or add GetPublicIP to interface.
	// Adding to interface is cleaner but for now type assert?
	// Actually we should add it to InfrastructureManager interface.
	// But let's assume if we can't get it, we proceed without it (empty string).
	if client, ok := r.serverProvisioner.(interface {
		GetPublicIP(ctx context.Context) (string, error)
	}); ok {
		ip, err := client.GetPublicIP(ctx)
		if err == nil {
			publicIP = ip
		} else {
			log.Printf("Warning: Failed to detect public IP: %v", err)
		}
	}

	// 2. Firewall
	if err := r.reconcileFirewall(ctx, publicIP); err != nil {
		return fmt.Errorf("failed to reconcile firewall: %w", err)
	}

	// 3. Load Balancers
	if err := r.reconcileLoadBalancers(ctx); err != nil {
		return fmt.Errorf("failed to reconcile load balancers: %w", err)
	}

	// 4. Placement Groups
	if err := r.reconcilePlacementGroups(ctx); err != nil {
		return fmt.Errorf("failed to reconcile placement groups: %w", err)
	}

	// 5. Floating IPs
	if err := r.reconcileFloatingIPs(ctx); err != nil {
		return fmt.Errorf("failed to reconcile floating IPs: %w", err)
	}

	// 6. Servers (CP & Worker)
	// For this iteration, we focus on Infra config.
	// The existing ReconcileServers code needs to be updated to attach to this infra.
	if err := r.reconcileControlPlane(ctx); err != nil {
		return fmt.Errorf("failed to reconcile control plane: %w", err)
	}

	return nil
}

func (r *Reconciler) reconcileControlPlane(ctx context.Context) error {
	return nil
}

func (r *Reconciler) reconcileWorkers(ctx context.Context) error {
	return nil
}

// Helpers
func ptr[T any](v T) *T {
	return &v
}
