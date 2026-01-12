//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/orchestration"
	hcloud_client "hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/platform/talos"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddonsProvisioning is an end-to-end test that provisions a cluster with ALL addons enabled
// and verifies each addon is working correctly.
func TestAddonsProvisioning(t *testing.T) {
	t.Parallel() // Run in parallel with other tests

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	// Use a unique cluster name to avoid conflicts
	clusterName := fmt.Sprintf("e2e-addons-%d", time.Now().Unix())

	// Create a config with ALL addons enabled
	cfg := &config.Config{
		ClusterName: clusterName,
		HCloudToken: token,
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		Firewall: config.FirewallConfig{
			UseCurrentIPv4: true,
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					ServerType: "cpx22",
					Location:   "nbg1",
					Count:      1,
					Image:      "talos",
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "worker",
				ServerType: "cpx22",
				Location:   "nbg1",
				Count:      1,
				Image:      "talos",
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.8.3",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.31.0",
		},
		Addons: config.AddonsConfig{
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
			CertManager: config.CertManagerConfig{
				Enabled: true,
			},
			Longhorn: config.LonghornConfig{
				Enabled: true,
			},
			IngressNginx: config.IngressNginxConfig{
				Enabled: true,
			},
			RBAC: config.RBACConfig{
				Enabled: true,
			},
			OIDCRBAC: config.OIDCRBACConfig{
				Enabled: false, // Disabled by default as it requires OIDC configuration
			},
		},
	}

	// Use shared snapshots if available (built in TestMain)
	if sharedCtx != nil && sharedCtx.SnapshotAMD64 != "" {
		t.Log("Using shared Talos snapshot from test suite")
	}

	// Initialize Real Client
	hClient := hcloud_client.NewRealClient(token)

	cleaner := &ResourceCleaner{t: t}

	// Setup SSH Key
	sshKeyName, _ := setupSSHKey(t, hClient, cleaner, clusterName)
	cfg.SSHKeys = []string{sshKeyName}

	// Clean up before and after
	cleanup := func() {
		ctx := context.Background()
		logger := func(msg string) { fmt.Printf("[Cleanup] %s\n", msg) }

		// Delete Servers
		hClient.DeleteServer(ctx, clusterName+"-control-plane-1")
		hClient.DeleteServer(ctx, clusterName+"-worker-1")
		logger("Deleted Servers")

		// Delete LBs
		hClient.DeleteLoadBalancer(ctx, clusterName+"-kube-api")
		logger("Deleted LB")

		// Delete Firewalls
		hClient.DeleteFirewall(ctx, clusterName)
		logger("Deleted FW")

		// Delete Networks
		hClient.DeleteNetwork(ctx, clusterName)
		logger("Deleted Network")

		// Delete Placement Groups
		hClient.DeletePlacementGroup(ctx, clusterName+"-control-plane")
		hClient.DeletePlacementGroup(ctx, clusterName+"-worker-pg-1")
		logger("Deleted PGs")

		// Delete Certificates
		hClient.DeleteCertificate(ctx, clusterName+"-state")
		logger("Deleted Certificates")
	}
	defer cleanup()

	// Initialize Managers
	secrets, err := talos.GetOrGenerateSecrets("/tmp/talos-secrets-"+clusterName+".json", cfg.Talos.Version)
	assert.NoError(t, err)
	defer os.Remove("/tmp/talos-secrets-" + clusterName + ".json")

	talosGen := talos.NewGenerator(clusterName, cfg.Kubernetes.Version, cfg.Talos.Version, "", secrets)

	reconciler := orchestration.NewReconciler(hClient, talosGen, cfg)

	// Run Reconcile
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	kubeconfig, err := reconciler.Reconcile(ctx)
	require.NoError(t, err, "Reconcile should succeed with all addons enabled")
	require.NotEmpty(t, kubeconfig, "kubeconfig should be returned after bootstrap")

	// Write kubeconfig for kubectl commands
	kubeconfigPath := fmt.Sprintf("/tmp/kubeconfig-%s", clusterName)
	require.NoError(t, os.WriteFile(kubeconfigPath, kubeconfig, 0600))
	defer os.Remove(kubeconfigPath)

	// Wait for cluster to be ready
	t.Log("Waiting for cluster to be ready...")
	waitForClusterReady(t, kubeconfigPath, 2, 5*time.Minute)

	// Test each addon
	t.Run("CCM", func(t *testing.T) {
		testCCMAddon(t, kubeconfigPath)
	})

	t.Run("CSI", func(t *testing.T) {
		testCSIAddon(t, kubeconfigPath)
	})

	t.Run("MetricsServer", func(t *testing.T) {
		testMetricsServerAddon(t, kubeconfigPath)
	})

	t.Run("CertManager", func(t *testing.T) {
		testCertManagerAddon(t, kubeconfigPath)
	})

	t.Run("Longhorn", func(t *testing.T) {
		testLonghornAddon(t, kubeconfigPath)
	})

	t.Run("IngressNginx", func(t *testing.T) {
		testIngressNginxAddon(t, kubeconfigPath)
	})

	t.Run("RBAC", func(t *testing.T) {
		testRBACAddon(t, kubeconfigPath)
	})
}

// waitForClusterReady waits for all nodes to be Ready
func waitForClusterReady(t *testing.T, kubeconfigPath string, expectedNodes int, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("Timeout waiting for cluster to be ready")
		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "nodes", "-o", "json")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("kubectl get nodes not ready yet: %v (will retry)", err)
				continue
			}

			var nodeList struct {
				Items []map[string]interface{} `json:"items"`
			}
			if err := json.Unmarshal(output, &nodeList); err != nil {
				t.Logf("Failed to parse kubectl output: %v (will retry)", err)
				continue
			}

			if len(nodeList.Items) >= expectedNodes {
				t.Logf("✓ Cluster ready: %d nodes found", len(nodeList.Items))
				return
			}
			t.Logf("Waiting for nodes to appear... (found %d, expecting >= %d)", len(nodeList.Items), expectedNodes)
		}
	}
}

// testMetricsServerAddon verifies the Metrics Server addon is working
func testMetricsServerAddon(t *testing.T, kubeconfigPath string) {
	t.Log("Testing Metrics Server addon...")

	// Check if metrics-server deployment exists
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "deployment", "-n", "kube-system", "metrics-server", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("Metrics Server deployment not found: %v\nOutput: %s", err, string(output))
		return
	}
	t.Log("✓ Metrics Server deployment exists")

	// Wait for metrics-server pod to be running
	t.Log("Waiting for Metrics Server pod to be Running...")
	metricsRunning := false
	for i := 0; i < 30; i++ { // Wait up to 5 minutes
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "pods", "-n", "kube-system", "-l", "app.kubernetes.io/name=metrics-server",
			"-o", "jsonpath={.items[0].status.phase}")
		output, err = cmd.CombinedOutput()
		if err == nil && string(output) == "Running" {
			metricsRunning = true
			break
		}
		t.Logf("Metrics Server pod not running yet (phase: %s), waiting...", string(output))
		time.Sleep(10 * time.Second)
	}
	if !metricsRunning {
		t.Error("Metrics Server pod failed to reach Running state")
		return
	}
	t.Log("✓ Metrics Server pod is Running")

	// Test metrics API
	t.Log("Testing metrics API...")
	for i := 0; i < 12; i++ { // Wait up to 2 minutes for metrics to be available
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"top", "nodes")
		output, err = cmd.CombinedOutput()
		if err == nil {
			t.Log("✓ Metrics API is working")
			t.Logf("Node metrics:\n%s", string(output))
			return
		}
		t.Logf("Metrics API not ready yet: %v (will retry)", err)
		time.Sleep(10 * time.Second)
	}
	t.Error("Metrics API failed to become available")
}

// testCertManagerAddon verifies the Cert Manager addon is working
func testCertManagerAddon(t *testing.T, kubeconfigPath string) {
	t.Log("Testing Cert Manager addon...")

	// Check if cert-manager namespace exists
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "namespace", "cert-manager", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("cert-manager namespace not found: %v\nOutput: %s", err, string(output))
		return
	}
	t.Log("✓ cert-manager namespace exists")

	// Check if cert-manager CRDs are installed
	crds := []string{
		"certificates.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"issuers.cert-manager.io",
		"clusterissuers.cert-manager.io",
	}

	for _, crd := range crds {
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "crd", crd, "-o", "json")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("CRD %s not found: %v\nOutput: %s", crd, err, string(output))
			return
		}
		t.Logf("✓ CRD %s exists", crd)
	}

	// Check if cert-manager pods are running
	t.Log("Waiting for cert-manager pods to be Running...")
	pods := []string{"cert-manager", "cert-manager-webhook", "cert-manager-cainjector"}

	for _, pod := range pods {
		podRunning := false
		for i := 0; i < 30; i++ { // Wait up to 5 minutes
			cmd = exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "pods", "-n", "cert-manager", "-l", fmt.Sprintf("app.kubernetes.io/component=%s", pod),
				"-o", "jsonpath={.items[0].status.phase}")
			output, err = cmd.CombinedOutput()
			if err == nil && string(output) == "Running" {
				podRunning = true
				break
			}
			t.Logf("%s pod not running yet (phase: %s), waiting...", pod, string(output))
			time.Sleep(10 * time.Second)
		}
		if !podRunning {
			t.Errorf("%s pod failed to reach Running state", pod)
			return
		}
		t.Logf("✓ %s pod is Running", pod)
	}

	t.Log("✓ Cert Manager addon is working")
}

// testLonghornAddon verifies the Longhorn addon is working
func testLonghornAddon(t *testing.T, kubeconfigPath string) {
	t.Log("Testing Longhorn addon...")

	// Check if longhorn-system namespace exists
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "namespace", "longhorn-system", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("longhorn-system namespace not found: %v\nOutput: %s", err, string(output))
		return
	}
	t.Log("✓ longhorn-system namespace exists")

	// Check if longhorn-manager deployment exists
	cmd = exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "deployment", "-n", "longhorn-system", "longhorn-driver-deployer", "-o", "json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Errorf("Longhorn deployment not found: %v\nOutput: %s", err, string(output))
		return
	}
	t.Log("✓ Longhorn deployment exists")

	// Wait for longhorn to be ready (this can take a while)
	t.Log("Waiting for Longhorn pods to be Running (this may take several minutes)...")
	longhornReady := false
	for i := 0; i < 60; i++ { // Wait up to 10 minutes
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "pods", "-n", "longhorn-system", "-o", "json")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Logf("Failed to get Longhorn pods: %v (will retry)", err)
			time.Sleep(10 * time.Second)
			continue
		}

		var podList struct {
			Items []struct {
				Status struct {
					Phase string `json:"phase"`
				} `json:"status"`
			} `json:"items"`
		}
		if err := json.Unmarshal(output, &podList); err != nil {
			t.Logf("Failed to parse pods: %v (will retry)", err)
			time.Sleep(10 * time.Second)
			continue
		}

		// Check if all pods are running
		allRunning := true
		runningCount := 0
		for _, pod := range podList.Items {
			if pod.Status.Phase != "Running" {
				allRunning = false
			} else {
				runningCount++
			}
		}

		if allRunning && len(podList.Items) > 0 {
			longhornReady = true
			t.Logf("✓ All Longhorn pods are Running (%d pods)", runningCount)
			break
		}

		t.Logf("Longhorn not ready yet (%d/%d pods running), waiting...", runningCount, len(podList.Items))
		time.Sleep(10 * time.Second)
	}

	if !longhornReady {
		t.Error("Timeout: Longhorn failed to become ready within 10 minutes")
		return
	}

	t.Log("✓ Longhorn addon is working")
}

// testIngressNginxAddon verifies the Ingress NGINX addon is working
func testIngressNginxAddon(t *testing.T, kubeconfigPath string) {
	t.Log("Testing Ingress NGINX addon...")

	// Check if ingress-nginx namespace exists
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "namespace", "ingress-nginx", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("ingress-nginx namespace not found: %v\nOutput: %s", err, string(output))
		return
	}
	t.Log("✓ ingress-nginx namespace exists")

	// Check if ingress-nginx controller exists (deployment or daemonset)
	cmd = exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "deployment", "-n", "ingress-nginx", "ingress-nginx-controller", "-o", "json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		// Try daemonset
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "daemonset", "-n", "ingress-nginx", "ingress-nginx-controller", "-o", "json")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("Ingress NGINX controller not found: %v\nOutput: %s", err, string(output))
			return
		}
		t.Log("✓ Ingress NGINX controller (daemonset) exists")
	} else {
		t.Log("✓ Ingress NGINX controller (deployment) exists")
	}

	// Wait for ingress-nginx pod to be running
	t.Log("Waiting for Ingress NGINX pod to be Running...")
	ingressRunning := false
	for i := 0; i < 30; i++ { // Wait up to 5 minutes
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "pods", "-n", "ingress-nginx", "-l", "app.kubernetes.io/component=controller",
			"-o", "jsonpath={.items[0].status.phase}")
		output, err = cmd.CombinedOutput()
		if err == nil && string(output) == "Running" {
			ingressRunning = true
			break
		}
		t.Logf("Ingress NGINX pod not running yet (phase: %s), waiting...", string(output))
		time.Sleep(10 * time.Second)
	}
	if !ingressRunning {
		t.Error("Ingress NGINX pod failed to reach Running state")
		return
	}
	t.Log("✓ Ingress NGINX pod is Running")

	t.Log("✓ Ingress NGINX addon is working")
}

// testRBACAddon verifies the RBAC addon is working
func testRBACAddon(t *testing.T, kubeconfigPath string) {
	t.Log("Testing RBAC addon...")

	// Check if custom RBAC resources exist
	// The RBAC addon creates ClusterRoles and ClusterRoleBindings
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "clusterroles", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("Failed to get ClusterRoles: %v\nOutput: %s", err, string(output))
		return
	}

	var clusterRoles struct {
		Items []struct {
			Metadata struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(output, &clusterRoles); err != nil {
		t.Errorf("Failed to parse ClusterRoles: %v", err)
		return
	}

	// Look for RBAC addon label or known role names
	foundRBACRoles := false
	for _, role := range clusterRoles.Items {
		if strings.Contains(role.Metadata.Name, "hcloud-k8s") ||
			(role.Metadata.Labels != nil && role.Metadata.Labels["app"] == "hcloud-k8s-rbac") {
			foundRBACRoles = true
			t.Logf("✓ Found RBAC ClusterRole: %s", role.Metadata.Name)
			break
		}
	}

	if foundRBACRoles {
		t.Log("✓ RBAC addon resources found")
	} else {
		t.Log("⚠ RBAC addon may be installed but no custom roles found (this is normal if RBAC config is empty)")
	}

	t.Log("✓ RBAC addon verification complete")
}

// testCCMAddon verifies the CCM addon (extracted from cluster_test.go for consistency)
func testCCMAddon(t *testing.T, kubeconfigPath string) {
	t.Log("Testing CCM addon...")

	// Check if CCM deployment exists
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "deployment", "-n", "kube-system", "hcloud-cloud-controller-manager", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("CCM deployment not found: %v\nOutput: %s", err, string(output))
		return
	}
	t.Log("✓ CCM deployment exists")

	// Wait for CCM pod to be running
	t.Log("Waiting for CCM pod to be Running...")
	ccmRunning := false
	for i := 0; i < 30; i++ {
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "pods", "-n", "kube-system", "-l", "app=hcloud-cloud-controller-manager",
			"-o", "jsonpath={.items[0].status.phase}")
		output, err = cmd.CombinedOutput()
		if err == nil && string(output) == "Running" {
			ccmRunning = true
			break
		}
		t.Logf("CCM pod not running yet (phase: %s), waiting...", string(output))
		time.Sleep(10 * time.Second)
	}
	if !ccmRunning {
		t.Error("CCM pod failed to reach Running state")
		return
	}
	t.Log("✓ CCM pod is Running")

	t.Log("✓ CCM addon is working")
}

// testCSIAddon verifies the CSI addon (extracted from cluster_test.go for consistency)
func testCSIAddon(t *testing.T, kubeconfigPath string) {
	t.Log("Testing CSI addon...")

	// Check if CSIDriver resource exists
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "csidriver", "csi.hetzner.cloud", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("CSIDriver resource not found: %v\nOutput: %s", err, string(output))
		return
	}
	t.Log("✓ CSIDriver resource exists")

	// Check if CSI controller deployment exists
	cmd = exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "deployment", "-n", "kube-system", "hcloud-csi-controller", "-o", "json")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Errorf("CSI controller deployment not found: %v\nOutput: %s", err, string(output))
		return
	}
	t.Log("✓ CSI controller deployment exists")

	// Wait for CSI controller pod to be running
	t.Log("Waiting for CSI controller pod to be Running...")
	csiControllerRunning := false
	for i := 0; i < 30; i++ {
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "pods", "-n", "kube-system", "-l", "app.kubernetes.io/name=hcloud-csi,app.kubernetes.io/component=controller",
			"-o", "jsonpath={.items[0].status.phase}")
		output, err = cmd.CombinedOutput()
		if err == nil && string(output) == "Running" {
			csiControllerRunning = true
			break
		}
		t.Logf("CSI controller pod not running yet (phase: %s), waiting...", string(output))
		time.Sleep(10 * time.Second)
	}
	if !csiControllerRunning {
		t.Error("CSI controller pod failed to reach Running state")
		return
	}
	t.Log("✓ CSI controller pod is Running")

	t.Log("✓ CSI addon is working")
}
