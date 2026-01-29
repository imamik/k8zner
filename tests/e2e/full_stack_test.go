//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/orchestration"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/platform/talos"
	"github.com/imamik/k8zner/internal/util/keygen"
)

// TestE2EDevCluster runs the full E2E test for a dev cluster configuration.
//
// Dev Mode Configuration:
//   - 1 Control Plane node (cx23)
//   - 1 Worker node (cx22)
//   - Shared Load Balancer (API + Ingress on same LB)
//   - All standard addons
//
// This test deploys everything at once, then verifies with a checklist.
//
// Environment variables:
//
//	HCLOUD_TOKEN - Required
//	E2E_KEEP_SNAPSHOTS - Set to "true" to cache snapshots
//
// Example:
//
//	HCLOUD_TOKEN=xxx go test -v -timeout=45m -tags=e2e -run TestE2EDevCluster ./tests/e2e/
func TestE2EDevCluster(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	clusterName := fmt.Sprintf("e2e-dev-%d", time.Now().Unix())
	t.Logf("=== Starting Dev Cluster E2E Test: %s ===", clusterName)

	client := hcloud.NewRealClient(token)
	state := NewE2EState(clusterName, client)
	defer cleanupE2ECluster(t, state)

	// === PHASE 1: DEPLOY EVERYTHING ===
	t.Log("=== DEPLOYMENT PHASE ===")

	// 1.1 Get snapshot (from sharedCtx or build)
	deployGetSnapshot(t, state)

	// 1.2 Deploy cluster infrastructure + bootstrap
	cfg := deployDevCluster(t, state, token)

	// 1.3 Install all addons
	deployAllAddons(t, state, cfg, token)

	t.Log("=== DEPLOYMENT COMPLETE ===")

	// === PHASE 2: VERIFICATION CHECKLIST ===
	t.Log("=== VERIFICATION CHECKLIST ===")

	// Infrastructure checks
	t.Run("Checklist_Infrastructure", func(t *testing.T) {
		checklistInfrastructure(t, state, 1, 1) // 1 CP, 1 worker
	})

	// Kubernetes cluster checks
	t.Run("Checklist_Kubernetes", func(t *testing.T) {
		checklistKubernetes(t, state, 2) // 2 total nodes
	})

	// Core addon checks
	t.Run("Checklist_CoreAddons", func(t *testing.T) {
		checklistCoreAddons(t, state)
	})

	// Stack addon checks
	t.Run("Checklist_StackAddons", func(t *testing.T) {
		checklistStackAddons(t, state)
	})

	// Functional tests
	t.Run("Checklist_Functional", func(t *testing.T) {
		checklistFunctionalTests(t, state)
	})

	t.Log("=== DEV CLUSTER E2E TEST PASSED ===")
}

// TestE2EHACluster runs the full E2E test for an HA cluster configuration.
//
// HA Mode Configuration:
//   - 3 Control Plane nodes (cx23)
//   - 2 Worker nodes (cx22)
//   - Separate Load Balancers for API and Ingress
//   - All standard addons
//
// This test deploys everything at once, then verifies with a checklist.
//
// Environment variables:
//
//	HCLOUD_TOKEN - Required
//	E2E_KEEP_SNAPSHOTS - Set to "true" to cache snapshots
//
// Example:
//
//	HCLOUD_TOKEN=xxx go test -v -timeout=60m -tags=e2e -run TestE2EHACluster ./tests/e2e/
func TestE2EHACluster(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	clusterName := fmt.Sprintf("e2e-ha-%d", time.Now().Unix())
	t.Logf("=== Starting HA Cluster E2E Test: %s ===", clusterName)

	client := hcloud.NewRealClient(token)
	state := NewE2EState(clusterName, client)
	defer cleanupE2ECluster(t, state)

	// === PHASE 1: DEPLOY EVERYTHING ===
	t.Log("=== DEPLOYMENT PHASE ===")

	// 1.1 Get snapshot (from sharedCtx or build)
	deployGetSnapshot(t, state)

	// 1.2 Deploy cluster infrastructure + bootstrap
	cfg := deployHACluster(t, state, token)

	// 1.3 Install all addons
	deployAllAddons(t, state, cfg, token)

	t.Log("=== DEPLOYMENT COMPLETE ===")

	// === PHASE 2: VERIFICATION CHECKLIST ===
	t.Log("=== VERIFICATION CHECKLIST ===")

	// Infrastructure checks
	t.Run("Checklist_Infrastructure", func(t *testing.T) {
		checklistInfrastructure(t, state, 3, 2) // 3 CP, 2 workers
	})

	// Kubernetes cluster checks
	t.Run("Checklist_Kubernetes", func(t *testing.T) {
		checklistKubernetes(t, state, 5) // 5 total nodes
	})

	// Core addon checks
	t.Run("Checklist_CoreAddons", func(t *testing.T) {
		checklistCoreAddons(t, state)
	})

	// Stack addon checks
	t.Run("Checklist_StackAddons", func(t *testing.T) {
		checklistStackAddons(t, state)
	})

	// Functional tests
	t.Run("Checklist_Functional", func(t *testing.T) {
		checklistFunctionalTests(t, state)
	})

	// HA-specific checks
	t.Run("Checklist_HA", func(t *testing.T) {
		checklistHASpecific(t, state)
	})

	t.Log("=== HA CLUSTER E2E TEST PASSED ===")
}

// =============================================================================
// DEPLOYMENT FUNCTIONS
// =============================================================================

// deployGetSnapshot ensures a snapshot is available (from sharedCtx or builds one).
func deployGetSnapshot(t *testing.T, state *E2EState) {
	t.Log("[Deploy] Getting Talos snapshot...")

	// Check sharedCtx first (from TestMain)
	if sharedCtx != nil && sharedCtx.SnapshotAMD64 != "" {
		state.SnapshotAMD64 = sharedCtx.SnapshotAMD64
		t.Logf("[Deploy] Using shared snapshot: %s", state.SnapshotAMD64)
		return
	}

	// Phase snapshots will build if needed
	phaseSnapshots(t, state)
}

// deployDevCluster deploys a dev mode cluster (1 CP, 1 worker, shared LB).
func deployDevCluster(t *testing.T, state *E2EState, token string) *config.Config {
	t.Log("[Deploy] Deploying Dev cluster (1 CP, 1 Worker, shared LB)...")

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	// Setup SSH key
	if err := setupSSHKeyForFullStack(ctx, t, state); err != nil {
		t.Fatalf("Failed to setup SSH key: %v", err)
	}

	// Create v2 config (dev mode)
	v2Cfg := &v2.Config{
		Name:   state.ClusterName,
		Region: v2.RegionNuremberg,
		Mode:   v2.ModeDev,
		Workers: v2.Worker{
			Count: 1,
			Size:  v2.SizeCX22,
		},
	}

	return deployClusterWithConfig(ctx, t, state, v2Cfg, token)
}

// deployHACluster deploys an HA mode cluster (3 CPs, 2 workers, separate LBs).
func deployHACluster(t *testing.T, state *E2EState, token string) *config.Config {
	t.Log("[Deploy] Deploying HA cluster (3 CPs, 2 Workers, separate LBs)...")

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()

	// Setup SSH key
	if err := setupSSHKeyForFullStack(ctx, t, state); err != nil {
		t.Fatalf("Failed to setup SSH key: %v", err)
	}

	// Create v2 config (HA mode)
	v2Cfg := &v2.Config{
		Name:   state.ClusterName,
		Region: v2.RegionNuremberg,
		Mode:   v2.ModeHA,
		Workers: v2.Worker{
			Count: 2,
			Size:  v2.SizeCX22,
		},
	}

	return deployClusterWithConfig(ctx, t, state, v2Cfg, token)
}

// deployClusterWithConfig deploys a cluster with the given v2 config.
func deployClusterWithConfig(ctx context.Context, t *testing.T, state *E2EState, v2Cfg *v2.Config, token string) *config.Config {
	vm := v2.DefaultVersionMatrix()

	// Expand to internal config
	cfg, err := v2.Expand(v2Cfg)
	if err != nil {
		t.Fatalf("Failed to expand v2 config: %v", err)
	}

	cfg.TestID = state.TestID
	cfg.SSHKeys = []string{state.SSHKeyName}
	cfg.HCloudToken = token

	// Generate Talos secrets
	secrets, err := talos.GetOrGenerateSecrets("/tmp/talos-secrets-"+state.ClusterName+".json", vm.Talos)
	if err != nil {
		t.Fatalf("Failed to generate Talos secrets: %v", err)
	}
	state.TalosSecretsPath = "/tmp/talos-secrets-" + state.ClusterName + ".json"

	talosGen := talos.NewGenerator(state.ClusterName, "v"+vm.Kubernetes, vm.Talos, "", secrets)

	state.TalosConfig, err = talosGen.GetClientConfig()
	if err != nil {
		t.Logf("Warning: Could not get Talos config: %v", err)
	}

	// Create reconciler and provision cluster
	reconciler := orchestration.NewReconciler(state.Client, talosGen, cfg)

	t.Log("[Deploy] Starting cluster reconciliation...")
	startTime := time.Now()
	kubeconfig, err := reconciler.Reconcile(ctx)
	duration := time.Since(startTime)

	if err != nil {
		runClusterDiagnostics(ctx, t, state)
		t.Fatalf("Cluster provisioning failed after %v: %v", duration, err)
	}

	state.Kubeconfig = kubeconfig
	t.Logf("[Deploy] Cluster provisioned in %v", duration)

	// Save kubeconfig
	kubeconfigPath := fmt.Sprintf("/tmp/kubeconfig-%s", state.ClusterName)
	if err := os.WriteFile(kubeconfigPath, kubeconfig, 0600); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}
	state.KubeconfigPath = kubeconfigPath

	// Collect state info
	collectClusterState(ctx, t, state)

	return cfg
}

// deployAllAddons installs all standard addons.
func deployAllAddons(t *testing.T, state *E2EState, cfg *config.Config, token string) {
	t.Log("[Deploy] Installing all addons...")

	ctx := context.Background()

	// Get network ID for CCM/CSI
	network, _ := state.Client.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	// Build full addon config (versions come from addons package defaults)
	fullCfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: token,
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Addons: config.AddonsConfig{
			// Core addons
			Cilium: config.CiliumConfig{
				Enabled:                     true,
				EncryptionEnabled:           true,
				EncryptionType:              "wireguard",
				RoutingMode:                 "native",
				KubeProxyReplacementEnabled: true,
				HubbleEnabled:               true,
				HubbleRelayEnabled:          true,
				HubbleUIEnabled:             false,
			},
			CCM: config.CCMConfig{
				Enabled: true,
			},
			CSI: config.CSIConfig{
				Enabled:             true,
				DefaultStorageClass: true,
			},
			MetricsServer: config.MetricsServerConfig{
				Enabled: true,
			},
			// Stack addons
			CertManager: config.CertManagerConfig{
				Enabled: true,
			},
			Traefik: config.TraefikConfig{
				Enabled: true,
			},
			ArgoCD: config.ArgoCDConfig{
				Enabled: true,
			},
		},
	}

	startTime := time.Now()
	if err := addons.Apply(ctx, fullCfg, state.Kubeconfig, networkID); err != nil {
		t.Fatalf("Failed to install addons: %v", err)
	}
	duration := time.Since(startTime)

	t.Logf("[Deploy] All addons installed in %v", duration)
}

// =============================================================================
// CHECKLIST VERIFICATION FUNCTIONS
// =============================================================================

// checklistInfrastructure verifies Hetzner infrastructure resources.
func checklistInfrastructure(t *testing.T, state *E2EState, expectedCPs, expectedWorkers int) {
	t.Log("[ ] Checking infrastructure resources...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Check network exists
	t.Run("Network", func(t *testing.T) {
		network, err := state.Client.GetNetwork(ctx, state.ClusterName)
		if err != nil || network == nil {
			t.Fatalf("Network %s not found", state.ClusterName)
		}
		t.Logf("[x] Network: %s (ID: %d)", network.Name, network.ID)
	})

	// Check firewall exists
	t.Run("Firewall", func(t *testing.T) {
		fw, err := state.Client.GetFirewall(ctx, state.ClusterName)
		if err != nil || fw == nil {
			t.Fatalf("Firewall %s not found", state.ClusterName)
		}
		t.Logf("[x] Firewall: %s (ID: %d)", fw.Name, fw.ID)
	})

	// Check load balancer(s)
	t.Run("LoadBalancer", func(t *testing.T) {
		lb, err := state.Client.GetLoadBalancer(ctx, state.ClusterName+"-kube-api")
		if err != nil || lb == nil {
			t.Fatalf("Load balancer %s-kube-api not found", state.ClusterName)
		}
		t.Logf("[x] Load Balancer: %s (IP: %s)", lb.Name, hcloud.LoadBalancerIPv4(lb))
	})

	// Check control plane servers
	t.Run("ControlPlanes", func(t *testing.T) {
		for i := 1; i <= expectedCPs; i++ {
			serverName := fmt.Sprintf("%s-control-plane-%d", state.ClusterName, i)
			ip, err := state.Client.GetServerIP(ctx, serverName)
			if err != nil {
				t.Errorf("Control plane %d not found: %v", i, err)
			} else {
				t.Logf("[x] Control Plane %d: %s (IP: %s)", i, serverName, ip)
			}
		}
	})

	// Check worker servers
	t.Run("Workers", func(t *testing.T) {
		for i := 1; i <= expectedWorkers; i++ {
			serverName := fmt.Sprintf("%s-workers-%d", state.ClusterName, i)
			ip, err := state.Client.GetServerIP(ctx, serverName)
			if err != nil {
				t.Errorf("Worker %d not found: %v", i, err)
			} else {
				t.Logf("[x] Worker %d: %s (IP: %s)", i, serverName, ip)
			}
		}
	})

	t.Log("[x] Infrastructure checklist passed")
}

// checklistKubernetes verifies Kubernetes cluster health.
func checklistKubernetes(t *testing.T, state *E2EState, expectedNodes int) {
	t.Log("[ ] Checking Kubernetes cluster...")

	// Check API server is reachable
	t.Run("APIServer", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		if state.LoadBalancerIP == "" {
			t.Fatal("No load balancer IP available")
		}
		if err := WaitForPort(ctx, state.LoadBalancerIP, 6443, 2*time.Minute); err != nil {
			t.Fatalf("API server not reachable: %v", err)
		}
		t.Logf("[x] API Server reachable at %s:6443", state.LoadBalancerIP)
	})

	// Check node count
	t.Run("NodeCount", func(t *testing.T) {
		waitForNodesReady(t, state.KubeconfigPath, expectedNodes, 5*time.Minute)
		t.Logf("[x] All %d nodes ready", expectedNodes)
	})

	// Check all nodes are Ready
	t.Run("NodeStatus", func(t *testing.T) {
		cmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", state.KubeconfigPath,
			"get", "nodes", "-o", "wide")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to get nodes: %v", err)
		}
		t.Logf("[x] Node status:\n%s", string(output))
	})

	// Check system pods are running
	t.Run("SystemPods", func(t *testing.T) {
		cmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", state.KubeconfigPath,
			"get", "pods", "-n", "kube-system", "--no-headers")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to get system pods: %v", err)
		}
		lines := 0
		for _, b := range output {
			if b == '\n' {
				lines++
			}
		}
		t.Logf("[x] %d pods running in kube-system", lines)
	})

	t.Log("[x] Kubernetes checklist passed")
}

// checklistCoreAddons verifies core addons are working.
func checklistCoreAddons(t *testing.T, state *E2EState) {
	t.Log("[ ] Checking core addons...")

	// Check Cilium
	t.Run("Cilium", func(t *testing.T) {
		waitForPod(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=cilium-operator", 3*time.Minute)
		waitForDaemonSet(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=cilium-agent", 3*time.Minute)
		t.Log("[x] Cilium CNI running")
		state.AddonsInstalled["cilium"] = true
	})

	// Check CCM (Hetzner CCM handles LoadBalancers and sets provider IDs)
	t.Run("CCM", func(t *testing.T) {
		// Wait for Hetzner CCM pod to be ready
		waitForPod(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=hcloud-cloud-controller-manager", 5*time.Minute)
		// Verify provider IDs are set - CCM needs time to initialize and set provider IDs on all nodes
		verifyProviderIDs(t, state.KubeconfigPath, 4*time.Minute)
		t.Log("[x] Hetzner CCM running with provider IDs set")
		state.AddonsInstalled["ccm"] = true
	})

	// Check CSI
	t.Run("CSI", func(t *testing.T) {
		waitForPod(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=hcloud-csi,app.kubernetes.io/component=controller", 3*time.Minute)
		verifyStorageClass(t, state.KubeconfigPath)
		t.Log("[x] Hetzner CSI running with StorageClass")
		state.AddonsInstalled["csi"] = true
	})

	// Check Metrics Server
	t.Run("MetricsServer", func(t *testing.T) {
		waitForPod(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=metrics-server", 3*time.Minute)
		testMetricsAPI(t, state.KubeconfigPath)
		t.Log("[x] Metrics Server running")
		state.AddonsInstalled["metrics-server"] = true
	})

	t.Log("[x] Core addons checklist passed")
}

// checklistStackAddons verifies stack addons are working.
func checklistStackAddons(t *testing.T, state *E2EState) {
	t.Log("[ ] Checking stack addons...")

	// Check Cert Manager
	t.Run("CertManager", func(t *testing.T) {
		waitForPod(t, state.KubeconfigPath, "cert-manager", "app.kubernetes.io/component=controller", 3*time.Minute)
		verifyCRDExists(t, state.KubeconfigPath, "certificates.cert-manager.io")
		t.Log("[x] Cert Manager running")
		state.AddonsInstalled["cert-manager"] = true
	})

	// Check Traefik
	t.Run("Traefik", func(t *testing.T) {
		waitForPod(t, state.KubeconfigPath, "traefik", "app.kubernetes.io/name=traefik", 5*time.Minute)
		verifyIngressClassExists(t, state.KubeconfigPath, "traefik")
		t.Log("[x] Traefik ingress controller running")
		state.AddonsInstalled["traefik"] = true
	})

	// Check ArgoCD
	t.Run("ArgoCD", func(t *testing.T) {
		waitForPod(t, state.KubeconfigPath, "argocd", "app.kubernetes.io/name=argocd-server", 5*time.Minute)
		verifyCRDExists(t, state.KubeconfigPath, "applications.argoproj.io")
		t.Log("[x] ArgoCD running")
		state.AddonsInstalled["argocd"] = true
	})

	t.Log("[x] Stack addons checklist passed")
}

// checklistFunctionalTests runs functional tests for CCM and CSI.
func checklistFunctionalTests(t *testing.T, state *E2EState) {
	t.Log("[ ] Running functional tests...")

	// Test CCM load balancer provisioning
	t.Run("CCM_LoadBalancer", func(t *testing.T) {
		testCCMLoadBalancer(t, state)
		t.Log("[x] CCM can provision load balancers")
	})

	// Test CSI volume provisioning
	t.Run("CSI_Volume", func(t *testing.T) {
		testCSIVolume(t, state)
		t.Log("[x] CSI can provision and mount volumes")
	})

	// Test Cilium network connectivity
	t.Run("Cilium_Network", func(t *testing.T) {
		testCiliumNetworkConnectivity(t, state)
		t.Log("[x] Cilium network connectivity working")
	})

	t.Log("[x] Functional tests checklist passed")
}

// checklistHASpecific runs HA-specific verification checks.
func checklistHASpecific(t *testing.T, state *E2EState) {
	t.Log("[ ] Running HA-specific checks...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Check etcd cluster has 3 members
	t.Run("EtcdCluster", func(t *testing.T) {
		if len(state.TalosConfig) > 0 && len(state.ControlPlaneIPs) > 0 {
			diag := NewClusterDiagnostics(t, state.ControlPlaneIPs[0], state.LoadBalancerIP, state.TalosConfig)
			diag.checkEtcdStatus(ctx)
		} else {
			t.Log("[x] etcd check skipped (no Talos config)")
		}
	})

	// Check all 3 control planes are accessible
	t.Run("ControlPlaneAccess", func(t *testing.T) {
		for i, ip := range state.ControlPlaneIPs {
			if err := quickPortCheck(ip, 6443); err != nil {
				t.Errorf("Control plane %d (%s) API not accessible: %v", i+1, ip, err)
			} else {
				t.Logf("[x] Control plane %d (%s) API accessible", i+1, ip)
			}
		}
	})

	t.Log("[x] HA-specific checklist passed")
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// setupSSHKeyForFullStack generates and uploads an SSH key for cluster deployment.
func setupSSHKeyForFullStack(ctx context.Context, t *testing.T, state *E2EState) error {
	keyName := fmt.Sprintf("%s-key-%d", state.ClusterName, time.Now().UnixNano())

	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	labels := map[string]string{
		"cluster": state.ClusterName,
		"test-id": state.TestID,
	}

	_, err = state.Client.CreateSSHKey(ctx, keyName, string(keyPair.PublicKey), labels)
	if err != nil {
		return fmt.Errorf("failed to upload SSH key: %w", err)
	}

	state.SSHKeyName = keyName
	state.SSHPrivateKey = keyPair.PrivateKey
	return nil
}

// collectClusterState gathers IPs and other state info after cluster creation.
func collectClusterState(ctx context.Context, t *testing.T, state *E2EState) {
	// Collect control plane IPs
	for i := 1; i <= 3; i++ {
		serverName := fmt.Sprintf("%s-control-plane-%d", state.ClusterName, i)
		ip, err := state.Client.GetServerIP(ctx, serverName)
		if err == nil {
			state.ControlPlaneIPs = append(state.ControlPlaneIPs, ip)
		}
	}

	// Collect worker IPs
	for i := 1; i <= 5; i++ {
		serverName := fmt.Sprintf("%s-workers-%d", state.ClusterName, i)
		ip, err := state.Client.GetServerIP(ctx, serverName)
		if err == nil {
			state.WorkerIPs = append(state.WorkerIPs, ip)
		}
	}

	// Get load balancer IP
	lb, err := state.Client.GetLoadBalancer(ctx, state.ClusterName+"-kube-api")
	if err == nil && lb != nil {
		state.LoadBalancerIP = hcloud.LoadBalancerIPv4(lb)
	}

	t.Logf("[Deploy] Collected state: %d CPs, %d Workers, LB IP: %s",
		len(state.ControlPlaneIPs), len(state.WorkerIPs), state.LoadBalancerIP)
}
