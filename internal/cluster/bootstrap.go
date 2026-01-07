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

	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/netutil"
	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
)

// Bootstrapper handles the cluster bootstrapping process.
type Bootstrapper struct {
	hClient hcloud_internal.InfrastructureManager
}

// NewBootstrapper creates a new Bootstrapper.
func NewBootstrapper(hClient hcloud_internal.InfrastructureManager) *Bootstrapper {
	return &Bootstrapper{
		hClient: hClient,
	}
}

// Bootstrap performs the bootstrap process.
// It checks for the state marker, applies machine configs to all control plane nodes,
// waits for them to be ready, calls bootstrap on the first node, creates the state marker,
// and retrieves the kubeconfig.
// Returns the kubeconfig bytes and any error encountered.
func (b *Bootstrapper) Bootstrap(ctx context.Context, clusterName string, controlPlaneNodes map[string]string, machineConfigs map[string][]byte, clientConfigBytes []byte) ([]byte, error) {
	markerName := fmt.Sprintf("%s-state", clusterName)

	// 1. Check for State Marker
	cert, err := b.hClient.GetCertificate(ctx, markerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check for state marker: %w", err)
	}
	if cert != nil {
		log.Printf("Cluster %s is already initialized (state marker found). Skipping bootstrap.", clusterName)
		// Cluster already bootstrapped, try to retrieve kubeconfig
		// If retrieval fails (e.g., in tests or if cluster isn't ready), return nil kubeconfig
		kubeconfig, err := b.retrieveKubeconfig(ctx, controlPlaneNodes, clientConfigBytes)
		if err != nil {
			log.Printf("Note: Could not retrieve kubeconfig from existing cluster: %v", err)
			return nil, nil
		}
		return kubeconfig, nil
	}

	log.Printf("Bootstrapping cluster %s with %d control plane nodes...", clusterName, len(controlPlaneNodes))

	// 2. Apply machine configurations to all control plane nodes
	log.Println("Applying machine configurations to control plane nodes...")
	for nodeName, nodeIP := range controlPlaneNodes {
		machineConfig, ok := machineConfigs[nodeName]
		if !ok {
			return nil, fmt.Errorf("missing machine config for node %s", nodeName)
		}

		log.Printf("Applying config to node %s (%s)...", nodeName, nodeIP)
		if err := b.applyMachineConfig(ctx, nodeIP, machineConfig, clientConfigBytes); err != nil {
			return nil, fmt.Errorf("failed to apply config to node %s: %w", nodeName, err)
		}
	}

	// 3. Wait for all nodes to reboot and become ready
	log.Println("Waiting for nodes to reboot and become ready...")
	for nodeName, nodeIP := range controlPlaneNodes {
		log.Printf("Waiting for node %s (%s) to be ready...", nodeName, nodeIP)
		if err := b.waitForNodeReady(ctx, nodeIP, clientConfigBytes); err != nil {
			return nil, fmt.Errorf("node %s failed to become ready: %w", nodeName, err)
		}
		log.Printf("Node %s is ready", nodeName)
	}

	// 4. Initialize Talos Client for bootstrap
	cfg, err := config.FromString(string(clientConfigBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse client config: %w", err)
	}

	// Get first control plane IP for bootstrap
	var firstCPIP string
	for _, ip := range controlPlaneNodes {
		firstCPIP = ip
		break
	}

	// Create Talos Client Context
	// Use config which includes proper TLS setup with client certificates for mTLS
	clientCtx, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(firstCPIP))
	if err != nil {
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() {
		_ = clientCtx.Close()
	}()

	// 5. Bootstrap the cluster
	log.Printf("Bootstrapping etcd on first control plane node %s...", firstCPIP)
	req := &machine.BootstrapRequest{}

	if err := clientCtx.Bootstrap(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to bootstrap: %w", err)
	}

	// 6. Create State Marker
	log.Println("Creating state marker...")
	labels := map[string]string{
		"cluster": clusterName,
		"state":   "initialized",
	}

	dummyCert, dummyKey, err := generateDummyCert()
	if err != nil {
		return nil, fmt.Errorf("failed to generate dummy cert for marker: %w", err)
	}

	_, err = b.hClient.EnsureCertificate(ctx, markerName, dummyCert, dummyKey, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to create state marker: %w", err)
	}

	log.Println("Bootstrap complete!")

	// 7. Retrieve Kubeconfig
	log.Println("Retrieving kubeconfig...")
	kubeconfig, err := b.retrieveKubeconfig(ctx, controlPlaneNodes, clientConfigBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	log.Println("Kubeconfig retrieved successfully!")
	return kubeconfig, nil
}

// applyMachineConfig applies a machine configuration to a Talos node.
// For pre-installed Talos (from snapshots), nodes boot into maintenance mode
// and require an insecure connection to apply the initial configuration.
func (b *Bootstrapper) applyMachineConfig(ctx context.Context, nodeIP string, machineConfig []byte, _ []byte) error {
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
func (b *Bootstrapper) waitForNodeReady(ctx context.Context, nodeIP string, clientConfigBytes []byte) error {
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
// It waits for the Kubernetes API to become available before retrieving.
func (b *Bootstrapper) retrieveKubeconfig(ctx context.Context, controlPlaneNodes map[string]string, clientConfigBytes []byte) ([]byte, error) {
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
// This should be called after worker nodes are provisioned but before they're expected to join the cluster.
func (b *Bootstrapper) ApplyWorkerConfigs(ctx context.Context, workerNodes map[string]string, workerConfig []byte, clientConfigBytes []byte) error {
	if len(workerNodes) == 0 {
		log.Println("No worker nodes to configure")
		return nil
	}

	log.Printf("Applying machine configurations to %d worker nodes...", len(workerNodes))

	for nodeName, nodeIP := range workerNodes {
		log.Printf("Applying config to worker node %s (%s)...", nodeName, nodeIP)
		if err := b.applyMachineConfig(ctx, nodeIP, workerConfig, clientConfigBytes); err != nil {
			return fmt.Errorf("failed to apply config to worker node %s: %w", nodeName, err)
		}
	}

	// Wait for all worker nodes to reboot and become ready
	log.Println("Waiting for worker nodes to reboot and become ready...")
	for nodeName, nodeIP := range workerNodes {
		log.Printf("Waiting for worker node %s (%s) to be ready...", nodeName, nodeIP)
		if err := b.waitForNodeReady(ctx, nodeIP, clientConfigBytes); err != nil {
			return fmt.Errorf("worker node %s failed to become ready: %w", nodeName, err)
		}
		log.Printf("Worker node %s is ready", nodeName)
	}

	log.Println("All worker nodes configured successfully")
	return nil
}
