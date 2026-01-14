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

	"hcloud-k8s/internal/addons"
	"hcloud-k8s/internal/config"
)

// phaseAddons installs and tests addons sequentially.
// This is Phase 3 of the E2E lifecycle.
// Tests are run sequentially to avoid API rate limiting and resource contention.
func phaseAddons(t *testing.T, state *E2EState) {
	t.Log("Testing addons sequentially...")

	// Get HCloud token for addons
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Fatal("HCLOUD_TOKEN required for addon testing")
	}

	// Core addons (required for basic functionality)
	t.Run("CCM", func(t *testing.T) { testAddonCCM(t, state, token) })
	t.Run("CSI", func(t *testing.T) { testAddonCSI(t, state, token) })
	t.Run("MetricsServer", func(t *testing.T) { testAddonMetricsServer(t, state) })

	// Optional addons
	t.Run("CertManager", func(t *testing.T) { testAddonCertManager(t, state) })
	t.Run("IngressNginx", func(t *testing.T) { testAddonIngressNginx(t, state) })
	t.Run("RBAC", func(t *testing.T) { testAddonRBAC(t, state) })
	t.Run("Longhorn", func(t *testing.T) { testAddonLonghorn(t, state) })

	t.Log("✓ Phase 3: Addons (all tested)")
}

// testAddonCCM installs and tests the Hetzner Cloud Controller Manager.
func testAddonCCM(t *testing.T, state *E2EState, token string) {
	t.Log("Installing CCM addon...")

	ctx := context.Background()
	cfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: token,
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{Enabled: true},
		},
	}

	network, _ := state.Client.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	if err := addons.Apply(ctx, cfg, state.Kubeconfig, networkID); err != nil {
		t.Fatalf("Failed to install CCM: %v", err)
	}

	// Wait for CCM pod
	waitForPod(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=hcloud-cloud-controller-manager", 5*time.Minute)

	// Verify provider IDs are set
	verifyProviderIDs(t, state.KubeconfigPath, 2*time.Minute)

	// Test LB provisioning
	testCCMLoadBalancer(t, state)

	state.AddonsInstalled["ccm"] = true
	t.Log("✓ CCM addon working")
}

// testAddonCSI installs and tests the Hetzner Cloud CSI Driver.
func testAddonCSI(t *testing.T, state *E2EState, token string) {
	t.Log("Installing CSI addon...")

	ctx := context.Background()
	cfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: token,
		Addons: config.AddonsConfig{
			CSI: config.CSIConfig{
				Enabled:             true,
				DefaultStorageClass: true,
			},
		},
	}

	network, _ := state.Client.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	if err := addons.Apply(ctx, cfg, state.Kubeconfig, networkID); err != nil {
		t.Fatalf("Failed to install CSI: %v", err)
	}

	// Wait for CSI controller
	waitForPod(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=hcloud-csi,app.kubernetes.io/component=controller", 5*time.Minute)

	// Verify StorageClass exists
	verifyStorageClass(t, state.KubeconfigPath)

	// Test volume lifecycle
	testCSIVolume(t, state)

	state.AddonsInstalled["csi"] = true
	t.Log("✓ CSI addon working")
}

// testAddonMetricsServer installs and tests the Metrics Server.
func testAddonMetricsServer(t *testing.T, state *E2EState) {
	t.Log("Installing Metrics Server addon...")

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		Addons: config.AddonsConfig{
			MetricsServer: config.MetricsServerConfig{Enabled: true},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0); err != nil {
		t.Fatalf("Failed to install Metrics Server: %v", err)
	}

	// Wait for metrics-server pod
	waitForPod(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=metrics-server", 5*time.Minute)

	// Test metrics API
	testMetricsAPI(t, state.KubeconfigPath)

	state.AddonsInstalled["metrics-server"] = true
	t.Log("✓ Metrics Server addon working")
}

// testAddonCertManager installs and tests Cert Manager.
func testAddonCertManager(t *testing.T, state *E2EState) {
	t.Log("Installing Cert Manager addon...")

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		Addons: config.AddonsConfig{
			CertManager: config.CertManagerConfig{Enabled: true},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0); err != nil {
		t.Fatalf("Failed to install Cert Manager: %v", err)
	}

	// Wait for cert-manager pods
	waitForPod(t, state.KubeconfigPath, "cert-manager", "app.kubernetes.io/component=controller", 5*time.Minute)

	// Verify CRDs exist
	verifyCRDExists(t, state.KubeconfigPath, "certificates.cert-manager.io")

	state.AddonsInstalled["cert-manager"] = true
	t.Log("✓ Cert Manager addon working")
}

// testAddonIngressNginx installs and tests Ingress NGINX.
func testAddonIngressNginx(t *testing.T, state *E2EState) {
	t.Log("Installing Ingress NGINX addon...")

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		Addons: config.AddonsConfig{
			IngressNginx: config.IngressNginxConfig{Enabled: true},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0); err != nil {
		t.Fatalf("Failed to install Ingress NGINX: %v", err)
	}

	// Wait for ingress controller pod
	waitForPod(t, state.KubeconfigPath, "ingress-nginx", "app.kubernetes.io/component=controller", 5*time.Minute)

	state.AddonsInstalled["ingress-nginx"] = true
	t.Log("✓ Ingress NGINX addon working")
}

// testAddonRBAC installs and tests RBAC addon.
func testAddonRBAC(t *testing.T, state *E2EState) {
	t.Log("Installing RBAC addon...")

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		Addons: config.AddonsConfig{
			RBAC: config.RBACConfig{Enabled: true},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0); err != nil {
		t.Fatalf("Failed to install RBAC: %v", err)
	}

	state.AddonsInstalled["rbac"] = true
	t.Log("✓ RBAC addon working")
}

// testAddonLonghorn installs and tests Longhorn (slow, so it's last).
func testAddonLonghorn(t *testing.T, state *E2EState) {
	t.Log("Installing Longhorn addon (this may take several minutes)...")

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		Addons: config.AddonsConfig{
			Longhorn: config.LonghornConfig{Enabled: true},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0); err != nil {
		t.Fatalf("Failed to install Longhorn: %v", err)
	}

	// Wait for longhorn pods (this takes a while)
	waitForPod(t, state.KubeconfigPath, "longhorn-system", "app=longhorn-manager", 10*time.Minute)

	state.AddonsInstalled["longhorn"] = true
	t.Log("✓ Longhorn addon working")
}

// Addon test helper functions

func waitForPod(t *testing.T, kubeconfigPath, namespace, selector string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for pod with selector %s in namespace %s", selector, namespace)
		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "pods", "-n", namespace, "-l", selector,
				"-o", "jsonpath={.items[0].status.phase}")
			output, err := cmd.CombinedOutput()
			if err == nil && string(output) == "Running" {
				t.Logf("  ✓ Pod %s is Running", selector)
				return
			}
			t.Logf("  Waiting for pod (phase: %s)...", string(output))
		}
	}
}

func verifyProviderIDs(t *testing.T, kubeconfigPath string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("Timeout waiting for provider IDs to be set")
		case <-ticker.C:
			cmd := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "nodes", "-o", "json")
			output, err := cmd.CombinedOutput()
			if err != nil {
				continue
			}

			var nodes struct {
				Items []struct {
					Spec struct {
						ProviderID string `json:"providerID"`
					} `json:"spec"`
				} `json:"items"`
			}
			if json.Unmarshal(output, &nodes) == nil {
				allSet := true
				for _, node := range nodes.Items {
					if !strings.HasPrefix(node.Spec.ProviderID, "hcloud://") {
						allSet = false
						break
					}
				}
				if allSet && len(nodes.Items) > 0 {
					t.Log("  ✓ All nodes have provider IDs")
					return
				}
			}
		}
	}
}

func verifyStorageClass(t *testing.T, kubeconfigPath string) {
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "storageclass", "hcloud-volumes")
	if err := cmd.Run(); err != nil {
		t.Fatal("StorageClass hcloud-volumes not found")
	}
	t.Log("  ✓ StorageClass hcloud-volumes exists")
}

func verifyCRDExists(t *testing.T, kubeconfigPath, crdName string) {
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "crd", crdName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("CRD %s not found", crdName)
	}
	t.Logf("  ✓ CRD %s exists", crdName)
}

func testMetricsAPI(t *testing.T, kubeconfigPath string) {
	// Wait up to 2 minutes for metrics to be available
	for i := 0; i < 12; i++ {
		cmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"top", "nodes")
		if err := cmd.Run(); err == nil {
			t.Log("  ✓ Metrics API working")
			return
		}
		time.Sleep(10 * time.Second)
	}
	t.Fatal("Metrics API not available after 2 minutes")
}

// testCCMLoadBalancer tests CCM's ability to provision load balancers.
func testCCMLoadBalancer(t *testing.T, state *E2EState) {
	t.Log("  Testing CCM LB provisioning...")

	// Create test deployment and service (extracted to helper for brevity)
	testLBName := "e2e-ccm-test-lb"
	manifest := fmt.Sprintf(`
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: default
spec:
  type: LoadBalancer
  ports:
    - port: 80
  selector:
    app: nonexistent
`, testLBName)

	// Apply manifest
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create test LB service: %v\nOutput: %s", err, string(output))
	}

	// Wait for external IP (max 3 minutes)
	externalIP := ""
	maxAttempts := 36 // 36 * 5s = 3 minutes
	for i := 0; i < maxAttempts; i++ {
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", state.KubeconfigPath,
			"get", "svc", testLBName, "-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
		output, err := cmd.CombinedOutput()
		if err == nil && len(output) > 0 {
			externalIP = string(output)
			break
		}

		// Log progress every 30 seconds
		if i > 0 && i%6 == 0 {
			elapsed := i * 5
			t.Logf("  [%ds] Waiting for LB external IP...", elapsed)

			// Show service description for debugging
			if i == 12 { // At 1 minute mark
				descCmd := exec.CommandContext(context.Background(), "kubectl",
					"--kubeconfig", state.KubeconfigPath,
					"describe", "svc", testLBName)
				if descOutput, _ := descCmd.CombinedOutput(); len(descOutput) > 0 {
					t.Logf("  Service status at %ds:\n%s", elapsed, string(descOutput))
				}
			}
		}

		time.Sleep(5 * time.Second)
	}

	if externalIP == "" {
		// Gather diagnostic info before failing
		t.Log("  CCM failed to provision load balancer - gathering diagnostics...")

		// Show final service status
		descCmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", state.KubeconfigPath,
			"describe", "svc", testLBName)
		if descOutput, _ := descCmd.CombinedOutput(); len(descOutput) > 0 {
			t.Logf("  Final service status:\n%s", string(descOutput))
		}

		// Show CCM logs
		logsCmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", state.KubeconfigPath,
			"logs", "-n", "kube-system", "-l", "app.kubernetes.io/name=hcloud-cloud-controller-manager",
			"--tail=50")
		if logsOutput, _ := logsCmd.CombinedOutput(); len(logsOutput) > 0 {
			t.Logf("  CCM logs (last 50 lines):\n%s", string(logsOutput))
		}

		t.Fatal("  CCM failed to provision load balancer")
	}

	t.Logf("  ✓ CCM provisioned LB with IP: %s", externalIP)

	// Cleanup
	exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"delete", "svc", testLBName).Run()

	// Wait for LB deletion
	time.Sleep(30 * time.Second)
	t.Log("  ✓ CCM LB test complete")
}

// testCSIVolume tests CSI's ability to provision and mount volumes.
func testCSIVolume(t *testing.T, state *E2EState) {
	t.Log("  Testing CSI volume provisioning...")

	pvcName := "e2e-csi-test-pvc"
	podName := "e2e-csi-test-pod"

	// Create PVC
	pvcManifest := fmt.Sprintf(`
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: %s
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 10Gi
  storageClassName: hcloud-volumes
`, pvcName)

	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"apply", "-f", "-")
	cmd.Stdin = strings.NewReader(pvcManifest)
	if err := cmd.Run(); err != nil {
		t.Fatal("  Failed to create PVC")
	}

	// Create pod to trigger binding
	podManifest := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: %s
spec:
  containers:
  - name: test
    image: busybox:latest
    command: ["sleep", "3600"]
    volumeMounts:
    - name: test-volume
      mountPath: /data
  volumes:
  - name: test-volume
    persistentVolumeClaim:
      claimName: %s
  tolerations:
  - operator: Exists
`, podName, pvcName)

	cmd = exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"apply", "-f", "-")
	cmd.Stdin = strings.NewReader(podManifest)
	if err := cmd.Run(); err != nil {
		t.Fatal("  Failed to create test pod")
	}

	// Wait for pod to be running (confirms volume attached)
	for i := 0; i < 36; i++ {
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", state.KubeconfigPath,
			"get", "pod", podName, "-o", "jsonpath={.status.phase}")
		output, _ := cmd.CombinedOutput()
		if string(output) == "Running" {
			t.Log("  ✓ CSI volume provisioned and mounted")
			break
		}
		if i == 35 {
			t.Fatal("  Timeout waiting for pod with CSI volume")
		}
		time.Sleep(5 * time.Second)
	}

	// Cleanup
	exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"delete", "pod", podName, "--force", "--grace-period=0").Run()
	exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"delete", "pvc", pvcName).Run()

	time.Sleep(10 * time.Second)
	t.Log("  ✓ CSI volume test complete")
}
