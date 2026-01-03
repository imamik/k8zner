// Package cluster provides the reconciliation logic for provisioning and managing Hetzner Cloud resources.
package cluster

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
)

// Reconciler is responsible for reconciling the state of the cluster.
type TalosConfigProducer interface {
	GenerateControlPlaneConfig(san []string) ([]byte, error)
	GenerateWorkerConfig() ([]byte, error)
	GetClientConfig() ([]byte, error)
}

// Reconciler is responsible for reconciling the state of the cluster.
type Reconciler struct {
	serverProvisioner hcloud_internal.ServerProvisioner
	networkManager    hcloud_internal.NetworkManager
	firewallManager   hcloud_internal.FirewallManager
	lbManager         hcloud_internal.LoadBalancerManager
	pgManager         hcloud_internal.PlacementGroupManager
	fipManager        hcloud_internal.FloatingIPManager
	certManager       hcloud_internal.CertificateManager
	infra             hcloud_internal.InfrastructureManager // Combined interface for Bootstrapper
	talosGenerator    TalosConfigProducer
	config            *config.Config
	bootstrapper      *Bootstrapper

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
		serverProvisioner: infra,
		networkManager:    infra,
		firewallManager:   infra,
		lbManager:         infra,
		pgManager:         infra,
		fipManager:        infra,
		certManager:       infra,
		infra:             infra,
		talosGenerator:    talosGenerator,
		config:            cfg,
		bootstrapper:      NewBootstrapper(infra),
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

	// Fetch Public IP
	var publicIP string
	if ip, err := r.infra.GetPublicIP(ctx); err == nil {
		publicIP = ip
	} else {
		log.Printf("Warning: Failed to detect public IP: %v", err)
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

	// 6. Control Plane Servers
	cpIPs, err := r.reconcileControlPlane(ctx)
	if err != nil {
		return fmt.Errorf("failed to reconcile control plane: %w", err)
	}

	// 7. Bootstrap
	if len(cpIPs) > 0 {
		clientCfg, err := r.talosGenerator.GetClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get client config: %w", err)
		}

		// Use the first CP node's IP for bootstrapping
		var firstCPIP string
		var names []string
		for name := range cpIPs {
			names = append(names, name)
		}
		sort.Strings(names)
		if len(names) > 0 {
			firstCPIP = cpIPs[names[0]]
		}

		if firstCPIP != "" {
			if err := r.bootstrapper.Bootstrap(ctx, r.config.ClusterName, firstCPIP, clientCfg); err != nil {
				return fmt.Errorf("failed to bootstrap cluster: %w", err)
			}
		}
	}

	// 8. Worker Servers
	if err := r.reconcileWorkers(ctx); err != nil {
		return fmt.Errorf("failed to reconcile workers: %w", err)
	}

	return nil
}

// reconcileControlPlane provisions control plane servers and returns a map of ServerName -> PublicIP
func (r *Reconciler) reconcileControlPlane(ctx context.Context) (map[string]string, error) {
	log.Printf("Reconciling Control Plane...")

	ips := make(map[string]string)

	// Collect all SANs
	var sans []string

	// LB IP (Public) - if Ingress enabled or API LB?
	// The API LB is "kube-api".
	lbName := fmt.Sprintf("%s-kube-api", r.config.ClusterName)
	lb, err := r.lbManager.GetLoadBalancer(ctx, lbName)
	if err != nil {
		return nil, err
	}
	if lb != nil {
		sans = append(sans, lb.PublicNet.IPv4.IP.String())
		// Also add private IP of LB
		for _, net := range lb.PrivateNet {
			sans = append(sans, net.IP.String())
		}
	}

	// Add Floating IPs if any (Control Plane VIP)
	if r.config.ControlPlane.PublicVIPIPv4Enabled {
		// TODO: Implement VIP lookup if ID not provided
		// For now assume standard pattern
	}

	// Generate Talos Config for CP
	cpConfig, err := r.talosGenerator.GenerateControlPlaneConfig(sans)
	if err != nil {
		return nil, err
	}

	// Create Servers
	for _, pool := range r.config.ControlPlane.NodePools {
		for j := 1; j <= pool.Count; j++ {
			// Name: <cluster>-<pool>-<index> (e.g. cluster-control-plane-1)
			serverName := fmt.Sprintf("%s-%s-%d", r.config.ClusterName, pool.Name, j)

			// Check if exists
			serverID, err := r.serverProvisioner.GetServerID(ctx, serverName)
			if err != nil {
				return nil, err
			}

			if serverID != "" {
				// Server exists, get IP
				ip, err := r.serverProvisioner.GetServerIP(ctx, serverName)
				if err != nil {
					return nil, err
				}
				ips[serverName] = ip
				continue
			}

			// Create
			log.Printf("Creating Control Plane Server %s...", serverName)

			// Labels
			labels := map[string]string{
				"cluster": r.config.ClusterName,
				"role":    "control-plane",
				"pool":    pool.Name,
			}
			for k, v := range pool.Labels {
				labels[k] = v
			}

			// SSH Keys
			// Use all defined keys
			sshKeys := r.config.SSHKeys

			// UserData
			userData := string(cpConfig)

			// Create
			image := pool.Image
			if image == "" {
				image = "talos"
			}

			_, err = r.serverProvisioner.CreateServer(
				ctx,
				serverName,
				image,
				pool.ServerType,
				pool.Location,
				sshKeys,
				labels,
				userData,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create server %s: %w", serverName, err)
			}

			// Get IP after creation
			ip, err := r.serverProvisioner.GetServerIP(ctx, serverName)
			if err != nil {
				// Retry?
				time.Sleep(2 * time.Second)
				ip, err = r.serverProvisioner.GetServerIP(ctx, serverName)
				if err != nil {
					return nil, err
				}
			}
			ips[serverName] = ip
		}
	}

	return ips, nil
}

func (r *Reconciler) reconcileWorkers(ctx context.Context) error {
	log.Printf("Reconciling Workers...")

	workerConfig, err := r.talosGenerator.GenerateWorkerConfig()
	if err != nil {
		return err
	}

	for _, pool := range r.config.Workers {
		// Placement Group
		if pool.PlacementGroup {
			pgName := fmt.Sprintf("%s-%s", r.config.ClusterName, pool.Name)
			labels := map[string]string{
				"cluster": r.config.ClusterName,
				"pool":    pool.Name,
			}
			_, err := r.pgManager.EnsurePlacementGroup(ctx, pgName, "spread", labels)
			if err != nil {
				return err
			}
		}

		for j := 1; j <= pool.Count; j++ {
			serverName := fmt.Sprintf("%s-%s-%d", r.config.ClusterName, pool.Name, j)

			// Check if exists
			serverID, err := r.serverProvisioner.GetServerID(ctx, serverName)
			if err != nil {
				return err
			}
			if serverID != "" {
				continue
			}

			// Create
			log.Printf("Creating Worker Server %s...", serverName)

			labels := map[string]string{
				"cluster": r.config.ClusterName,
				"role":    "worker",
				"pool":    pool.Name,
			}
			for k, v := range pool.Labels {
				labels[k] = v
			}

			image := pool.Image
			if image == "" {
				image = "talos"
			}

			_, err = r.serverProvisioner.CreateServer(
				ctx,
				serverName,
				image,
				pool.ServerType,
				pool.Location,
				r.config.SSHKeys,
				labels,
				string(workerConfig),
			)
			if err != nil {
				return fmt.Errorf("failed to create server %s: %w", serverName, err)
			}
		}
	}

	return nil
}

// Helpers
func ptr[T any](v T) *T {
	return &v
}
