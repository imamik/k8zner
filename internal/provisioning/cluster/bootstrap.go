// Package cluster provides cluster bootstrap and configuration functionality.
// This file manages the bootstrap process which initializes a new cluster by applying
// machine configs, waiting for nodes to be ready, and retrieving the kubeconfig.
package cluster

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/imamik/k8zner/internal/provisioning"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
)

const phase = "cluster"

// BootstrapCluster performs the bootstrap process for a new cluster.
// The main function orchestrates the steps - each helper does ONE thing.
func (p *Provisioner) BootstrapCluster(ctx *provisioning.Context) error {
	if err := p.ensureTalosConfigInState(ctx); err != nil {
		return err
	}

	// Already bootstrapped? Just retrieve kubeconfig
	if p.isAlreadyBootstrapped(ctx) {
		return p.tryRetrieveExistingKubeconfig(ctx)
	}

	// Bootstrap sequence
	ctx.Logger.Printf("[%s] Bootstrapping cluster %s with %d control plane nodes...",
		phase, ctx.Config.ClusterName, len(ctx.State.ControlPlaneIPs))

	if err := p.applyControlPlaneConfigs(ctx); err != nil {
		return err
	}
	if err := p.waitForControlPlaneReady(ctx); err != nil {
		return err
	}
	if err := p.bootstrapEtcd(ctx); err != nil {
		return err
	}
	if err := p.createStateMarker(ctx); err != nil {
		return err
	}

	// Apply worker configs after control plane is ready and etcd is bootstrapped
	if err := p.ApplyWorkerConfigs(ctx); err != nil {
		return err
	}

	return p.retrieveAndStoreKubeconfig(ctx)
}

// ensureTalosConfigInState ensures the Talos client config is stored in state.
func (p *Provisioner) ensureTalosConfigInState(ctx *provisioning.Context) error {
	if len(ctx.State.TalosConfig) > 0 {
		return nil
	}
	clientCfg, err := ctx.Talos.GetClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}
	ctx.State.TalosConfig = clientCfg
	return nil
}

// isAlreadyBootstrapped checks if the cluster state marker exists.
func (p *Provisioner) isAlreadyBootstrapped(ctx *provisioning.Context) bool {
	markerName := fmt.Sprintf("%s-state", ctx.Config.ClusterName)
	cert, err := ctx.Infra.GetCertificate(ctx, markerName)
	if err != nil {
		return false
	}
	if cert != nil {
		ctx.Logger.Printf("[%s] Cluster %s is already initialized (state marker found). Skipping bootstrap.",
			phase, ctx.Config.ClusterName)
		return true
	}
	return false
}

// tryRetrieveExistingKubeconfig attempts to retrieve kubeconfig from an existing cluster.
// It also handles scaling by configuring any new nodes that are still in maintenance mode.
func (p *Provisioner) tryRetrieveExistingKubeconfig(ctx *provisioning.Context) error {
	// Handle scaling: configure any new nodes that are in maintenance mode
	if err := p.configureNewNodes(ctx); err != nil {
		return fmt.Errorf("failed to configure new nodes during scale: %w", err)
	}

	kubeconfig, err := p.retrieveKubeconfig(ctx, ctx.State.ControlPlaneIPs, ctx.State.TalosConfig, ctx.Logger)
	if err != nil {
		ctx.Logger.Printf("[%s] Note: Could not retrieve kubeconfig from existing cluster: %v", phase, err)
		return nil
	}
	ctx.State.Kubeconfig = kubeconfig
	return nil
}

// configureNewNodes detects and configures any new nodes that are still in maintenance mode.
// This is called during scaling operations when the cluster is already bootstrapped.
func (p *Provisioner) configureNewNodes(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Checking %d control plane IPs and %d worker IPs for new nodes...",
		phase, len(ctx.State.ControlPlaneIPs), len(ctx.State.WorkerIPs))

	// Find new control plane nodes in maintenance mode
	newCPNodes := make(map[string]string)
	for nodeName, nodeIP := range ctx.State.ControlPlaneIPs {
		ctx.Logger.Printf("[%s] Checking control plane node %s (%s)...", phase, nodeName, nodeIP)
		if p.isNodeInMaintenanceMode(ctx, nodeIP) {
			ctx.Logger.Printf("[%s] Node %s is in maintenance mode (new node)", phase, nodeName)
			newCPNodes[nodeName] = nodeIP
		} else {
			ctx.Logger.Printf("[%s] Node %s is already configured", phase, nodeName)
		}
	}

	// Find new worker nodes in maintenance mode
	newWorkerNodes := make(map[string]string)
	for nodeName, nodeIP := range ctx.State.WorkerIPs {
		ctx.Logger.Printf("[%s] Checking worker node %s (%s)...", phase, nodeName, nodeIP)
		if p.isNodeInMaintenanceMode(ctx, nodeIP) {
			ctx.Logger.Printf("[%s] Node %s is in maintenance mode (new node)", phase, nodeName)
			newWorkerNodes[nodeName] = nodeIP
		} else {
			ctx.Logger.Printf("[%s] Node %s is already configured", phase, nodeName)
		}
	}

	if len(newCPNodes) == 0 && len(newWorkerNodes) == 0 {
		ctx.Logger.Printf("[%s] No new nodes detected, cluster is up to date", phase)
		return nil
	}

	// Configure new control plane nodes
	if len(newCPNodes) > 0 {
		ctx.Logger.Printf("[%s] Found %d new control plane nodes to configure", phase, len(newCPNodes))
		for nodeName, nodeIP := range newCPNodes {
			serverID := ctx.State.ControlPlaneServerIDs[nodeName]
			machineConfig, err := ctx.Talos.GenerateControlPlaneConfig(ctx.State.SANs, nodeName, serverID)
			if err != nil {
				return fmt.Errorf("failed to generate machine config for new CP node %s: %w", nodeName, err)
			}
			ctx.Logger.Printf("[%s] Applying config to new control plane node %s (%s)...", phase, nodeName, nodeIP)
			if err := p.applyMachineConfig(ctx, nodeIP, machineConfig); err != nil {
				return fmt.Errorf("failed to apply config to new CP node %s: %w", nodeName, err)
			}
		}
		// Wait for new control plane nodes to become ready
		for nodeName, nodeIP := range newCPNodes {
			ctx.Logger.Printf("[%s] Waiting for new control plane node %s (%s) to be ready...", phase, nodeName, nodeIP)
			if err := p.waitForNodeReady(ctx, nodeIP, ctx.State.TalosConfig, ctx.Logger); err != nil {
				return fmt.Errorf("new control plane node %s failed to become ready: %w", nodeName, err)
			}
			ctx.Logger.Printf("[%s] New control plane node %s is ready", phase, nodeName)
		}
	}

	// Configure new worker nodes
	if len(newWorkerNodes) > 0 {
		ctx.Logger.Printf("[%s] Found %d new worker nodes to configure", phase, len(newWorkerNodes))
		for nodeName, nodeIP := range newWorkerNodes {
			serverID := ctx.State.WorkerServerIDs[nodeName]
			nodeConfig, err := ctx.Talos.GenerateWorkerConfig(nodeName, serverID)
			if err != nil {
				return fmt.Errorf("failed to generate worker config for new node %s: %w", nodeName, err)
			}
			ctx.Logger.Printf("[%s] Applying config to new worker node %s (%s)...", phase, nodeName, nodeIP)
			if err := p.applyMachineConfig(ctx, nodeIP, nodeConfig); err != nil {
				return fmt.Errorf("failed to apply config to new worker node %s: %w", nodeName, err)
			}
		}
		// Wait for new worker nodes to become ready
		for nodeName, nodeIP := range newWorkerNodes {
			ctx.Logger.Printf("[%s] Waiting for new worker node %s (%s) to be ready...", phase, nodeName, nodeIP)
			if err := p.waitForNodeReady(ctx, nodeIP, ctx.State.TalosConfig, ctx.Logger); err != nil {
				return fmt.Errorf("new worker node %s failed to become ready: %w", nodeName, err)
			}
			ctx.Logger.Printf("[%s] New worker node %s is ready", phase, nodeName)
		}
	}

	ctx.Logger.Printf("[%s] Successfully configured %d new control plane nodes and %d new worker nodes",
		phase, len(newCPNodes), len(newWorkerNodes))
	return nil
}

// isNodeInMaintenanceMode checks if a node is in maintenance mode (unconfigured).
// A node in maintenance mode will accept an insecure connection but won't respond
// to authenticated requests with the cluster's Talos config.
func (p *Provisioner) isNodeInMaintenanceMode(ctx *provisioning.Context, nodeIP string) bool {
	// First check if the port is reachable - wait for new nodes to boot
	// Talos boot from snapshot typically takes 30-60 seconds
	portWaitTimeout := ctx.Timeouts.PortWait
	ctx.Logger.Printf("[%s] Waiting for Talos API on %s:50000...", phase, nodeIP)
	portCtx, cancel := context.WithTimeout(ctx, portWaitTimeout)
	defer cancel()
	if err := waitForPort(portCtx, nodeIP, 50000, portWaitTimeout, ctx.Timeouts.PortPoll, ctx.Timeouts.DialTimeout); err != nil {
		// Port not reachable - node might be offline or failed
		ctx.Logger.Printf("[%s] Node %s port 50000 not reachable, skipping", phase, nodeIP)
		return false
	}
	ctx.Logger.Printf("[%s] Node %s port 50000 is reachable", phase, nodeIP)

	// Try authenticated connection first
	if len(ctx.State.TalosConfig) == 0 {
		ctx.Logger.Printf("[%s] Warning: TalosConfig is empty, cannot check auth", phase)
		return false
	}

	cfg, err := config.FromString(string(ctx.State.TalosConfig))
	if err != nil {
		ctx.Logger.Printf("[%s] Warning: Failed to parse TalosConfig: %v", phase, err)
		return false
	}

	checkCtx, checkCancel := context.WithTimeout(ctx, 10*time.Second)
	defer checkCancel()

	authClient, err := client.New(checkCtx, client.WithConfig(cfg), client.WithEndpoints(nodeIP))
	if err != nil {
		ctx.Logger.Printf("[%s] Cannot create auth client for %s: %v - assuming maintenance mode", phase, nodeIP, err)
		return true // Can't create auth client, assume maintenance mode
	}
	defer func() { _ = authClient.Close() }()

	// Try to get version with authenticated client
	_, err = authClient.Version(checkCtx)
	if err == nil {
		// Authenticated connection works, node is already configured
		ctx.Logger.Printf("[%s] Node %s: authenticated connection succeeded - already configured", phase, nodeIP)
		return false
	}
	ctx.Logger.Printf("[%s] Node %s: authenticated connection failed: %v - trying insecure", phase, nodeIP, err)

	// Authenticated connection failed - could be maintenance mode or other issue
	// Try insecure connection to confirm it's maintenance mode
	insecureCtx, insecureCancel := context.WithTimeout(ctx, 10*time.Second)
	defer insecureCancel()

	insecureClient, err := client.New(insecureCtx,
		client.WithEndpoints(nodeIP),
		//nolint:gosec // InsecureSkipVerify is required to detect Talos maintenance mode
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	if err != nil {
		ctx.Logger.Printf("[%s] Cannot create insecure client for %s: %v", phase, nodeIP, err)
		return false
	}
	defer func() { _ = insecureClient.Close() }()

	_, err = insecureClient.Version(insecureCtx)
	if err == nil {
		// Insecure connection works but authenticated doesn't = maintenance mode
		ctx.Logger.Printf("[%s] Node %s is in maintenance mode (insecure works, auth fails)", phase, nodeIP)
		return true
	}

	// Check if the error indicates maintenance mode
	// In Talos maintenance mode, the Version API returns "API is not implemented in maintenance mode"
	// This actually confirms the node IS in maintenance mode!
	errStr := err.Error()
	if strings.Contains(errStr, "not implemented in maintenance mode") ||
		strings.Contains(errStr, "maintenance mode") {
		ctx.Logger.Printf("[%s] Node %s is in maintenance mode (detected via error message)", phase, nodeIP)
		return true
	}

	ctx.Logger.Printf("[%s] Node %s: both auth and insecure failed: %v", phase, nodeIP, err)
	return false
}

// applyControlPlaneConfigs applies machine configs to all control plane nodes.
func (p *Provisioner) applyControlPlaneConfigs(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Applying machine configurations to control plane nodes...", phase)
	for nodeName, nodeIP := range ctx.State.ControlPlaneIPs {
		serverID := ctx.State.ControlPlaneServerIDs[nodeName]
		machineConfig, err := ctx.Talos.GenerateControlPlaneConfig(ctx.State.SANs, nodeName, serverID)
		if err != nil {
			return fmt.Errorf("failed to generate machine config for node %s: %w", nodeName, err)
		}
		ctx.Logger.Printf("[%s] Applying config to node %s (%s)...", phase, nodeName, nodeIP)
		if err := p.applyMachineConfig(ctx, nodeIP, machineConfig); err != nil {
			return fmt.Errorf("failed to apply config to node %s: %w", nodeName, err)
		}
	}
	return nil
}

// waitForControlPlaneReady waits for all control plane nodes to reboot and become ready.
func (p *Provisioner) waitForControlPlaneReady(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Waiting for nodes to reboot and become ready...", phase)
	for nodeName, nodeIP := range ctx.State.ControlPlaneIPs {
		ctx.Logger.Printf("[%s] Waiting for node %s (%s) to be ready...", phase, nodeName, nodeIP)
		if err := p.waitForNodeReady(ctx, nodeIP, ctx.State.TalosConfig, ctx.Logger); err != nil {
			return fmt.Errorf("node %s failed to become ready: %w", nodeName, err)
		}
		ctx.Logger.Printf("[%s] Node %s is ready", phase, nodeName)
	}
	return nil
}

// bootstrapEtcd initializes etcd on the first control plane node.
func (p *Provisioner) bootstrapEtcd(ctx *provisioning.Context) error {
	firstCPIP := p.getFirstControlPlaneIP(ctx)

	cfg, err := config.FromString(string(ctx.State.TalosConfig))
	if err != nil {
		return fmt.Errorf("failed to parse talos config: %w", err)
	}
	clientCtx, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(firstCPIP))
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = clientCtx.Close() }()

	ctx.Logger.Printf("[%s] Bootstrapping etcd on first control plane node %s...", phase, firstCPIP)
	if err := clientCtx.Bootstrap(ctx, &machine.BootstrapRequest{}); err != nil {
		return fmt.Errorf("failed to bootstrap: %w", err)
	}
	return nil
}

// createStateMarker creates a certificate marker indicating the cluster is initialized.
func (p *Provisioner) createStateMarker(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Creating state marker...", phase)

	markerName := fmt.Sprintf("%s-state", ctx.Config.ClusterName)
	labels := map[string]string{
		"cluster": ctx.Config.ClusterName,
		"state":   "initialized",
	}

	dummyCert, dummyKey, err := generateDummyCert()
	if err != nil {
		return fmt.Errorf("failed to generate dummy cert for marker: %w", err)
	}

	if _, err := ctx.Infra.EnsureCertificate(ctx, markerName, dummyCert, dummyKey, labels); err != nil {
		return fmt.Errorf("failed to create state marker: %w", err)
	}

	ctx.Logger.Printf("[%s] Bootstrap complete!", phase)
	return nil
}

// retrieveAndStoreKubeconfig retrieves the kubeconfig and stores it in state.
func (p *Provisioner) retrieveAndStoreKubeconfig(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Retrieving kubeconfig...", phase)
	kubeconfig, err := p.retrieveKubeconfig(ctx, ctx.State.ControlPlaneIPs, ctx.State.TalosConfig, ctx.Logger)
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}
	ctx.State.Kubeconfig = kubeconfig
	return nil
}

// getFirstControlPlaneIP returns the IP of any control plane node (used for bootstrap).
func (p *Provisioner) getFirstControlPlaneIP(ctx *provisioning.Context) string {
	for _, ip := range ctx.State.ControlPlaneIPs {
		return ip
	}
	return ""
}

// applyMachineConfig applies a machine configuration to a Talos node.
func (p *Provisioner) applyMachineConfig(ctx *provisioning.Context, nodeIP string, machineConfig []byte) error {
	// Wait for Talos API to be available
	if err := waitForPort(ctx, nodeIP, 50000, ctx.Timeouts.TalosAPI, ctx.Timeouts.PortPoll, ctx.Timeouts.DialTimeout); err != nil {
		return fmt.Errorf("failed to wait for Talos API: %w", err)
	}

	// Create insecure client for maintenance mode
	// Fresh Talos nodes from snapshots boot into maintenance mode and don't have
	// credentials yet, so we must use an insecure connection to apply the initial config.
	clientCtx, err := client.New(ctx,
		client.WithEndpoints(nodeIP),
		//nolint:gosec // InsecureSkipVerify is required for Talos maintenance mode
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = clientCtx.Close() }()

	// Apply the configuration
	// Use REBOOT mode for pre-installed Talos (from Packer snapshots)
	// This matches Terraform's behavior and avoids confusion with AUTO mode
	// which might trigger installation logic on pre-installed systems
	applyReq := &machine.ApplyConfigurationRequest{
		Data: machineConfig,
		Mode: machine.ApplyConfigurationRequest_REBOOT,
	}

	_, err = clientCtx.ApplyConfiguration(ctx, applyReq)
	if err != nil {
		return fmt.Errorf("failed to apply configuration: %w", err)
	}

	return nil
}

// waitForNodeReady waits for a node to reboot and become ready after applying configuration.
func (p *Provisioner) waitForNodeReady(ctx *provisioning.Context, nodeIP string, clientConfigBytes []byte, logger provisioning.Logger) error {
	// Parse client config
	cfg, err := config.FromString(string(clientConfigBytes))
	if err != nil {
		return fmt.Errorf("failed to parse client config: %w", err)
	}

	// Wait for port to become unavailable (node rebooting)
	logger.Printf("[%s] Waiting for node %s to begin reboot...", phase, nodeIP)
	time.Sleep(defaultRebootInitialWait)

	// Wait for port to come back up
	logger.Printf("[%s] Waiting for node %s to come back online...", phase, nodeIP)
	if err := waitForPort(ctx, nodeIP, 50000, ctx.Timeouts.TalosAPI, ctx.Timeouts.PortPoll, ctx.Timeouts.DialTimeout); err != nil {
		return fmt.Errorf("failed to wait for node to come back: %w", err)
	}

	// Create authenticated client
	// Use config from talosconfig which includes client certificates for mTLS
	clientCtx, err := client.New(ctx,
		client.WithConfig(cfg),
		client.WithEndpoints(nodeIP),
	)
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = clientCtx.Close() }()

	// Wait for node to report ready status
	ticker := time.NewTicker(ctx.Timeouts.NodeReadyPoll)
	defer ticker.Stop()

	timeout := time.After(ctx.Timeouts.NodeReady)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for node to be ready")
		case <-ticker.C:
			// Try to get version - if this succeeds, node is ready
			_, err := clientCtx.Version(ctx)
			if err == nil {
				return nil
			}
			logger.Printf("[%s] Node %s not yet ready, waiting... (error: %v)", phase, nodeIP, err)
		}
	}
}

// retrieveKubeconfig retrieves the kubeconfig from the cluster after bootstrap.
func (p *Provisioner) retrieveKubeconfig(ctx *provisioning.Context, controlPlaneNodes map[string]string, clientConfigBytes []byte, logger provisioning.Logger) ([]byte, error) {
	// Parse client config
	cfg, err := config.FromString(string(clientConfigBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse client config: %w", err)
	}

	// Get first control plane IP
	var firstCPIP string
	for _, ip := range controlPlaneNodes {
		firstCPIP = ip
		break
	}

	// Create Talos Client
	clientCtx, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(firstCPIP))
	if err != nil {
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = clientCtx.Close() }()

	// Wait for Kubernetes API to be ready
	logger.Printf("[%s] Waiting for Kubernetes API to become ready...", phase)
	ticker := time.NewTicker(ctx.Timeouts.NodeReadyPoll)
	defer ticker.Stop()

	timeout := time.After(ctx.Timeouts.Kubeconfig)

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for Kubernetes API to be ready")
		case <-ticker.C:
			// Try to retrieve kubeconfig - if this succeeds, API is ready
			kubeconfigBytes, err := clientCtx.Kubeconfig(ctx)
			if err == nil && len(kubeconfigBytes) > 0 {
				logger.Printf("[%s] Kubernetes API is ready!", phase)
				return kubeconfigBytes, nil
			}
			logger.Printf("[%s] Kubernetes API not yet ready, waiting... (error: %v)", phase, err)
		}
	}
}

// generateDummyCert generates a dummy self-signed certificate for the state marker.
func generateDummyCert() (string, string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"HCloud K8s State Marker"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour * 10), // 10 years
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})

	return string(certPEM), string(keyPEM), nil
}

// ApplyWorkerConfigs applies Talos machine configurations to worker nodes.
func (p *Provisioner) ApplyWorkerConfigs(ctx *provisioning.Context) error {
	workerNodes := ctx.State.WorkerIPs
	if len(workerNodes) == 0 {
		ctx.Logger.Printf("[%s] No worker nodes to configure", phase)
		return nil
	}

	ctx.Logger.Printf("[%s] Applying machine configurations to %d worker nodes...", phase, len(workerNodes))

	for nodeName, nodeIP := range workerNodes {
		serverID := ctx.State.WorkerServerIDs[nodeName]
		nodeConfig, err := ctx.Talos.GenerateWorkerConfig(nodeName, serverID)
		if err != nil {
			return fmt.Errorf("failed to generate worker config for %s: %w", nodeName, err)
		}
		ctx.Logger.Printf("[%s] Applying config to worker node %s (%s)...", phase, nodeName, nodeIP)
		if err := p.applyMachineConfig(ctx, nodeIP, nodeConfig); err != nil {
			return fmt.Errorf("failed to apply config to worker node %s: %w", nodeName, err)
		}
	}

	// Wait for all worker nodes to reboot and become ready
	ctx.Logger.Printf("[%s] Waiting for worker nodes to reboot and become ready...", phase)
	for nodeName, nodeIP := range workerNodes {
		ctx.Logger.Printf("[%s] Waiting for worker node %s (%s) to be ready...", phase, nodeName, nodeIP)
		if err := p.waitForNodeReady(ctx, nodeIP, ctx.State.TalosConfig, ctx.Logger); err != nil {
			return fmt.Errorf("worker node %s failed to become ready: %w", nodeName, err)
		}
		ctx.Logger.Printf("[%s] Worker node %s is ready", phase, nodeName)
	}

	ctx.Logger.Printf("[%s] All worker nodes configured successfully", phase)
	return nil
}
