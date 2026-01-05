// Package cluster provides the reconciliation logic for provisioning and managing Hetzner Cloud resources.
package cluster

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/image"
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
	snapshotManager   hcloud_internal.SnapshotManager
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
		snapshotManager:   infra,
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

	// 1.5. Pre-build all required Talos images in parallel with public IP fetch
	type asyncResult struct {
		name string
		err  error
	}
	resultChan := make(chan asyncResult, 2)

	// Start image building
	go func() {
		err := r.ensureAllImages(ctx)
		resultChan <- asyncResult{name: "images", err: err}
	}()

	// Start public IP fetch
	var publicIP string
	go func() {
		if ip, err := r.infra.GetPublicIP(ctx); err == nil {
			publicIP = ip
			resultChan <- asyncResult{name: "publicIP", err: nil}
		} else {
			log.Printf("Warning: Failed to detect public IP: %v", err)
			resultChan <- asyncResult{name: "publicIP", err: err}
		}
	}()

	// Wait for both to complete
	for i := 0; i < 2; i++ {
		result := <-resultChan
		if result.name == "images" && result.err != nil {
			return fmt.Errorf("failed to ensure Talos images: %w", result.err)
		}
	}

	// 2-5. Parallelize infrastructure setup after network
	log.Printf("=== PARALLELIZING INFRASTRUCTURE SETUP at %s ===", time.Now().Format("15:04:05"))
	infraChan := make(chan asyncResult, 4)

	// Firewall
	go func() {
		log.Printf("[firewall] Starting at %s", time.Now().Format("15:04:05"))
		err := r.reconcileFirewall(ctx, publicIP)
		log.Printf("[firewall] Completed at %s", time.Now().Format("15:04:05"))
		infraChan <- asyncResult{name: "firewall", err: err}
	}()

	// Load Balancers
	go func() {
		log.Printf("[loadBalancers] Starting at %s", time.Now().Format("15:04:05"))
		err := r.reconcileLoadBalancers(ctx)
		log.Printf("[loadBalancers] Completed at %s", time.Now().Format("15:04:05"))
		infraChan <- asyncResult{name: "loadBalancers", err: err}
	}()

	// Placement Groups
	go func() {
		log.Printf("[placementGroups] Starting at %s", time.Now().Format("15:04:05"))
		err := r.reconcilePlacementGroups(ctx)
		log.Printf("[placementGroups] Completed at %s", time.Now().Format("15:04:05"))
		infraChan <- asyncResult{name: "placementGroups", err: err}
	}()

	// Floating IPs
	go func() {
		log.Printf("[floatingIPs] Starting at %s", time.Now().Format("15:04:05"))
		err := r.reconcileFloatingIPs(ctx)
		log.Printf("[floatingIPs] Completed at %s", time.Now().Format("15:04:05"))
		infraChan <- asyncResult{name: "floatingIPs", err: err}
	}()

	// Wait for all infrastructure components
	for i := 0; i < 4; i++ {
		result := <-infraChan
		if result.err != nil {
			return fmt.Errorf("failed to reconcile %s: %w", result.name, result.err)
		}
	}
	log.Printf("=== INFRASTRUCTURE SETUP COMPLETE at %s ===", time.Now().Format("15:04:05"))

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

		// Generate machine configs for each control plane node
		// For now, all control plane nodes get the same config
		// In the future, we could customize per-node if needed
		cpConfig, err := r.talosGenerator.GenerateControlPlaneConfig(nil) // SANs already set during reconcileControlPlane
		if err != nil {
			return fmt.Errorf("failed to generate control plane config: %w", err)
		}

		machineConfigs := make(map[string][]byte)
		for name := range cpIPs {
			machineConfigs[name] = cpConfig
		}

		if err := r.bootstrapper.Bootstrap(ctx, r.config.ClusterName, cpIPs, machineConfigs, clientCfg); err != nil {
			return fmt.Errorf("failed to bootstrap cluster: %w", err)
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

	// Parallelize worker pool provisioning
	if len(r.config.Workers) == 0 {
		log.Println("No worker pools configured")
		return nil
	}

	log.Printf("=== CREATING %d WORKER POOLS IN PARALLEL at %s ===", len(r.config.Workers), time.Now().Format("15:04:05"))

	type workerResult struct {
		poolName string
		err      error
	}

	resultChan := make(chan workerResult, len(r.config.Workers))

	for i, pool := range r.config.Workers {
		i, pool := i, pool // capture loop variables
		go func() {
			log.Printf("[workerPool:%s] Starting at %s", pool.Name, time.Now().Format("15:04:05"))
			// Placement Group (Managed inside reconcileNodePool for Workers due to sharding)
			// We pass nil here, and handle it inside reconcileNodePool based on pool config and index.
			_, err := r.reconcileNodePool(ctx, pool.Name, pool.Count, pool.ServerType, pool.Location, pool.Image, "worker", pool.Labels, string(workerConfig), nil, i)
			log.Printf("[workerPool:%s] Completed at %s", pool.Name, time.Now().Format("15:04:05"))
			resultChan <- workerResult{poolName: pool.Name, err: err}
		}()
	}

	// Wait for all worker pools to complete
	for i := 0; i < len(r.config.Workers); i++ {
		result := <-resultChan
		if result.err != nil {
			return fmt.Errorf("failed to reconcile worker pool %s: %w", result.poolName, result.err)
		}
	}

	log.Printf("=== SUCCESSFULLY CREATED ALL WORKER POOLS at %s ===", time.Now().Format("15:04:05"))
	return nil
}

// reconcileNodePool provisions a pool of servers in parallel.
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
	// Pre-calculate all server configurations
	type serverConfig struct {
		name      string
		privateIP string
		pgID      *int64
	}

	configs := make([]serverConfig, count)

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

		configs[j-1] = serverConfig{
			name:      serverName,
			privateIP: privateIP,
			pgID:      currentPGID,
		}
	}

	// Create all servers in parallel
	log.Printf("=== CREATING %d SERVERS IN PARALLEL for pool %s at %s ===", count, poolName, time.Now().Format("15:04:05"))
	type serverResult struct {
		name string
		ip   string
		err  error
	}

	resultChan := make(chan serverResult, count)

	for _, cfg := range configs {
		cfg := cfg // capture loop variable
		go func() {
			log.Printf("[server:%s] Starting at %s", cfg.name, time.Now().Format("15:04:05"))
			ip, err := r.ensureServer(ctx, cfg.name, serverType, location, image, role, poolName, extraLabels, userData, cfg.pgID, cfg.privateIP)
			log.Printf("[server:%s] Completed at %s", cfg.name, time.Now().Format("15:04:05"))
			resultChan <- serverResult{name: cfg.name, ip: ip, err: err}
		}()
	}

	// Collect results
	ips := make(map[string]string)
	for i := 0; i < count; i++ {
		result := <-resultChan
		if result.err != nil {
			return nil, fmt.Errorf("failed to create server %s: %w", result.name, result.err)
		}
		ips[result.name] = result.ip
	}

	log.Printf("=== SUCCESSFULLY CREATED %d SERVERS for pool %s at %s ===", count, poolName, time.Now().Format("15:04:05"))
	return ips, nil
}

// ensureAllImages pre-builds all required Talos images in parallel.
// This is called early in reconciliation to avoid sequential image building during server creation.
func (r *Reconciler) ensureAllImages(ctx context.Context) error {
	log.Println("Pre-building all required Talos images...")

	// Collect all unique server types from control plane and worker pools
	serverTypes := make(map[string]bool)

	// Control plane server types
	for _, pool := range r.config.ControlPlane.NodePools {
		if pool.Image == "" || pool.Image == "talos" {
			serverTypes[pool.ServerType] = true
		}
	}

	// Worker server types
	for _, pool := range r.config.Workers {
		if pool.Image == "" || pool.Image == "talos" {
			serverTypes[pool.ServerType] = true
		}
	}

	if len(serverTypes) == 0 {
		log.Println("No Talos images needed (all pools use custom images)")
		return nil
	}

	// Determine unique architectures needed
	architectures := make(map[string]bool)
	for serverType := range serverTypes {
		arch := "amd64"
		if len(serverType) >= 3 && serverType[:3] == "cax" {
			arch = "arm64"
		}
		architectures[arch] = true
	}

	log.Printf("Building images for architectures: %v", getKeys(architectures))

	// Get versions from config
	talosVersion := r.config.Talos.Version
	k8sVersion := r.config.Kubernetes.Version
	if talosVersion == "" {
		talosVersion = "v1.8.3"
	}
	if k8sVersion == "" {
		k8sVersion = "v1.31.0"
	}

	// Get location from first control plane node, or default to nbg1
	location := "nbg1"
	if len(r.config.ControlPlane.NodePools) > 0 && r.config.ControlPlane.NodePools[0].Location != "" {
		location = r.config.ControlPlane.NodePools[0].Location
	}

	// Build images in parallel
	type buildResult struct {
		arch string
		err  error
	}

	resultChan := make(chan buildResult, len(architectures))

	for arch := range architectures {
		arch := arch // capture loop variable
		go func() {
			labels := map[string]string{
				"os":            "talos",
				"talos-version": talosVersion,
				"k8s-version":   k8sVersion,
				"arch":          arch,
			}

			// Check if snapshot already exists
			snapshot, err := r.snapshotManager.GetSnapshotByLabels(ctx, labels)
			if err != nil {
				resultChan <- buildResult{arch: arch, err: fmt.Errorf("failed to check for existing snapshot: %w", err)}
				return
			}

			if snapshot != nil {
				log.Printf("Found existing Talos snapshot for %s: %s (ID: %d)", arch, snapshot.Description, snapshot.ID)
				resultChan <- buildResult{arch: arch, err: nil}
				return
			}

			// Build image
			log.Printf("Building Talos image for %s/%s/%s in location %s...", talosVersion, k8sVersion, arch, location)
			builder := r.createImageBuilder()
			if builder == nil {
				resultChan <- buildResult{arch: arch, err: fmt.Errorf("image builder not available")}
				return
			}

			snapshotID, err := builder.Build(ctx, talosVersion, k8sVersion, arch, location, labels)
			if err != nil {
				resultChan <- buildResult{arch: arch, err: fmt.Errorf("failed to build image: %w", err)}
				return
			}

			log.Printf("Successfully built Talos snapshot for %s: ID %s", arch, snapshotID)
			resultChan <- buildResult{arch: arch, err: nil}
		}()
	}

	// Wait for all builds to complete
	var errors []error
	for i := 0; i < len(architectures); i++ {
		result := <-resultChan
		if result.err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", result.arch, result.err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to build some images: %v", errors)
	}

	log.Println("All required Talos images are ready")
	return nil
}

// getKeys returns the keys of a map as a slice (helper function).
func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ensureImage ensures the required Talos image exists, building it if necessary.
func (r *Reconciler) ensureImage(ctx context.Context, serverType, location string) (string, error) {
	// Determine architecture from server type
	arch := "amd64"
	if len(serverType) >= 3 && serverType[:3] == "cax" {
		arch = "arm64"
	}

	// Get versions from config
	talosVersion := r.config.Talos.Version
	k8sVersion := r.config.Kubernetes.Version

	// Set defaults if not configured
	if talosVersion == "" {
		talosVersion = "v1.8.3"
	}
	if k8sVersion == "" {
		k8sVersion = "v1.31.0"
	}

	// Default location if not provided
	if location == "" {
		location = "nbg1"
	}

	// Check if snapshot already exists
	labels := map[string]string{
		"os":            "talos",
		"talos-version": talosVersion,
		"k8s-version":   k8sVersion,
		"arch":          arch,
	}

	snapshot, err := r.snapshotManager.GetSnapshotByLabels(ctx, labels)
	if err != nil {
		return "", fmt.Errorf("failed to check for existing snapshot: %w", err)
	}

	if snapshot != nil {
		snapshotID := fmt.Sprintf("%d", snapshot.ID)
		log.Printf("Found existing Talos snapshot: %s (ID: %s)", snapshot.Description, snapshotID)
		return snapshotID, nil
	}

	// Snapshot doesn't exist, build it
	log.Printf("Talos snapshot not found for %s/%s/%s, building in location %s...", talosVersion, k8sVersion, arch, location)

	// Import image builder
	builder := r.createImageBuilder()
	if builder == nil {
		return "", fmt.Errorf("image builder not available")
	}

	snapshotID, err := builder.Build(ctx, talosVersion, k8sVersion, arch, location, labels)
	if err != nil {
		return "", fmt.Errorf("failed to build Talos image: %w", err)
	}

	// Return the snapshot ID that was just created
	log.Printf("Successfully built Talos snapshot ID: %s", snapshotID)
	return snapshotID, nil
}

// createImageBuilder creates an image builder instance.
func (r *Reconciler) createImageBuilder() *image.Builder {
	// Pass nil for communicator factory - the builder will use its internal
	// SSH key generation and create its own SSH client with those keys
	return image.NewBuilder(r.infra, nil)
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

	// Image defaulting - if empty or "talos", ensure the versioned image exists
	if image == "" || image == "talos" {
		var err error
		image, err = r.ensureImage(ctx, serverType, location)
		if err != nil {
			return "", fmt.Errorf("failed to ensure Talos image: %w", err)
		}
		log.Printf("Using Talos image: %s", image)
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
