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

// configureNewNodes detects and configures any new nodes that are still in maintenance mode.
// This is called during scaling operations when the cluster is already bootstrapped.
func configureNewNodes(ctx *provisioning.Context) error {
	ctx.Observer.Printf("[%s] Checking %d control plane IPs and %d worker IPs for new nodes...",
		phase, len(ctx.State.ControlPlaneIPs), len(ctx.State.WorkerIPs))

	newCPNodes := detectMaintenanceModeNodes(ctx, ctx.State.ControlPlaneIPs, "control plane")
	newWorkerNodes := detectMaintenanceModeNodes(ctx, ctx.State.WorkerIPs, "worker")

	if len(newCPNodes) == 0 && len(newWorkerNodes) == 0 {
		ctx.Observer.Printf("[%s] No new nodes detected, cluster is up to date", phase)
		return nil
	}

	if err := configureAndWaitForNewCPs(ctx, newCPNodes); err != nil {
		return err
	}
	if err := configureAndWaitForNewWorkers(ctx, newWorkerNodes); err != nil {
		return err
	}

	ctx.Observer.Printf("[%s] Successfully configured %d new control plane nodes and %d new worker nodes",
		phase, len(newCPNodes), len(newWorkerNodes))
	return nil
}

// detectMaintenanceModeNodes checks each node IP and returns those in maintenance mode.
func detectMaintenanceModeNodes(ctx *provisioning.Context, nodeIPs map[string]string, role string) map[string]string {
	newNodes := make(map[string]string)
	for nodeName, nodeIP := range nodeIPs {
		ctx.Observer.Printf("[%s] Checking %s node %s (%s)...", phase, role, nodeName, nodeIP)
		if isNodeInMaintenanceMode(ctx, nodeIP) {
			ctx.Observer.Printf("[%s] Node %s is in maintenance mode (new node)", phase, nodeName)
			newNodes[nodeName] = nodeIP
		} else {
			ctx.Observer.Printf("[%s] Node %s is already configured", phase, nodeName)
		}
	}
	return newNodes
}

// configureAndWaitForNewCPs applies configs and waits for new control plane nodes.
func configureAndWaitForNewCPs(ctx *provisioning.Context, newCPNodes map[string]string) error {
	if len(newCPNodes) == 0 {
		return nil
	}

	ctx.Observer.Printf("[%s] Found %d new control plane nodes to configure", phase, len(newCPNodes))
	for nodeName, nodeIP := range newCPNodes {
		serverID := ctx.State.ControlPlaneServerIDs[nodeName]
		machineConfig, err := ctx.Talos.GenerateControlPlaneConfig(ctx.State.SANs, nodeName, serverID)
		if err != nil {
			return fmt.Errorf("failed to generate machine config for new CP node %s: %w", nodeName, err)
		}
		ctx.Observer.Printf("[%s] Applying config to new control plane node %s (%s)...", phase, nodeName, nodeIP)
		if err := applyMachineConfig(ctx, nodeIP, machineConfig); err != nil {
			return fmt.Errorf("failed to apply config to new CP node %s: %w", nodeName, err)
		}
	}

	for nodeName, nodeIP := range newCPNodes {
		ctx.Observer.Printf("[%s] Waiting for new control plane node %s (%s) to be ready...", phase, nodeName, nodeIP)
		if err := waitForNodeReady(ctx, nodeIP, ctx.State.TalosConfig, ctx.Observer); err != nil {
			return fmt.Errorf("new control plane node %s failed to become ready: %w", nodeName, err)
		}
		ctx.Observer.Printf("[%s] New control plane node %s is ready", phase, nodeName)
	}
	return nil
}

// configureAndWaitForNewWorkers applies configs and waits for new worker nodes.
func configureAndWaitForNewWorkers(ctx *provisioning.Context, newWorkerNodes map[string]string) error {
	if len(newWorkerNodes) == 0 {
		return nil
	}

	ctx.Observer.Printf("[%s] Found %d new worker nodes to configure", phase, len(newWorkerNodes))
	for nodeName, nodeIP := range newWorkerNodes {
		serverID := ctx.State.WorkerServerIDs[nodeName]
		nodeConfig, err := ctx.Talos.GenerateWorkerConfig(nodeName, serverID)
		if err != nil {
			return fmt.Errorf("failed to generate worker config for new node %s: %w", nodeName, err)
		}
		ctx.Observer.Printf("[%s] Applying config to new worker node %s (%s)...", phase, nodeName, nodeIP)
		if err := applyMachineConfig(ctx, nodeIP, nodeConfig); err != nil {
			return fmt.Errorf("failed to apply config to new worker node %s: %w", nodeName, err)
		}
	}

	for nodeName, nodeIP := range newWorkerNodes {
		ctx.Observer.Printf("[%s] Waiting for new worker node %s (%s) to be ready...", phase, nodeName, nodeIP)
		if err := waitForNodeReady(ctx, nodeIP, ctx.State.TalosConfig, ctx.Observer); err != nil {
			return fmt.Errorf("new worker node %s failed to become ready: %w", nodeName, err)
		}
		ctx.Observer.Printf("[%s] New worker node %s is ready", phase, nodeName)
	}
	return nil
}

// isNodeInMaintenanceMode checks if a node is in maintenance mode (unconfigured).
// A node in maintenance mode accepts insecure connections but won't respond
// to authenticated requests with the cluster's Talos config.
func isNodeInMaintenanceMode(ctx *provisioning.Context, nodeIP string) bool {
	portWaitTimeout := ctx.Timeouts.PortWait
	ctx.Observer.Printf("[%s] Waiting for Talos API on %s:50000...", phase, nodeIP)
	portCtx, cancel := context.WithTimeout(ctx, portWaitTimeout)
	defer cancel()
	if err := waitForPort(portCtx, nodeIP, 50000, portWaitTimeout, ctx.Timeouts.PortPoll, ctx.Timeouts.DialTimeout); err != nil {
		ctx.Observer.Printf("[%s] Node %s port 50000 not reachable, skipping", phase, nodeIP)
		return false
	}
	ctx.Observer.Printf("[%s] Node %s port 50000 is reachable", phase, nodeIP)

	if len(ctx.State.TalosConfig) == 0 {
		ctx.Observer.Printf("[%s] Warning: TalosConfig is empty, cannot check auth", phase)
		return false
	}

	if isAuthenticatedConnectionWorking(ctx, nodeIP) {
		return false
	}

	return isInsecureMaintenanceMode(ctx, nodeIP)
}

// isAuthenticatedConnectionWorking tests if an authenticated Talos connection succeeds.
// Returns true if the node is already configured (auth works), false otherwise.
func isAuthenticatedConnectionWorking(ctx *provisioning.Context, nodeIP string) bool {
	cfg, err := config.FromString(string(ctx.State.TalosConfig))
	if err != nil {
		ctx.Observer.Printf("[%s] Warning: Failed to parse TalosConfig: %v", phase, err)
		return false
	}

	checkCtx, checkCancel := context.WithTimeout(ctx, 10*time.Second)
	defer checkCancel()

	authClient, err := client.New(checkCtx, client.WithConfig(cfg), client.WithEndpoints(nodeIP))
	if err != nil {
		ctx.Observer.Printf("[%s] Cannot create auth client for %s: %v - assuming maintenance mode", phase, nodeIP, err)
		return false
	}
	defer func() { _ = authClient.Close() }()

	_, err = authClient.Version(checkCtx)
	if err == nil {
		ctx.Observer.Printf("[%s] Node %s: authenticated connection succeeded - already configured", phase, nodeIP)
		return true
	}
	ctx.Observer.Printf("[%s] Node %s: authenticated connection failed: %v - trying insecure", phase, nodeIP, err)
	return false
}

// isInsecureMaintenanceMode checks if a node responds to insecure connections,
// confirming it is in Talos maintenance mode.
func isInsecureMaintenanceMode(ctx *provisioning.Context, nodeIP string) bool {
	insecureCtx, insecureCancel := context.WithTimeout(ctx, 10*time.Second)
	defer insecureCancel()

	insecureClient, err := client.New(insecureCtx,
		client.WithEndpoints(nodeIP),
		//nolint:gosec // InsecureSkipVerify is required to detect Talos maintenance mode
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	if err != nil {
		ctx.Observer.Printf("[%s] Cannot create insecure client for %s: %v", phase, nodeIP, err)
		return false
	}
	defer func() { _ = insecureClient.Close() }()

	_, err = insecureClient.Version(insecureCtx)
	if err == nil {
		ctx.Observer.Printf("[%s] Node %s is in maintenance mode (insecure works, auth fails)", phase, nodeIP)
		return true
	}

	// "not implemented in maintenance mode" confirms the node IS in maintenance mode
	errStr := err.Error()
	if strings.Contains(errStr, "not implemented in maintenance mode") ||
		strings.Contains(errStr, "maintenance mode") {
		ctx.Observer.Printf("[%s] Node %s is in maintenance mode (detected via error message)", phase, nodeIP)
		return true
	}

	ctx.Observer.Printf("[%s] Node %s: both auth and insecure failed: %v", phase, nodeIP, err)
	return false
}

// applyMachineConfig applies a machine configuration to a Talos node.
func applyMachineConfig(ctx *provisioning.Context, nodeIP string, machineConfig []byte) error {
	if err := waitForPort(ctx, nodeIP, 50000, ctx.Timeouts.TalosAPI, ctx.Timeouts.PortPoll, ctx.Timeouts.DialTimeout); err != nil {
		return fmt.Errorf("failed to wait for Talos API: %w", err)
	}

	// Fresh Talos nodes boot into maintenance mode without credentials,
	// so we must use an insecure connection to apply the initial config.
	clientCtx, err := client.New(ctx,
		client.WithEndpoints(nodeIP),
		//nolint:gosec // InsecureSkipVerify is required for Talos maintenance mode
		client.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = clientCtx.Close() }()

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
func waitForNodeReady(ctx *provisioning.Context, nodeIP string, clientConfigBytes []byte, observer provisioning.Observer) error {
	cfg, err := config.FromString(string(clientConfigBytes))
	if err != nil {
		return fmt.Errorf("failed to parse client config: %w", err)
	}

	observer.Printf("[%s] Waiting for node %s to begin reboot...", phase, nodeIP)
	time.Sleep(defaultRebootInitialWait)

	observer.Printf("[%s] Waiting for node %s to come back online...", phase, nodeIP)
	if err := waitForPort(ctx, nodeIP, 50000, ctx.Timeouts.TalosAPI, ctx.Timeouts.PortPoll, ctx.Timeouts.DialTimeout); err != nil {
		return fmt.Errorf("failed to wait for node to come back: %w", err)
	}

	clientCtx, err := client.New(ctx,
		client.WithConfig(cfg),
		client.WithEndpoints(nodeIP),
	)
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = clientCtx.Close() }()

	ticker := time.NewTicker(ctx.Timeouts.NodeReadyPoll)
	defer ticker.Stop()

	timeout := time.After(ctx.Timeouts.NodeReady)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for node to be ready")
		case <-ticker.C:
			_, err := clientCtx.Version(ctx)
			if err == nil {
				return nil
			}
			observer.Printf("[%s] Node %s not yet ready, waiting... (error: %v)", phase, nodeIP, err)
		}
	}
}

// retrieveKubeconfig retrieves the kubeconfig from the cluster using the first control plane IP.
func retrieveKubeconfig(ctx *provisioning.Context, controlPlaneNodes map[string]string, clientConfigBytes []byte, observer provisioning.Observer) ([]byte, error) {
	var firstCPIP string
	for _, ip := range controlPlaneNodes {
		firstCPIP = ip
		break
	}
	return retrieveKubeconfigFromEndpoint(ctx, firstCPIP, clientConfigBytes, observer)
}

// retrieveKubeconfigFromEndpoint retrieves the kubeconfig from a specific endpoint.
func retrieveKubeconfigFromEndpoint(ctx *provisioning.Context, endpoint string, clientConfigBytes []byte, observer provisioning.Observer) ([]byte, error) {
	cfg, err := config.FromString(string(clientConfigBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse client config: %w", err)
	}

	clientCtx, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(endpoint))
	if err != nil {
		return nil, fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = clientCtx.Close() }()

	observer.Printf("[%s] Waiting for Kubernetes API to become ready via %s...", phase, endpoint)
	ticker := time.NewTicker(ctx.Timeouts.NodeReadyPoll)
	defer ticker.Stop()

	timeout := time.After(ctx.Timeouts.Kubeconfig)

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for Kubernetes API to be ready")
		case <-ticker.C:
			kubeconfigBytes, err := clientCtx.Kubeconfig(ctx)
			if err == nil && len(kubeconfigBytes) > 0 {
				observer.Printf("[%s] Kubernetes API is ready!", phase)
				return kubeconfigBytes, nil
			}
			observer.Printf("[%s] Kubernetes API not yet ready, waiting... (error: %v)", phase, err)
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
