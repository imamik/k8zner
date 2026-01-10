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
	"time"

	"hcloud-k8s/internal/provisioning"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
)

const phase = "cluster"

// BootstrapCluster performs the bootstrap process.
// It checks for the state marker, applies machine configs to all control plane nodes,
// waits for them to be ready, calls bootstrap on the first node, creates the state marker,
// and retrieves the kubeconfig.
func (p *Provisioner) BootstrapCluster(ctx *provisioning.Context) error {
	clusterName := ctx.Config.ClusterName
	controlPlaneNodes := ctx.State.ControlPlaneIPs

	// Ensure we have the client config in state
	if len(ctx.State.TalosConfig) == 0 {
		clientCfg, err := ctx.Talos.GetClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get client config: %w", err)
		}
		ctx.State.TalosConfig = clientCfg
	}

	markerName := fmt.Sprintf("%s-state", clusterName)

	// 1. Check for State Marker
	cert, err := ctx.Infra.GetCertificate(ctx, markerName)
	if err != nil {
		return fmt.Errorf("failed to check for state marker: %w", err)
	}
	if cert != nil {
		ctx.Logger.Printf("[%s] Cluster %s is already initialized (state marker found). Skipping bootstrap.", phase, clusterName)
		// Cluster already bootstrapped, try to retrieve kubeconfig
		kubeconfig, err := p.retrieveKubeconfig(ctx, controlPlaneNodes, ctx.State.TalosConfig, ctx.Logger)
		if err != nil {
			ctx.Logger.Printf("[%s] Note: Could not retrieve kubeconfig from existing cluster: %v", phase, err)
			return nil
		}
		ctx.State.Kubeconfig = kubeconfig
		return nil
	}

	ctx.Logger.Printf("[%s] Bootstrapping cluster %s with %d control plane nodes...", phase, clusterName, len(controlPlaneNodes))

	// 2. Apply machine configurations to all control plane nodes
	ctx.Logger.Printf("[%s] Applying machine configurations to control plane nodes...", phase)
	for nodeName, nodeIP := range controlPlaneNodes {
		// Generate config for node
		machineConfig, err := ctx.Talos.GenerateControlPlaneConfig(ctx.State.SANs, nodeName)
		if err != nil {
			return fmt.Errorf("failed to generate machine config for node %s: %w", nodeName, err)
		}

		ctx.Logger.Printf("[%s] Applying config to node %s (%s)...", phase, nodeName, nodeIP)
		if err := p.applyMachineConfig(ctx, nodeIP, machineConfig, ctx.State.TalosConfig); err != nil {
			return fmt.Errorf("failed to apply config to node %s: %w", nodeName, err)
		}
	}

	// 3. Wait for all nodes to reboot and become ready
	ctx.Logger.Printf("[%s] Waiting for nodes to reboot and become ready...", phase)
	for nodeName, nodeIP := range controlPlaneNodes {
		ctx.Logger.Printf("[%s] Waiting for node %s (%s) to be ready...", phase, nodeName, nodeIP)
		if err := p.waitForNodeReady(ctx, nodeIP, ctx.State.TalosConfig, ctx.Logger); err != nil {
			return fmt.Errorf("node %s failed to become ready: %w", nodeName, err)
		}
		ctx.Logger.Printf("[%s] Node %s is ready", phase, nodeName)
	}

	// Initialize Talos Client for bootstrap
	clientConfigBytes := ctx.State.TalosConfig

	// Get first control plane IP for bootstrap
	var firstCPIP string
	for _, ip := range controlPlaneNodes {
		firstCPIP = ip
		break
	}

	// Create Talos Client Context
	cfg, _ := config.FromString(string(clientConfigBytes))
	clientCtx, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(firstCPIP))
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() {
		_ = clientCtx.Close()
	}()

	// 5. Bootstrap the cluster
	ctx.Logger.Printf("[%s] Bootstrapping etcd on first control plane node %s...", phase, firstCPIP)
	req := &machine.BootstrapRequest{}

	if err := clientCtx.Bootstrap(ctx, req); err != nil {
		return fmt.Errorf("failed to bootstrap: %w", err)
	}

	// 6. Create State Marker
	ctx.Logger.Printf("[%s] Creating state marker...", phase)
	labels := map[string]string{
		"cluster": clusterName,
		"state":   "initialized",
	}

	dummyCert, dummyKey, err := generateDummyCert()
	if err != nil {
		return fmt.Errorf("failed to generate dummy cert for marker: %w", err)
	}

	_, err = ctx.Infra.EnsureCertificate(ctx, markerName, dummyCert, dummyKey, labels)
	if err != nil {
		return fmt.Errorf("failed to create state marker: %w", err)
	}

	ctx.Logger.Printf("[%s] Bootstrap complete!", phase)

	// 7. Retrieve Kubeconfig
	ctx.Logger.Printf("[%s] Retrieving kubeconfig...", phase)
	kubeconfig, err := p.retrieveKubeconfig(ctx, controlPlaneNodes, clientConfigBytes, ctx.Logger)
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	ctx.State.Kubeconfig = kubeconfig
	return nil
}

// applyMachineConfig applies a machine configuration to a Talos node.
func (p *Provisioner) applyMachineConfig(ctx context.Context, nodeIP string, machineConfig []byte, _ []byte) error {
	// Wait for Talos API to be available
	if err := waitForPort(ctx, nodeIP, 50000, talosAPIWaitTimeout); err != nil {
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
func (p *Provisioner) waitForNodeReady(ctx context.Context, nodeIP string, clientConfigBytes []byte, logger provisioning.Logger) error {
	// Parse client config
	cfg, err := config.FromString(string(clientConfigBytes))
	if err != nil {
		return fmt.Errorf("failed to parse client config: %w", err)
	}

	// Wait for port to become unavailable (node rebooting)
	logger.Printf("[%s] Waiting for node %s to begin reboot...", phase, nodeIP)
	time.Sleep(10 * time.Second)

	// Wait for port to come back up
	logger.Printf("[%s] Waiting for node %s to come back online...", phase, nodeIP)
	if err := waitForPort(ctx, nodeIP, 50000, talosAPIWaitTimeout); err != nil {
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
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeout := time.After(10 * time.Minute)

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
func (p *Provisioner) retrieveKubeconfig(ctx context.Context, controlPlaneNodes map[string]string, clientConfigBytes []byte, logger provisioning.Logger) ([]byte, error) {
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
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeout := time.After(15 * time.Minute)

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
		nodeConfig, err := ctx.Talos.GenerateWorkerConfig(nodeName)
		if err != nil {
			return fmt.Errorf("failed to generate worker config for %s: %w", nodeName, err)
		}
		ctx.Logger.Printf("[%s] Applying config to worker node %s (%s)...", phase, nodeName, nodeIP)
		if err := p.applyMachineConfig(ctx, nodeIP, nodeConfig, ctx.State.TalosConfig); err != nil {
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
