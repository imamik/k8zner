// Package cluster provides the reconciliation logic for provisioning and managing Hetzner Cloud resources.
package cluster

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/retry"
)

// TalosConfigProducer defines the interface for generating Talos configurations.
type TalosConfigProducer interface {
	GenerateControlPlaneConfig(san []string) ([]byte, error)
	GenerateWorkerConfig() ([]byte, error)
	GetClientConfig() ([]byte, error)
	SetEndpoint(endpoint string)
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
	timeouts          *config.Timeouts

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
		timeouts:          config.LoadTimeouts(),
	}
}

// Reconcile ensures that the desired state matches the actual state.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	log.Println("Starting reconciliation...")

	// 0. Calculate Subnets.
	if err := r.config.CalculateSubnets(); err != nil {
		return fmt.Errorf("failed to calculate subnets: %w", err)
	}

	// 1. Network.
	if err := r.reconcileNetwork(ctx); err != nil {
		return fmt.Errorf("failed to reconcile network: %w", err)
	}

	// Fetch Public IP.
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

// reconcileControlPlane provisions control plane servers and returns a map of ServerName -> PublicIP.
func (r *Reconciler) reconcileControlPlane(ctx context.Context) (map[string]string, error) {
	log.Printf("Reconciling Control Plane...")

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
		// Use LB Public IP as endpoint
		if lb.PublicNet.IPv4.IP != nil {
			lbIP := lb.PublicNet.IPv4.IP.String()
			sans = append(sans, lbIP)

			// UPDATE TALOS ENDPOINT
			// We use the LB IP as the control plane endpoint.
			endpoint := fmt.Sprintf("https://%s:6443", lbIP)
			log.Printf("Setting Talos Endpoint to: %s", endpoint)
			r.talosGenerator.SetEndpoint(endpoint)
		}

		// Also add private IP of LB
		for _, net := range lb.PrivateNet {
			sans = append(sans, net.IP.String())
		}
	}

	// Add Floating IPs if any (Control Plane VIP)
	// if r.config.ControlPlane.PublicVIPIPv4Enabled {
	// 	// TODO: Implement VIP lookup if ID not provided
	// 	// For now assume standard pattern
	// }

	// Generate Talos Config for CP
	cpConfig, err := r.talosGenerator.GenerateControlPlaneConfig(sans)
	if err != nil {
		return nil, err
	}

	// Provision Servers
	ips := make(map[string]string)
	for i, pool := range r.config.ControlPlane.NodePools {
		// Placement Group for Control Plane
		pgName := fmt.Sprintf("%s-%s", r.config.ClusterName, pool.Name)
		pgLabels := map[string]string{
			"cluster": r.config.ClusterName,
			"pool":    pool.Name,
		}
		pg, err := r.pgManager.EnsurePlacementGroup(ctx, pgName, "spread", pgLabels)
		if err != nil {
			return nil, err
		}

		poolIPs, err := r.reconcileNodePool(ctx, pool.Name, pool.Count, pool.ServerType, pool.Location, pool.Image, "control-plane", pool.Labels, string(cpConfig), &pg.ID, i)
		if err != nil {
			return nil, err
		}
		for k, v := range poolIPs {
			ips[k] = v
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

	for i, pool := range r.config.Workers {
		// Placement Group (Managed inside reconcileNodePool for Workers due to sharding)
		// We pass nil here, and handle it inside reconcileNodePool based on pool config and index.
		_, err = r.reconcileNodePool(ctx, pool.Name, pool.Count, pool.ServerType, pool.Location, pool.Image, "worker", pool.Labels, string(workerConfig), nil, i)
		if err != nil {
			return err
		}
	}

	return nil
}

// reconcileNodePool provisions a pool of servers.
func (r *Reconciler) reconcileNodePool(
	ctx context.Context,
	poolName string,
	count int,
	serverType string,
	location string,
	image string,
	role string,
	extraLabels map[string]string,
	userData string,
	pgID *int64,
	poolIndex int,
) (map[string]string, error) {
	ips := make(map[string]string)

	for j := 1; j <= count; j++ {
		// Name: <cluster>-<pool>-<index> (e.g. cluster-control-plane-1)
		serverName := fmt.Sprintf("%s-%s-%d", r.config.ClusterName, poolName, j)

		// Calculate global index for subnet calculations
		// For CP: 10 * np_index + cp_index + 1
		// For Worker: wkr_index + 1
		var hostNum int
		var subnet string
		var err error

		if role == "control-plane" {
			// Terraform: ipv4_private = cidrhost(subnet, np_index * 10 + cp_index + 1)
			subnet, err = r.config.GetSubnetForRole("control-plane", 0)
			if err != nil {
				return nil, err
			}
			hostNum = poolIndex*10 + (j - 1) + 1
		} else {
			// Terraform: ipv4_private = cidrhost(subnet, wkr_index + 1)
			// Note: Terraform iterates worker nodepools and uses separate subnets for each
			// hcloud_network_subnet.worker[np.name]
			// The config.GetSubnetForRole("worker", i) handles the subnet iteration.
			subnet, err = r.config.GetSubnetForRole("worker", poolIndex)
			if err != nil {
				return nil, err
			}
			hostNum = (j - 1) + 1
		}

		privateIP, err := config.CIDRHost(subnet, hostNum)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate private ip: %w", err)
		}

		// Placement Group Sharding for Workers
		// Terraform: ${cluster}-${pool}-pg-${ceil((index+1)/10)}
		var currentPGID *int64
		if role == "worker" && pgID == nil { // Workers manage their own PGs if enabled
			// Check if enabled in config for this pool
			usePG := false
			// Find the pool config again (slightly inefficient but safe)
			for _, p := range r.config.Workers {
				if p.Name == poolName {
					usePG = p.PlacementGroup
					break
				}
			}

			if usePG {
				pgIndex := int((j-1)/10) + 1
				pgName := fmt.Sprintf("%s-%s-pg-%d", r.config.ClusterName, poolName, pgIndex)
				pgLabels := map[string]string{
					"cluster":  r.config.ClusterName,
					"pool":     poolName,
					"role":     "worker",
					"nodepool": poolName,
				}
				pg, err := r.pgManager.EnsurePlacementGroup(ctx, pgName, "spread", pgLabels)
				if err != nil {
					return nil, err
				}
				currentPGID = &pg.ID
			}
		} else {
			currentPGID = pgID
		}

		ip, err := r.ensureServer(ctx, serverName, serverType, location, image, role, poolName, extraLabels, userData, currentPGID, privateIP)
		if err != nil {
			return nil, err
		}
		ips[serverName] = ip
	}
	return ips, nil
}

// ensureServer ensures a server exists and returns its IP.
func (r *Reconciler) ensureServer(
	ctx context.Context,
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
	serverID, err := r.serverProvisioner.GetServerID(ctx, serverName)
	if err != nil {
		return "", err
	}

	if serverID != "" {
		// Server exists, get IP
		ip, err := r.serverProvisioner.GetServerIP(ctx, serverName)
		if err != nil {
			return "", err
		}
		return ip, nil
	}

	// Create
	log.Printf("Creating %s Server %s...", role, serverName)

	// Labels
	labels := map[string]string{
		"cluster": r.config.ClusterName,
		"role":    role,
		"pool":    poolName,
	}
	for k, v := range extraLabels {
		labels[k] = v
	}

	// Image defaulting
	if image == "" {
		image = "talos"
	}

	// Get Network ID
	if r.network == nil {
		return "", fmt.Errorf("network not initialized")
	}
	networkID := r.network.ID

	_, err = r.serverProvisioner.CreateServer(
		ctx,
		serverName,
		image,
		serverType,
		location,
		r.config.SSHKeys,
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
	ipCtx, cancel := context.WithTimeout(ctx, r.timeouts.ServerIP)
	defer cancel()

	var ip string
	err = retry.WithExponentialBackoff(ipCtx, func() error {
		var getErr error
		ip, getErr = r.serverProvisioner.GetServerIP(ctx, serverName)
		if getErr != nil {
			return getErr
		}
		if ip == "" {
			return fmt.Errorf("server IP not yet assigned")
		}
		return nil
	}, retry.WithMaxRetries(r.timeouts.RetryMaxAttempts), retry.WithInitialDelay(r.timeouts.RetryInitialDelay))

	if err != nil {
		return "", fmt.Errorf("failed to get server IP for %s: %w", serverName, err)
	}

	return ip, nil
}
