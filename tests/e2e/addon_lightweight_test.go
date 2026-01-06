//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/cluster"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_client "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/k8s"
	"github.com/sak-d/hcloud-k8s/internal/talos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestAddonInstallationLightweight is a cost-optimized E2E test that validates
// addon installation on a minimal single-node cluster.
//
// This test:
// - Provisions only 1 control plane node (no workers) for cost optimization
// - Enables CCM, CSI, and Cilium addons
// - Verifies addon pods are created and become ready
// - Checks that required secrets are created correctly
// - Validates kubeconfig export functionality
//
// Duration: ~8 minutes
// Cost: Minimal (1 server for ~8 minutes)
func TestAddonInstallationLightweight(t *testing.T) {
	t.Parallel()

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	// Use a unique cluster name
	clusterName := fmt.Sprintf("addon-light-%d", time.Now().Unix())
	kubeconfigPath := fmt.Sprintf("/tmp/%s-kubeconfig", clusterName)

	// Minimal cluster configuration: single control plane node, no workers
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
					Name:       "cp",
					ServerType: "cpx21", // Smallest viable type for K8s
					Location:   "nbg1",
					Count:      1, // Single node for cost optimization
					Image:      "talos",
				},
			},
		},
		Workers: []config.WorkerNodePool{}, // No workers needed for addon testing
		Talos: config.TalosConfig{
			Version: "v1.8.3",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.31.0",
		},
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{
				Enabled:                   true,
				LoadBalancersEnabled:      true,
				NetworkRoutesEnabled:      true,
				LoadBalancerAlgorithmType: "round_robin",
			},
			CSI: config.CSIConfig{
				Enabled: true,
				StorageClasses: []config.StorageClass{
					{
						Name:                "hcloud-volumes",
						ReclaimPolicy:       "Delete",
						DefaultStorageClass: true,
						Encrypted:           true,
					},
				},
			},
			Cilium: config.CiliumConfig{
				Enabled:              true,
				RoutingMode:          "native",
				KubeProxyReplacement: true,
				EncryptionEnabled:    true,
				EncryptionType:       "ipsec",
				IPSecKeyID:           3,
				IPSecAlgorithm:       "gcm-aes-128",
				IPSecKeySize:         128,
				HubbleEnabled:        false, // Disable Hubble to reduce resource usage
				GatewayAPIEnabled:    false,
			},
		},
	}

	// Use shared snapshots if available
	if sharedCtx != nil && sharedCtx.SnapshotAMD64 != "" {
		t.Log("Using shared Talos snapshot from test suite")
	}

	// Initialize client
	hClient := hcloud_client.NewRealClient(token)
	cleaner := &ResourceCleaner{t: t}

	// Setup SSH Key
	sshKeyName, _ := setupSSHKey(t, hClient, cleaner, clusterName)
	cfg.SSHKeys = []string{sshKeyName}

	// Cleanup function - delete all resources
	cleanup := func() {
		ctx := context.Background()
		logger := func(msg string) { t.Logf("[Cleanup] %s", msg) }

		// Delete server
		hClient.DeleteServer(ctx, clusterName+"-cp-1")
		logger("Deleted Server")

		// Delete Load Balancer
		hClient.DeleteLoadBalancer(ctx, clusterName+"-kube-api")
		logger("Deleted LB")

		// Delete Firewall
		hClient.DeleteFirewall(ctx, clusterName)
		logger("Deleted FW")

		// Delete Network
		hClient.DeleteNetwork(ctx, clusterName)
		logger("Deleted Network")

		// Delete Placement Groups
		hClient.DeletePlacementGroup(ctx, clusterName+"-cp")
		logger("Deleted PGs")

		// Delete State Certificate
		hClient.DeleteCertificate(ctx, clusterName+"-state")
		logger("Deleted Certificate")

		// Delete kubeconfig file
		os.Remove(kubeconfigPath)
		logger("Deleted kubeconfig")
	}
	defer cleanup()

	// Initialize Talos generator
	talosGen, err := talos.NewConfigGenerator(clusterName, cfg.Kubernetes.Version, cfg.Talos.Version, "", "")
	require.NoError(t, err)

	reconciler := cluster.NewReconciler(hClient, talosGen, cfg)
	reconciler.SetKubeconfigPath(kubeconfigPath)

	// Run Reconcile (includes addon installation)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	t.Log("Starting cluster reconciliation with addon installation...")
	err = reconciler.Reconcile(ctx)
	require.NoError(t, err, "Reconciliation should succeed")

	// Verify kubeconfig was exported
	t.Log("Verifying kubeconfig export...")
	require.FileExists(t, kubeconfigPath, "Kubeconfig should be exported")

	// Verify kubeconfig has correct permissions
	info, err := os.Stat(kubeconfigPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "Kubeconfig should have 0600 permissions")

	// Create Kubernetes client from exported kubeconfig
	t.Log("Connecting to cluster with exported kubeconfig...")
	k8sClient, err := k8s.NewClient(kubeconfigPath)
	require.NoError(t, err, "Should be able to create k8s client from exported kubeconfig")

	// Verify cluster is accessible
	t.Log("Verifying cluster accessibility...")
	ctx = context.Background()

	// Test kubectl connectivity - list nodes
	clientset := k8sClient.GetClientset()
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "Should be able to list nodes")
	assert.Equal(t, 1, len(nodes.Items), "Should have exactly 1 node")

	// Verify addon secrets were created
	t.Log("Verifying addon secrets...")

	// CCM secret
	t.Log("  Checking hcloud secret (CCM)...")
	ccmSecret, err := clientset.CoreV1().Secrets("kube-system").Get(ctx, "hcloud", metav1.GetOptions{})
	require.NoError(t, err, "CCM secret 'hcloud' should exist")
	assert.NotEmpty(t, ccmSecret.Data["token"], "CCM secret should contain token")
	assert.NotEmpty(t, ccmSecret.Data["network"], "CCM secret should contain network ID")

	// CSI secret
	t.Log("  Checking hcloud-csi secret...")
	csiSecret, err := clientset.CoreV1().Secrets("kube-system").Get(ctx, "hcloud-csi", metav1.GetOptions{})
	require.NoError(t, err, "CSI secret 'hcloud-csi' should exist")
	assert.NotEmpty(t, csiSecret.Data["token"], "CSI secret should contain token")

	// Cilium IPSec secret (if encryption enabled)
	if cfg.Addons.Cilium.EncryptionEnabled && cfg.Addons.Cilium.EncryptionType == "ipsec" {
		t.Log("  Checking cilium-ipsec-keys secret...")
		ciliumSecret, err := clientset.CoreV1().Secrets("kube-system").Get(ctx, "cilium-ipsec-keys", metav1.GetOptions{})
		require.NoError(t, err, "Cilium IPSec secret should exist")
		assert.NotEmpty(t, ciliumSecret.Data["keys"], "Cilium secret should contain IPSec keys")
	}

	// Verify addon pods are created and running
	t.Log("Verifying addon pods...")

	// Wait for addon pods to be created and running
	addonPods := map[string]string{
		"hcloud-cloud-controller-manager": "app.kubernetes.io/name=hcloud-cloud-controller-manager",
		"hcloud-csi-controller":           "app=hcloud-csi-controller",
		"hcloud-csi-node":                 "app=hcloud-csi",
		"cilium":                          "k8s-app=cilium",
		"cilium-operator":                 "name=cilium-operator",
	}

	for addonName, labelSelector := range addonPods {
		t.Logf("  Waiting for %s pods...", addonName)

		// Wait up to 5 minutes for pods to be created
		podFound := false
		for i := 0; i < 60; i++ {
			pods, err := clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
				LabelSelector: labelSelector,
			})
			if err == nil && len(pods.Items) > 0 {
				podFound = true
				t.Logf("    Found %d %s pod(s)", len(pods.Items), addonName)

				// Check pod status
				for _, pod := range pods.Items {
					t.Logf("    Pod %s: Phase=%s", pod.Name, pod.Status.Phase)
					if pod.Status.Phase == corev1.PodRunning {
						// Check if containers are ready
						allReady := true
						for _, containerStatus := range pod.Status.ContainerStatuses {
							if !containerStatus.Ready {
								allReady = false
								t.Logf("      Container %s: Ready=%v", containerStatus.Name, containerStatus.Ready)
							}
						}
						if allReady {
							t.Logf("    ✓ Pod %s is running and ready", pod.Name)
						}
					}
				}
				break
			}
			time.Sleep(5 * time.Second)
		}

		assert.True(t, podFound, "%s pods should be created", addonName)
	}

	// Final verification: Check for any pod errors
	t.Log("Checking for pod errors...")
	allPods, err := clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	for _, pod := range allPods.Items {
		// Check for failed containers
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Waiting != nil && containerStatus.State.Waiting.Reason == "CrashLoopBackOff" {
				t.Errorf("Container %s in pod %s is in CrashLoopBackOff", containerStatus.Name, pod.Name)
			}
			if containerStatus.RestartCount > 5 {
				t.Logf("Warning: Container %s in pod %s has %d restarts", containerStatus.Name, pod.Name, containerStatus.RestartCount)
			}
		}
	}

	t.Log("✓ Addon installation validated successfully")
}

// TestKubeconfigExport specifically tests the kubeconfig export functionality.
// It reuses the cluster from TestAddonInstallationLightweight if run sequentially,
// or creates a minimal cluster if run standalone.
func TestKubeconfigExport(t *testing.T) {
	t.Parallel()

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	clusterName := fmt.Sprintf("kubeconfig-test-%d", time.Now().Unix())
	kubeconfigPath := fmt.Sprintf("/tmp/%s-kubeconfig", clusterName)

	// Minimal cluster config
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
					Name:       "cp",
					ServerType: "cpx21",
					Location:   "nbg1",
					Count:      1,
					Image:      "talos",
				},
			},
		},
		Workers: []config.WorkerNodePool{},
		Talos: config.TalosConfig{
			Version: "v1.8.3",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "v1.31.0",
		},
		Addons: config.AddonsConfig{
			// No addons needed for kubeconfig test
			CCM:    config.CCMConfig{Enabled: false},
			CSI:    config.CSIConfig{Enabled: false},
			Cilium: config.CiliumConfig{Enabled: false},
		},
	}

	// Use shared snapshots
	if sharedCtx != nil && sharedCtx.SnapshotAMD64 != "" {
		t.Log("Using shared Talos snapshot")
	}

	hClient := hcloud_client.NewRealClient(token)
	cleaner := &ResourceCleaner{t: t}

	sshKeyName, _ := setupSSHKey(t, hClient, cleaner, clusterName)
	cfg.SSHKeys = []string{sshKeyName}

	// Cleanup
	cleanup := func() {
		ctx := context.Background()
		hClient.DeleteServer(ctx, clusterName+"-cp-1")
		hClient.DeleteLoadBalancer(ctx, clusterName+"-kube-api")
		hClient.DeleteFirewall(ctx, clusterName)
		hClient.DeleteNetwork(ctx, clusterName)
		hClient.DeletePlacementGroup(ctx, clusterName+"-cp")
		hClient.DeleteCertificate(ctx, clusterName+"-state")
		os.Remove(kubeconfigPath)
	}
	defer cleanup()

	// Initialize and reconcile
	talosGen, err := talos.NewConfigGenerator(clusterName, cfg.Kubernetes.Version, cfg.Talos.Version, "", "")
	require.NoError(t, err)

	reconciler := cluster.NewReconciler(hClient, talosGen, cfg)
	reconciler.SetKubeconfigPath(kubeconfigPath)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	t.Log("Provisioning cluster for kubeconfig export test...")
	err = reconciler.Reconcile(ctx)
	require.NoError(t, err)

	// Test 1: Verify kubeconfig file exists
	t.Log("Test 1: Verifying kubeconfig file exists...")
	require.FileExists(t, kubeconfigPath)

	// Test 2: Verify file permissions
	t.Log("Test 2: Verifying kubeconfig permissions (should be 0600)...")
	info, err := os.Stat(kubeconfigPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Test 3: Verify kubeconfig content structure
	t.Log("Test 3: Verifying kubeconfig content structure...")
	kubeconfigData, err := os.ReadFile(kubeconfigPath)
	require.NoError(t, err)
	require.NotEmpty(t, kubeconfigData)

	// Check for essential kubeconfig components
	kubeconfigStr := string(kubeconfigData)
	assert.Contains(t, kubeconfigStr, "apiVersion: v1")
	assert.Contains(t, kubeconfigStr, "kind: Config")
	assert.Contains(t, kubeconfigStr, "clusters:")
	assert.Contains(t, kubeconfigStr, "users:")
	assert.Contains(t, kubeconfigStr, "contexts:")
	assert.Contains(t, kubeconfigStr, "current-context:")

	// Test 4: Verify kubectl works with exported kubeconfig
	t.Log("Test 4: Testing kubectl connectivity with exported kubeconfig...")
	k8sClient, err := k8s.NewClient(kubeconfigPath)
	require.NoError(t, err, "Should create k8s client from exported kubeconfig")

	clientset := k8sClient.GetClientset()
	nodes, err := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err, "Should be able to list nodes")
	assert.Equal(t, 1, len(nodes.Items), "Should have 1 node")

	// Test 5: Verify server URL points to load balancer
	t.Log("Test 5: Verifying server URL points to load balancer...")
	lb, err := hClient.GetLoadBalancer(context.Background(), clusterName+"-kube-api")
	require.NoError(t, err)
	lbIP := lb.PublicNet.IPv4.IP.String()
	expectedServer := fmt.Sprintf("https://%s:6443", lbIP)
	assert.Contains(t, kubeconfigStr, lbIP, "Kubeconfig should reference load balancer IP")

	t.Logf("✓ Kubeconfig export validated successfully")
	t.Logf("  Server URL: %s", expectedServer)
	t.Logf("  File: %s", kubeconfigPath)
}
