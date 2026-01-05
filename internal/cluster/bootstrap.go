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
	"net"
	"time"

	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
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
// waits for them to be ready, calls bootstrap on the first node, and creates the state marker.
func (b *Bootstrapper) Bootstrap(ctx context.Context, clusterName string, controlPlaneNodes map[string]string, machineConfigs map[string][]byte, clientConfigBytes []byte) error {
	markerName := fmt.Sprintf("%s-state", clusterName)

	// 1. Check for State Marker
	cert, err := b.hClient.GetCertificate(ctx, markerName)
	if err != nil {
		return fmt.Errorf("failed to check for state marker: %w", err)
	}
	if cert != nil {
		log.Printf("Cluster %s is already initialized (state marker found). Skipping bootstrap.", clusterName)
		return nil
	}

	log.Printf("Bootstrapping cluster %s with %d control plane nodes...", clusterName, len(controlPlaneNodes))

	// 2. Apply machine configurations to all control plane nodes
	log.Println("Applying machine configurations to control plane nodes...")
	for nodeName, nodeIP := range controlPlaneNodes {
		machineConfig, ok := machineConfigs[nodeName]
		if !ok {
			return fmt.Errorf("missing machine config for node %s", nodeName)
		}

		log.Printf("Applying config to node %s (%s)...", nodeName, nodeIP)
		if err := b.applyMachineConfig(ctx, nodeIP, machineConfig, clientConfigBytes); err != nil {
			return fmt.Errorf("failed to apply config to node %s: %w", nodeName, err)
		}
	}

	// 3. Wait for all nodes to reboot and become ready
	log.Println("Waiting for nodes to reboot and become ready...")
	for nodeName, nodeIP := range controlPlaneNodes {
		log.Printf("Waiting for node %s (%s) to be ready...", nodeName, nodeIP)
		if err := b.waitForNodeReady(ctx, nodeIP, clientConfigBytes); err != nil {
			return fmt.Errorf("node %s failed to become ready: %w", nodeName, err)
		}
		log.Printf("Node %s is ready", nodeName)
	}

	// 4. Initialize Talos Client for bootstrap
	cfg, err := config.FromString(string(clientConfigBytes))
	if err != nil {
		return fmt.Errorf("failed to parse client config: %w", err)
	}

	// Get first control plane IP for bootstrap
	var firstCPIP string
	for _, ip := range controlPlaneNodes {
		firstCPIP = ip
		break
	}

	// Create Talos Client Context
	clientCtx, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(firstCPIP), client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}))
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

	_, err = b.hClient.EnsureCertificate(ctx, markerName, dummyCert, dummyKey, labels)
	if err != nil {
		return fmt.Errorf("failed to create state marker: %w", err)
	}

	log.Println("Bootstrap complete!")
	return nil
}

// waitForPort waits for a TCP port to be open.
func (b *Bootstrapper) waitForPort(ctx context.Context, ip string, port int) error {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Timeout after 10 minutes
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", address, 2*time.Second)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
	}
}

// applyMachineConfig applies a machine configuration to a Talos node.
// For pre-installed Talos (from snapshots), this uses authenticated connections
// matching Terraform's talos_machine_configuration_apply behavior.
func (b *Bootstrapper) applyMachineConfig(ctx context.Context, nodeIP string, machineConfig []byte, clientConfigBytes []byte) error {
	// Wait for Talos API to be available
	if err := b.waitForPort(ctx, nodeIP, 50000); err != nil {
		return fmt.Errorf("failed to wait for Talos API: %w", err)
	}

	// Parse client config for authentication
	cfg, err := config.FromString(string(clientConfigBytes))
	if err != nil {
		return fmt.Errorf("failed to parse client config: %w", err)
	}

	// Create authenticated client (like Terraform does)
	// Pre-installed Talos nodes from snapshots expect authenticated connections
	clientCtx, err := client.New(ctx,
		client.WithConfig(cfg),
		client.WithEndpoints(nodeIP),
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer clientCtx.Close()

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
	if err := b.waitForPort(ctx, nodeIP, 50000); err != nil {
		return fmt.Errorf("failed to wait for node to come back: %w", err)
	}

	// Create authenticated client
	clientCtx, err := client.New(ctx,
		client.WithConfig(cfg),
		client.WithEndpoints(nodeIP),
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer clientCtx.Close()

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
