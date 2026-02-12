// Package cluster provides cluster bootstrap and configuration functionality.
// This file manages the bootstrap process which initializes a new cluster by applying
// machine configs, waiting for nodes to be ready, and retrieving the kubeconfig.
package cluster

import (
	"fmt"
	"strings"
	"time"

	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/naming"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
)

const (
	phase = "cluster"

	// Bootstrap timing constants for config application
	nodeRebootWaitInterval = 10 * time.Second
	configRetryInterval    = 3 * time.Second
	maxConfigApplyRetries  = 10
)

// getLBEndpoint returns the Load Balancer endpoint for Talos API communication.
// In private-first mode, ALL external communication goes through the LB.
// Returns empty string if LB is not available.
func (p *Provisioner) getLBEndpoint(ctx *provisioning.Context) string {
	// Check state first
	if ctx.State.LoadBalancer != nil {
		if lbIP := hcloud.LoadBalancerIPv4(ctx.State.LoadBalancer); lbIP != "" {
			return lbIP
		}
	}
	// Fetch from API
	lb, err := ctx.Infra.GetLoadBalancer(ctx, naming.KubeAPILoadBalancer(ctx.Config.ClusterName))
	if err == nil && lb != nil {
		ctx.State.LoadBalancer = lb
		if lbIP := hcloud.LoadBalancerIPv4(lb); lbIP != "" {
			return lbIP
		}
	}
	return ""
}

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

// applyControlPlaneConfigs applies machine configs to all control plane nodes.
// In private-first mode, configs are applied sequentially via the Load Balancer.
func (p *Provisioner) applyControlPlaneConfigs(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Applying machine configurations to control plane nodes...", phase)

	if ctx.Config.IsPrivateFirst() {
		ctx.Logger.Printf("[%s] Private-first mode: applying configs via Load Balancer", phase)
		return p.applyControlPlaneConfigsViaLB(ctx)
	}

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

// applyControlPlaneConfigsViaLB applies machine configs via the Load Balancer.
// Each configured node reboots and exits maintenance mode, so the LB routes
// subsequent requests to remaining maintenance-mode nodes.
func (p *Provisioner) applyControlPlaneConfigsViaLB(ctx *provisioning.Context) error {
	lbEndpoint := p.getLBEndpoint(ctx)
	if lbEndpoint == "" {
		return fmt.Errorf("private-first mode requires Load Balancer but none available")
	}

	ctx.Logger.Printf("[%s] Private-first mode: all Talos API communication via LB %s:50000", phase, lbEndpoint)

	ctx.Logger.Printf("[%s] Waiting for LB to have healthy control plane targets...", phase)
	if err := waitForPort(ctx, lbEndpoint, 50000, ctx.Timeouts.TalosAPI, ctx.Timeouts.PortPoll, ctx.Timeouts.DialTimeout); err != nil {
		return fmt.Errorf("LB port 50000 not reachable: %w", err)
	}

	nodeCount := len(ctx.State.ControlPlaneIPs)
	ctx.Logger.Printf("[%s] Applying configs to %d control plane nodes via LB...", phase, nodeCount)

	configNum := 0
	for nodeName := range ctx.State.ControlPlaneIPs {
		configNum++
		if err := p.applyOneConfigViaLB(ctx, lbEndpoint, nodeName, configNum, nodeCount); err != nil {
			return err
		}

		// Wait for node to start rebooting before applying next config
		if configNum < nodeCount {
			ctx.Logger.Printf("[%s] Waiting for node to reboot before next config...", phase)
			time.Sleep(nodeRebootWaitInterval)
		}
	}

	ctx.Logger.Printf("[%s] All %d control plane configs applied via LB", phase, nodeCount)
	return nil
}

// applyOneConfigViaLB applies a single control plane config via the LB with retry logic.
func (p *Provisioner) applyOneConfigViaLB(ctx *provisioning.Context, lbEndpoint, nodeName string, configNum, nodeCount int) error {
	serverID := ctx.State.ControlPlaneServerIDs[nodeName]
	machineConfig, err := ctx.Talos.GenerateControlPlaneConfig(ctx.State.SANs, nodeName, serverID)
	if err != nil {
		return fmt.Errorf("failed to generate config for %s: %w", nodeName, err)
	}

	ctx.Logger.Printf("[%s] Applying config %d/%d (for %s) via LB...", phase, configNum, nodeCount, nodeName)

	var applyErr error
	for retry := 0; retry < maxConfigApplyRetries; retry++ {
		applyErr = p.applyMachineConfig(ctx, lbEndpoint, machineConfig)
		if applyErr == nil {
			ctx.Logger.Printf("[%s] Config %d/%d applied successfully", phase, configNum, nodeCount)
			return nil
		}
		// TLS errors indicate the node is already configured (requires mTLS)
		errStr := applyErr.Error()
		if strings.Contains(errStr, "certificate") ||
			strings.Contains(errStr, "handshake") ||
			strings.Contains(errStr, "tls") ||
			strings.Contains(errStr, "authentication") {
			ctx.Logger.Printf("[%s] LB hit configured node, retrying (%d/%d)...", phase, retry+1, maxConfigApplyRetries)
			time.Sleep(configRetryInterval)
			continue
		}
		break // Other errors fail immediately
	}
	return fmt.Errorf("failed to apply config for %s after %d retries: %w", nodeName, maxConfigApplyRetries, applyErr)
}

// waitForControlPlaneReady waits for all control plane nodes to reboot and become ready.
func (p *Provisioner) waitForControlPlaneReady(ctx *provisioning.Context) error {
	ctx.Logger.Printf("[%s] Waiting for nodes to reboot and become ready...", phase)

	if ctx.Config.IsPrivateFirst() {
		return p.waitForControlPlaneReadyViaLB(ctx)
	}

	for nodeName, nodeIP := range ctx.State.ControlPlaneIPs {
		ctx.Logger.Printf("[%s] Waiting for node %s (%s) to be ready...", phase, nodeName, nodeIP)
		if err := p.waitForNodeReady(ctx, nodeIP, ctx.State.TalosConfig, ctx.Logger); err != nil {
			return fmt.Errorf("node %s failed to become ready: %w", nodeName, err)
		}
		ctx.Logger.Printf("[%s] Node %s is ready", phase, nodeName)
	}
	return nil
}

// waitForControlPlaneReadyViaLB waits for control plane nodes via the Load Balancer.
func (p *Provisioner) waitForControlPlaneReadyViaLB(ctx *provisioning.Context) error {
	lbEndpoint := p.getLBEndpoint(ctx)
	if lbEndpoint == "" {
		return fmt.Errorf("private-first mode requires Load Balancer but none available")
	}

	expectedNodes := len(ctx.State.ControlPlaneIPs)
	ctx.Logger.Printf("[%s] Waiting for control plane to be ready via LB %s...", phase, lbEndpoint)

	cfg, err := config.FromString(string(ctx.State.TalosConfig))
	if err != nil {
		return fmt.Errorf("failed to parse talos config: %w", err)
	}

	ctx.Logger.Printf("[%s] Waiting for LB port 50000 to be available...", phase)
	if err := waitForPort(ctx, lbEndpoint, 50000, ctx.Timeouts.TalosAPI, ctx.Timeouts.PortPoll, ctx.Timeouts.DialTimeout); err != nil {
		return fmt.Errorf("LB port 50000 not reachable after reboot: %w", err)
	}

	ticker := time.NewTicker(ctx.Timeouts.NodeReadyPoll)
	defer ticker.Stop()
	timeout := time.After(ctx.Timeouts.NodeReady * time.Duration(expectedNodes))

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for control plane to be ready via LB")
		case <-ticker.C:
			clientCtx, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(lbEndpoint))
			if err != nil {
				ctx.Logger.Printf("[%s] Cannot create Talos client: %v", phase, err)
				continue
			}

			_, err = clientCtx.Version(ctx)
			_ = clientCtx.Close()

			if err == nil {
				ctx.Logger.Printf("[%s] Control plane ready via LB (authenticated connection succeeded)", phase)
				return nil
			}
			ctx.Logger.Printf("[%s] Control plane not yet ready: %v", phase, err)
		}
	}
}

// bootstrapEtcd initializes etcd on a control plane node.
func (p *Provisioner) bootstrapEtcd(ctx *provisioning.Context) error {
	var endpoint string
	if ctx.Config.IsPrivateFirst() {
		endpoint = p.getLBEndpoint(ctx)
		if endpoint == "" {
			return fmt.Errorf("private-first mode requires Load Balancer but none available")
		}
	} else {
		endpoint = p.getFirstControlPlaneIP(ctx)
	}

	cfg, err := config.FromString(string(ctx.State.TalosConfig))
	if err != nil {
		return fmt.Errorf("failed to parse talos config: %w", err)
	}
	clientCtx, err := client.New(ctx, client.WithConfig(cfg), client.WithEndpoints(endpoint))
	if err != nil {
		return fmt.Errorf("failed to create talos client: %w", err)
	}
	defer func() { _ = clientCtx.Close() }()

	ctx.Logger.Printf("[%s] Bootstrapping etcd via %s...", phase, endpoint)
	if err := clientCtx.Bootstrap(ctx, &machine.BootstrapRequest{}); err != nil {
		return fmt.Errorf("failed to bootstrap etcd: %w", err)
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

	var endpoint string
	if ctx.Config.IsPrivateFirst() {
		endpoint = p.getLBEndpoint(ctx)
		if endpoint == "" {
			return fmt.Errorf("private-first mode requires Load Balancer but none available")
		}
	} else {
		endpoint = p.getFirstControlPlaneIP(ctx)
	}

	kubeconfig, err := p.retrieveKubeconfigFromEndpoint(ctx, endpoint, ctx.State.TalosConfig, ctx.Logger)
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
