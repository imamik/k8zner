package cluster

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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
// It checks for the state marker, waits for the node, calls bootstrap, and saves the kubeconfig.
func (b *Bootstrapper) Bootstrap(ctx context.Context, clusterName string, firstControlPlaneIP string, clientConfigBytes []byte) error {
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

	log.Printf("Bootstrapping cluster %s...", clusterName)

	// 2. Initialize Talos Client
	cfg, err := config.FromString(string(clientConfigBytes))
	if err != nil {
		return fmt.Errorf("failed to parse client config: %w", err)
	}

	// We need to wait for the node to be ready (TCP 50000).
	if err := b.waitForPort(ctx, firstControlPlaneIP, 50000); err != nil {
		return fmt.Errorf("failed to connect to node %s: %w", firstControlPlaneIP, err)
	}

	// Create Talos Client Context
	clientCtx, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(firstControlPlaneIP))
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() {
		_ = clientCtx.Close()
	}()

	// 3. Bootstrap
	log.Println("Sending bootstrap command...")
	// BootstrapRequest in newer Talos versions (machinery v1.5+) doesn't need Node address in the request struct itself,
	// as it's handled by the client connection context.
	req := &machine.BootstrapRequest{}

	if err := clientCtx.Bootstrap(ctx, req); err != nil {
		return fmt.Errorf("failed to bootstrap: %w", err)
	}

	// 4. Create State Marker
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
