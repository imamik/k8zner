// Package cluster provides the reconciliation logic for provisioning and managing Hetzner Cloud resources.
package cluster

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

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

	// 2. Firewall
	if err := r.reconcileFirewall(ctx); err != nil {
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

func (r *Reconciler) reconcileNetwork(ctx context.Context) error {
	log.Printf("Reconciling Network %s...", r.config.ClusterName)
	labels := map[string]string{
		"cluster": r.config.ClusterName,
	}

	network, err := r.networkManager.EnsureNetwork(ctx, r.config.ClusterName, r.config.Network.IPv4CIDR, r.config.Network.Zone, labels)
	if err != nil {
		return err
	}
	r.network = network

	// Subnets
	// Note: We do NOT create the parent NodeIPv4CIDR as a subnet, only the leaf subnets (CP, LB, Worker)
	// creating the parent would cause "overlaps" error in HCloud.

	// Control Plane Subnet
	cpSubnet, err := r.config.GetSubnetForRole("control-plane", 0)
	if err != nil { return err }
	err = r.networkManager.EnsureSubnet(ctx, network, cpSubnet, r.config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
	if err != nil { return err }

	// LB Subnet
	lbSubnet, err := r.config.GetSubnetForRole("load-balancer", 0)
	if err != nil { return err }
	err = r.networkManager.EnsureSubnet(ctx, network, lbSubnet, r.config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
	if err != nil { return err }

	// Worker Subnets
	for i := range r.config.Workers {
		wSubnet, err := r.config.GetSubnetForRole("worker", i)
		if err != nil { return err }

		err = r.networkManager.EnsureSubnet(ctx, network, wSubnet, r.config.Network.Zone, hcloud.NetworkSubnetTypeCloud)
		if err != nil { return err }
	}

	return nil
}

func (r *Reconciler) reconcileFirewall(ctx context.Context) error {
	log.Printf("Reconciling Firewall %s...", r.config.ClusterName)

	// Base Rules
	rules := []hcloud.FirewallRule{
		{
			Direction: hcloud.FirewallRuleDirectionIn,
			Protocol:  hcloud.FirewallRuleProtocolICMP,
			SourceIPs: []net.IPNet{{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, {IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}},
		},
		{
			Direction: hcloud.FirewallRuleDirectionIn,
			Protocol:  hcloud.FirewallRuleProtocolTCP,
			Port:      ptr("22"),
			SourceIPs: []net.IPNet{{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, {IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}},
		},
		// API
		{
			Direction: hcloud.FirewallRuleDirectionIn,
			Protocol:  hcloud.FirewallRuleProtocolTCP,
			Port:      ptr("6443"),
			SourceIPs: []net.IPNet{{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, {IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}},
		},
		{
			Direction: hcloud.FirewallRuleDirectionIn,
			Protocol:  hcloud.FirewallRuleProtocolTCP,
			Port:      ptr("50000"), // Talos API
			SourceIPs: []net.IPNet{{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, {IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}},
		},
	}

	labels := map[string]string{
		"cluster": r.config.ClusterName,
	}

	fw, err := r.firewallManager.EnsureFirewall(ctx, r.config.ClusterName, rules, labels)
	if err != nil {
		return err
	}
	r.firewall = fw
	return nil
}

func (r *Reconciler) reconcileLoadBalancers(ctx context.Context) error {
	// API Load Balancer
	// Sum up control plane nodes
	cpCount := 0
	for _, pool := range r.config.ControlPlane.NodePools {
		cpCount += pool.Count
	}

	if cpCount > 0 {
		lbName := fmt.Sprintf("%s-control-plane", r.config.ClusterName)
		log.Printf("Reconciling Load Balancer %s...", lbName)

		labels := map[string]string{
			"cluster": r.config.ClusterName,
			"role": "control-plane-lb",
		}

		lb, err := r.lbManager.EnsureLoadBalancer(ctx, lbName, r.config.Location, "lb11", hcloud.LoadBalancerAlgorithmTypeLeastConnections, labels)
		if err != nil {
			return err
		}

		// Service: 6443
		service := hcloud.LoadBalancerAddServiceOpts{
			Protocol:        hcloud.LoadBalancerServiceProtocolTCP,
			ListenPort:      ptr(6443),
			DestinationPort: ptr(6443),
			HealthCheck: &hcloud.LoadBalancerAddServiceOptsHealthCheck{
				Protocol: hcloud.LoadBalancerServiceProtocolHTTP,
				Port:     ptr(6443),
				Interval: ptr(time.Second * 10),
				Timeout:  ptr(time.Second * 5),
				Retries:  ptr(3),
				HTTP: &hcloud.LoadBalancerAddServiceOptsHealthCheckHTTP{
					Path:        ptr("/version"),
					StatusCodes: []string{"401"},
					TLS:         ptr(true),
				},
			},
		}
		err = r.lbManager.ConfigureService(ctx, lb, service)
		if err != nil {
			return err
		}

		err = r.lbManager.AttachToNetwork(ctx, lb, r.network, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) reconcilePlacementGroups(ctx context.Context) error {
	// Control Plane Spread
	name := fmt.Sprintf("%s-control-plane", r.config.ClusterName)
	labels := map[string]string{"cluster": r.config.ClusterName, "role": "control-plane"}
	_, err := r.pgManager.EnsurePlacementGroup(ctx, name, "spread", labels)
	return err
}

func (r *Reconciler) reconcileFloatingIPs(ctx context.Context) error {
	if r.config.ControlPlane.PublicVIPIPv4Enabled {
		name := fmt.Sprintf("%s-control-plane-ipv4", r.config.ClusterName)
		labels := map[string]string{"cluster": r.config.ClusterName, "role": "control-plane"}
		_, err := r.fipManager.EnsureFloatingIP(ctx, name, r.config.Location, "ipv4", labels)
		if err != nil {
			return err
		}
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
