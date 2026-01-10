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
	"log"
	"math/big"
	"time"

	"hcloud-k8s/internal/provisioning"
	"hcloud-k8s/internal/util/netutil"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
)

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
		log.Printf("Cluster %s is already initialized (state marker found). Skipping bootstrap.", clusterName)
		// Cluster already bootstrapped, try to retrieve kubeconfig
		kubeconfig, err := p.retrieveKubeconfig(ctx, controlPlaneNodes, ctx.State.TalosConfig)
		if err != nil {
			log.Printf("Note: Could not retrieve kubeconfig from existing cluster: %v", err)
			return nil
		}
		ctx.State.Kubeconfig = kubeconfig
		return nil
	}

	log.Printf("Bootstrapping cluster %s with %d control plane nodes...", clusterName, len(controlPlaneNodes))

	// 2. Apply machine configurations to all control plane nodes
	log.Println("Applying machine configurations to control plane nodes...")
	for nodeName, nodeIP := range controlPlaneNodes {
		// Generate config for node
		machineConfig, err := ctx.Talos.GenerateControlPlaneConfig(ctx.State.SANs, nodeName)
		if err != nil {
			return fmt.Errorf("failed to generate machine config for node %s: %w", nodeName, err)
		}

		log.Printf("Applying config to node %s (%s)...", nodeName, nodeIP)
		if err := p.applyMachineConfig(ctx, nodeIP, machineConfig, ctx.State.TalosConfig); err != nil {
			return fmt.Errorf("failed to apply config to node %s: %w", nodeName, err)
		}
	}

	// 3. Wait for all nodes to reboot and become ready
	log.Println("Waiting for nodes to reboot and become ready...")
	for nodeName, nodeIP := range controlPlaneNodes {
		log.Printf("Waiting for node %s (%s) to be ready...", nodeName, nodeIP)
		if err := p.waitForNodeReady(ctx, nodeIP, ctx.State.TalosConfig); err != nil {
			return fmt.Errorf("node %s failed to become ready: %w", nodeName, err)
		}
		log.Printf("Node %s is ready", nodeName)
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
	log.Printf("Bootstrapping etcd on first control plane node %s...", firstCPIP)
	req := &machine.BootstrapRequest{}

	if err := clientCtx.Bootstrap(ctx, req); err != nil {
		return fmt.Errorf("failed to bootstrap: %w", err)
	}

	// 6. Create State Marker
	log.Println("Creating state marker...")
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

	log.Println("Bootstrap complete!")

	// 7. Retrieve Kubeconfig
	log.Println("Retrieving kubeconfig...")
	kubeconfig, err := p.retrieveKubeconfig(ctx, controlPlaneNodes, clientConfigBytes)
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	ctx.State.Kubeconfig = kubeconfig
	return nil
}

// applyMachineConfig applies a machine configuration to a Talos node.
func (p *Provisioner) applyMachineConfig(ctx context.Context, nodeIP string, machineConfig []byte, _ []byte) error {
	// Wait for Talos API to be available
	if err := netutil.WaitForPort(ctx, nodeIP, 50000, netutil.TalosAPIWaitTimeout); err != nil {
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
func (p *Provisioner) waitForNodeReady(ctx context.Context, nodeIP string, clientConfigBytes []byte) error {
	// Parse client config
	cfg, err := config.FromString(string(clientConfigBytes))
	if err != nil {
		return fmt.Errorf("failed to parse client config: %w", err)
	}

	// Wait for port to become unavailable (node rebooting)
	log.Printf("Waiting for node %s to begin reboot...", nodeIP)
	time.Sleep(10 * time.Second)

	// Wait for port to come back up
	log.Printf("Waiting for node %s to come back online...", nodeIP)
	if err := netutil.WaitForPort(ctx, nodeIP, 50000, netutil.TalosAPIWaitTimeout); err != nil {
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
			log.Printf("Node %s not yet ready, waiting... (error: %v)", nodeIP, err)
		}
	}
}

// retrieveKubeconfig retrieves the kubeconfig from the cluster after bootstrap.
func (p *Provisioner) retrieveKubeconfig(ctx context.Context, controlPlaneNodes map[string]string, clientConfigBytes []byte) ([]byte, error) {
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
	log.Println("Waiting for Kubernetes API to become ready...")
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
				log.Println("Kubernetes API is ready!")
				return kubeconfigBytes, nil
			}
			log.Printf("Kubernetes API not yet ready, waiting... (error: %v)", err)
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
		log.Println("No worker nodes to configure")
		return nil
	}

	log.Printf("Applying machine configurations to %d worker nodes...", len(workerNodes))

	for nodeName, nodeIP := range workerNodes {
		nodeConfig, err := ctx.Talos.GenerateWorkerConfig(nodeName)
		if err != nil {
			return fmt.Errorf("failed to generate worker config for %s: %w", nodeName, err)
		}
		log.Printf("Applying config to worker node %s (%s)...", nodeName, nodeIP)
		if err := p.applyMachineConfig(ctx, nodeIP, nodeConfig, ctx.State.TalosConfig); err != nil {
			return fmt.Errorf("failed to apply config to worker node %s: %w", nodeName, err)
		}
	}

	// Wait for all worker nodes to reboot and become ready
	log.Println("Waiting for worker nodes to reboot and become ready...")
	for nodeName, nodeIP := range workerNodes {
		log.Printf("Waiting for worker node %s (%s) to be ready...", nodeName, nodeIP)
		if err := p.waitForNodeReady(ctx, nodeIP, ctx.State.TalosConfig); err != nil {
			return fmt.Errorf("worker node %s failed to become ready: %w", nodeName, err)
		}
		log.Printf("Worker node %s is ready", nodeName)
	}

	log.Println("All worker nodes configured successfully")
	return nil
}
