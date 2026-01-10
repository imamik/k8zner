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

// TestClusterProvisioning is an end-to-end test that provisions a cluster and verifies resources.
// It requires HCLOUD_TOKEN to be set.
func TestClusterProvisioning(t *testing.T) {
	t.Parallel() // Run in parallel with other tests

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	// Use a unique cluster name to avoid conflicts
	clusterName := fmt.Sprintf("e2e-test-%d", time.Now().Unix())

	// Create a minimal config
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
		},
	}

	// Use shared snapshots if available (built in TestMain)
	if sharedCtx != nil && sharedCtx.SnapshotAMD64 != "" {
		t.Log("Using shared Talos snapshot from test suite")
		// Snapshots will be used automatically via auto-build feature
		// The reconciler will find existing snapshots with matching labels
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
	// Use real Talos generator for a "real" E2E run
	secrets, err := talos.GetOrGenerateSecrets("/tmp/talos-secrets-"+clusterName+".json", cfg.Talos.Version)
	assert.NoError(t, err)
	defer os.Remove("/tmp/talos-secrets-" + clusterName + ".json")

	talosGen := talos.NewGenerator(clusterName, cfg.Kubernetes.Version, cfg.Talos.Version, "", secrets)

	reconciler := orchestration.NewReconciler(hClient, talosGen, cfg)

	// Run Reconcile
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	kubeconfig, err := reconciler.Reconcile(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, kubeconfig, "kubeconfig should be returned after bootstrap")

	// Verify APIs are reachable
	// We check for Talos API on one of the servers
	cp1IP, err := hClient.GetServerIP(ctx, clusterName+"-control-plane-1")
	require.NoError(t, err)
	require.NotEmpty(t, cp1IP)

	// Kube API through Load Balancer
	lb, err := hClient.GetLoadBalancer(ctx, clusterName+"-kube-api")
	require.NoError(t, err)
	require.NotNil(t, lb)
	lbIP := lb.PublicNet.IPv4.IP.String()
	require.NotEmpty(t, lbIP)

	t.Logf("Verifying APIs: Talos=%s:50000, Kube=%s:6443", cp1IP, lbIP)

	// Connectivity Check: Talos API
	if err := WaitForPort(context.Background(), cp1IP, 50000, 2*time.Minute); err != nil {
		t.Errorf("Talos API not reachable: %v", err)
	} else {
		t.Log("Talos API is reachable!")
	}

	// Connectivity Check: Kube API
	if err := WaitForPort(context.Background(), lbIP, 6443, 10*time.Minute); err != nil {
		t.Errorf("Kube API not reachable: %v", err)
	} else {
		t.Log("Kube API is reachable!")
	}

	// Verify cluster with kubectl
	t.Log("Verifying cluster with kubectl...")
	kubeconfigPath := fmt.Sprintf("/tmp/kubeconfig-%s", clusterName)
	if err := os.WriteFile(kubeconfigPath, kubeconfig, 0600); err != nil {
		t.Errorf("Failed to write kubeconfig: %v", err)
	} else {
		defer os.Remove(kubeconfigPath)

		// Try kubectl get nodes (with retries as cluster might still be initializing)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		nodeCheckSuccess := false
		for {
			select {
			case <-ctx.Done():
				if !nodeCheckSuccess {
					t.Error("Timeout waiting for kubectl get nodes to succeed")
				}
				goto endNodeCheck
			case <-ticker.C:
				// Run kubectl get nodes
				cmd := exec.CommandContext(context.Background(), "kubectl",
					"--kubeconfig", kubeconfigPath,
					"get", "nodes", "-o", "json")
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Logf("kubectl get nodes not ready yet: %v (will retry)", err)
					continue
				}

				// Parse output to count nodes
				var nodeList struct {
					Items []map[string]interface{} `json:"items"`
				}
				if err := json.Unmarshal(output, &nodeList); err != nil {
					t.Logf("Failed to parse kubectl output: %v (will retry)", err)
					continue
				}

				if len(nodeList.Items) >= 2 { // 1 control plane + 1 worker
					t.Logf("✓ kubectl verified: %d nodes found", len(nodeList.Items))
					nodeCheckSuccess = true
					goto endNodeCheck
				}
				t.Logf("Waiting for nodes to appear... (found %d, expecting >= 2)", len(nodeList.Items))
			}
		}
	endNodeCheck:

		// Verify CCM is installed and running
		t.Log("Verifying Hetzner Cloud Controller Manager (CCM) installation...")

		// Check if CCM deployment exists
		cmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "deployment", "-n", "kube-system", "hcloud-cloud-controller-manager", "-o", "json")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("CCM deployment not found: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ CCM deployment exists")
		}

		// Check if hcloud secret exists
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "secret", "-n", "kube-system", "hcloud", "-o", "json")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("hcloud secret not found: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ hcloud secret exists")

			// Verify secret contains required keys
			var secret struct {
				Data map[string]string `json:"data"`
			}
			if err := json.Unmarshal(output, &secret); err != nil {
				t.Errorf("Failed to parse secret: %v", err)
			} else {
				if _, ok := secret.Data["token"]; !ok {
					t.Error("hcloud secret missing 'token' key")
				}
				if _, ok := secret.Data["network"]; !ok {
					t.Error("hcloud secret missing 'network' key")
				}
				if len(secret.Data) >= 2 {
					t.Log("✓ hcloud secret contains required keys")
				}
			}
		}

		// Verify CCM functional checks
		t.Log("Verifying CCM functionality...")

		// Wait for CCM pod to be running
		t.Log("Waiting for CCM pod to be Running...")
		ccmRunning := false
		for i := 0; i < 30; i++ { // Wait up to 5 minutes
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
		} else {
			t.Log("✓ CCM pod is Running")
		}

		// Check CCM logs for successful initialization
		t.Log("Checking CCM logs for successful initialization...")
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"logs", "-n", "kube-system", "-l", "app=hcloud-cloud-controller-manager", "--tail=100")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("Failed to get CCM logs: %v", err)
		} else {
			logs := string(output)
			if strings.Contains(logs, "Started") || strings.Contains(logs, "successfully") {
				t.Log("✓ CCM initialized successfully")
			}
			// Check for errors in logs
			if strings.Contains(logs, "Error") || strings.Contains(logs, "Failed") {
				t.Logf("Warning: CCM logs contain errors:\n%s", logs)
			}
		}

		// Verify nodes have cloud provider IDs (providerID)
		// CCM needs time to discover nodes and set providerIDs, so retry for up to 2 minutes
		t.Log("Verifying nodes have cloud provider IDs (will wait for CCM to set them)...")
		providerIDsSet := false
		for i := 0; i < 24; i++ { // Wait up to 2 minutes (24 * 5 seconds)
			cmd = exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "nodes", "-o", "json")
			output, err = cmd.CombinedOutput()
			if err != nil {
				t.Logf("Attempt %d: Failed to get nodes: %v (will retry)", i+1, err)
				time.Sleep(5 * time.Second)
				continue
			}

			var nodeList struct {
				Items []struct {
					Metadata struct {
						Name   string            `json:"name"`
						Labels map[string]string `json:"labels"`
					} `json:"metadata"`
					Spec struct {
						ProviderID string `json:"providerID"`
					} `json:"spec"`
				} `json:"items"`
			}
			if err := json.Unmarshal(output, &nodeList); err != nil {
				t.Logf("Attempt %d: Failed to parse nodes: %v (will retry)", i+1, err)
				time.Sleep(5 * time.Second)
				continue
			}

			allNodesHaveProviderID := true
			for _, node := range nodeList.Items {
				if node.Spec.ProviderID == "" {
					allNodesHaveProviderID = false
					break
				}
			}

			if allNodesHaveProviderID {
				// All nodes have providerIDs, now verify them
				for _, node := range nodeList.Items {
					if !strings.HasPrefix(node.Spec.ProviderID, "hcloud://") {
						t.Errorf("Node %s has invalid providerID format: %s (expected hcloud://...)", node.Metadata.Name, node.Spec.ProviderID)
					} else {
						t.Logf("✓ Node %s has providerID: %s", node.Metadata.Name, node.Spec.ProviderID)
					}

					// Check for cloud provider labels
					if region, ok := node.Metadata.Labels["topology.kubernetes.io/region"]; ok {
						t.Logf("  ✓ Node %s has region label: %s", node.Metadata.Name, region)
					}
					if zone, ok := node.Metadata.Labels["topology.kubernetes.io/zone"]; ok {
						t.Logf("  ✓ Node %s has zone label: %s", node.Metadata.Name, zone)
					}
					if instanceType, ok := node.Metadata.Labels["node.kubernetes.io/instance-type"]; ok {
						t.Logf("  ✓ Node %s has instance-type label: %s", node.Metadata.Name, instanceType)
					}
				}
				t.Log("✓ All nodes have valid cloud provider IDs")
				providerIDsSet = true
				break
			}

			t.Logf("Attempt %d: Some nodes still missing providerIDs, waiting...", i+1)
			time.Sleep(5 * time.Second)
		}

		if !providerIDsSet {
			t.Error("Timeout: Not all nodes received providerIDs from CCM within 2 minutes")
		}

		// Functional test: CCM Load Balancer provisioning
		t.Log("Testing CCM Load Balancer provisioning...")

		testLBDeployment := "e2e-nginx"
		testLBService := "e2e-nginx-lb"

		// Create a simple nginx deployment
		deploymentManifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
        - name: nginx
          image: nginx:alpine
          ports:
            - containerPort: 80
      tolerations:
        - operator: Exists
`, testLBDeployment, testLBDeployment, testLBDeployment)

		// Apply deployment
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"apply", "-f", "-")
		cmd.Stdin = strings.NewReader(deploymentManifest)
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("Failed to create nginx deployment: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ Nginx deployment created")
		}

		// Create a LoadBalancer service
		serviceManifest := fmt.Sprintf(`
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: default
  annotations:
    load-balancer.hetzner.cloud/location: nbg1
spec:
  type: LoadBalancer
  selector:
    app: %s
  ports:
    - port: 80
      targetPort: 80
`, testLBService, testLBDeployment)

		// Apply service
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"apply", "-f", "-")
		cmd.Stdin = strings.NewReader(serviceManifest)
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("Failed to create LoadBalancer service: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ LoadBalancer service created")
		}

		// Wait for Service to get an external IP
		t.Log("Waiting for CCM to provision Load Balancer and assign external IP...")
		var externalIP string
		lbProvisioned := false
		for i := 0; i < 60; i++ { // Wait up to 5 minutes
			cmd = exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "svc", testLBService, "-n", "default", "-o", "json")
			output, err = cmd.CombinedOutput()
			if err != nil {
				t.Logf("Service not ready yet: %v (will retry)", err)
				time.Sleep(5 * time.Second)
				continue
			}

			var svc struct {
				Status struct {
					LoadBalancer struct {
						Ingress []struct {
							IP string `json:"ip"`
						} `json:"ingress"`
					} `json:"loadBalancer"`
				} `json:"status"`
			}
			if err := json.Unmarshal(output, &svc); err != nil {
				t.Logf("Failed to parse service: %v (will retry)", err)
				time.Sleep(5 * time.Second)
				continue
			}

			if len(svc.Status.LoadBalancer.Ingress) > 0 && svc.Status.LoadBalancer.Ingress[0].IP != "" {
				externalIP = svc.Status.LoadBalancer.Ingress[0].IP
				lbProvisioned = true
				t.Logf("✓ Load Balancer provisioned with external IP: %s", externalIP)
				break
			}
			t.Logf("Waiting for external IP assignment...")
			time.Sleep(5 * time.Second)
		}

		if !lbProvisioned {
			t.Error("Timeout: CCM failed to provision Load Balancer within 5 minutes")
		}

		// Verify the Load Balancer exists in Hetzner Cloud
		if externalIP != "" {
			t.Log("Verifying Load Balancer exists in Hetzner Cloud...")
			// List all LBs and find the one with matching IP
			lbs, err := hClient.HCloudClient().LoadBalancer.All(context.Background())
			if err != nil {
				t.Errorf("Failed to list Hetzner Load Balancers: %v", err)
			} else {
				foundLB := false
				for _, lb := range lbs {
					if lb.PublicNet.IPv4.IP.String() == externalIP {
						foundLB = true
						t.Logf("✓ Found Hetzner Load Balancer: %s (ID: %d)", lb.Name, lb.ID)
						t.Logf("  Type: %s, Location: %s", lb.LoadBalancerType.Name, lb.Location.Name)
						break
					}
				}
				if !foundLB {
					t.Errorf("Load Balancer with IP %s not found in Hetzner Cloud", externalIP)
				}
			}

			// Test HTTP connectivity to the Load Balancer
			t.Log("Testing HTTP connectivity through Load Balancer...")
			httpSuccess := false
			for i := 0; i < 12; i++ { // Wait up to 1 minute for nginx to be ready
				resp, err := httpGet(fmt.Sprintf("http://%s/", externalIP))
				if err == nil && resp.StatusCode == 200 {
					httpSuccess = true
					t.Log("✓ HTTP request successful through Load Balancer")
					break
				}
				t.Logf("HTTP not ready yet: %v (will retry)", err)
				time.Sleep(5 * time.Second)
			}
			if !httpSuccess {
				t.Error("Failed to connect to nginx through Load Balancer")
			}
		}

		// Cleanup LB test resources
		t.Log("Cleaning up Load Balancer test resources...")

		// Delete service first (triggers LB deletion)
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"delete", "svc", testLBService, "-n", "default")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Logf("Warning: Failed to delete LB service: %v", err)
		} else {
			t.Log("✓ LoadBalancer service deleted")
		}

		// Delete deployment
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"delete", "deployment", testLBDeployment, "-n", "default")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Logf("Warning: Failed to delete nginx deployment: %v", err)
		} else {
			t.Log("✓ Nginx deployment deleted")
		}

		// Wait for LB to be deleted from Hetzner Cloud
		if externalIP != "" {
			t.Log("Waiting for Load Balancer to be deleted from Hetzner Cloud...")
			lbDeleted := false
			for i := 0; i < 24; i++ { // Wait up to 2 minutes
				lbs, err := hClient.HCloudClient().LoadBalancer.All(context.Background())
				if err != nil {
					t.Logf("Failed to list LBs: %v (will retry)", err)
					time.Sleep(5 * time.Second)
					continue
				}
				found := false
				for _, lb := range lbs {
					if lb.PublicNet.IPv4.IP.String() == externalIP {
						found = true
						break
					}
				}
				if !found {
					lbDeleted = true
					break
				}
				t.Logf("Load Balancer still exists, waiting for deletion...")
				time.Sleep(5 * time.Second)
			}
			if !lbDeleted {
				t.Error("Timeout: Load Balancer was not deleted within 2 minutes")
			} else {
				t.Log("✓ Load Balancer deleted from Hetzner Cloud")
			}
		}

		t.Log("✓ CCM Load Balancer lifecycle test complete")

		// Verify CSI Driver is installed and running
		t.Log("Verifying Hetzner Cloud CSI Driver installation...")

		// Check if CSIDriver resource exists
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "csidriver", "csi.hetzner.cloud", "-o", "json")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("CSIDriver resource not found: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ CSIDriver resource exists")
		}

		// Check if CSI controller deployment exists
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "deployment", "-n", "kube-system", "hcloud-csi-controller", "-o", "json")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("CSI controller deployment not found: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ CSI controller deployment exists")
		}

		// Check if CSI node daemonset exists
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "daemonset", "-n", "kube-system", "hcloud-csi-node", "-o", "json")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("CSI node daemonset not found: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ CSI node daemonset exists")
		}

		// Check if StorageClass exists and is default
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "storageclass", "hcloud-volumes", "-o", "json")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("StorageClass not found: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ StorageClass 'hcloud-volumes' exists")

			// Verify StorageClass is default
			var sc struct {
				Metadata struct {
					Annotations map[string]string `json:"annotations"`
				} `json:"metadata"`
				Provisioner string `json:"provisioner"`
			}
			if err := json.Unmarshal(output, &sc); err != nil {
				t.Errorf("Failed to parse StorageClass: %v", err)
			} else {
				if sc.Provisioner != "csi.hetzner.cloud" {
					t.Errorf("StorageClass has wrong provisioner: %s (expected csi.hetzner.cloud)", sc.Provisioner)
				} else {
					t.Log("✓ StorageClass uses csi.hetzner.cloud provisioner")
				}
				if sc.Metadata.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
					t.Log("✓ StorageClass is marked as default")
				} else {
					t.Error("StorageClass is not marked as default")
				}
			}
		}

		// Wait for CSI controller pod to be running
		t.Log("Waiting for CSI controller pod to be Running...")
		csiControllerRunning := false
		for i := 0; i < 30; i++ { // Wait up to 5 minutes
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
		} else {
			t.Log("✓ CSI controller pod is Running")
		}

		// Check CSI node pods are running on all nodes
		t.Log("Verifying CSI node pods are running on all nodes...")
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "pods", "-n", "kube-system", "-l", "app.kubernetes.io/name=hcloud-csi,app.kubernetes.io/component=node",
			"-o", "json")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("Failed to get CSI node pods: %v\nOutput: %s", err, string(output))
		} else {
			var podList struct {
				Items []struct {
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
					Status struct {
						Phase string `json:"phase"`
					} `json:"status"`
				} `json:"items"`
			}
			if err := json.Unmarshal(output, &podList); err != nil {
				t.Errorf("Failed to parse CSI node pods: %v", err)
			} else {
				runningCount := 0
				for _, pod := range podList.Items {
					if pod.Status.Phase == "Running" {
						runningCount++
						t.Logf("✓ CSI node pod %s is Running", pod.Metadata.Name)
					} else {
						t.Logf("  CSI node pod %s is %s", pod.Metadata.Name, pod.Status.Phase)
					}
				}
				if runningCount >= 2 { // At least 2 nodes (1 CP + 1 worker)
					t.Logf("✓ CSI node pods running on %d nodes", runningCount)
				} else {
					t.Errorf("Expected CSI node pods on at least 2 nodes, found %d running", runningCount)
				}
			}
		}

		t.Log("✓ CSI Driver verification complete")

		// Functional test: Create and delete a volume
		t.Log("Testing CSI volume provisioning (create/mount/delete)...")

		testPVCName := "e2e-test-pvc"
		testPodName := "e2e-test-pod"

		// Create a PVC
		pvcManifest := fmt.Sprintf(`
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: %s
  namespace: default
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: hcloud-volumes
`, testPVCName)

		// Apply PVC
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"apply", "-f", "-")
		cmd.Stdin = strings.NewReader(pvcManifest)
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("Failed to create PVC: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ PVC created")
		}

		// Create a Pod that uses the PVC (triggers WaitForFirstConsumer binding)
		podManifest := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: default
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
`, testPodName, testPVCName)

		// Apply Pod
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"apply", "-f", "-")
		cmd.Stdin = strings.NewReader(podManifest)
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Errorf("Failed to create test Pod: %v\nOutput: %s", err, string(output))
		} else {
			t.Log("✓ Test Pod created (will trigger volume provisioning)")
		}

		// Wait for PVC to be Bound
		t.Log("Waiting for PVC to be Bound...")
		pvcBound := false
		var pvName string
		for i := 0; i < 60; i++ { // Wait up to 5 minutes
			cmd = exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "pvc", testPVCName, "-n", "default", "-o", "json")
			output, err = cmd.CombinedOutput()
			if err != nil {
				t.Logf("PVC not ready yet: %v (will retry)", err)
				time.Sleep(5 * time.Second)
				continue
			}

			var pvc struct {
				Status struct {
					Phase string `json:"phase"`
				} `json:"status"`
				Spec struct {
					VolumeName string `json:"volumeName"`
				} `json:"spec"`
			}
			if err := json.Unmarshal(output, &pvc); err != nil {
				t.Logf("Failed to parse PVC: %v (will retry)", err)
				time.Sleep(5 * time.Second)
				continue
			}

			if pvc.Status.Phase == "Bound" {
				pvcBound = true
				pvName = pvc.Spec.VolumeName
				t.Logf("✓ PVC is Bound to PV: %s", pvName)
				break
			}
			t.Logf("PVC phase: %s (waiting for Bound...)", pvc.Status.Phase)
			time.Sleep(5 * time.Second)
		}

		if !pvcBound {
			t.Error("Timeout: PVC failed to become Bound within 5 minutes")
		}

		// Verify PV exists and has correct CSI driver
		if pvName != "" {
			cmd = exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "pv", pvName, "-o", "json")
			output, err = cmd.CombinedOutput()
			if err != nil {
				t.Errorf("Failed to get PV: %v\nOutput: %s", err, string(output))
			} else {
				var pv struct {
					Spec struct {
						CSI struct {
							Driver       string `json:"driver"`
							VolumeHandle string `json:"volumeHandle"`
						} `json:"csi"`
					} `json:"spec"`
				}
				if err := json.Unmarshal(output, &pv); err != nil {
					t.Errorf("Failed to parse PV: %v", err)
				} else {
					if pv.Spec.CSI.Driver != "csi.hetzner.cloud" {
						t.Errorf("PV has wrong CSI driver: %s", pv.Spec.CSI.Driver)
					} else {
						t.Log("✓ PV uses csi.hetzner.cloud driver")
					}
					if pv.Spec.CSI.VolumeHandle != "" {
						t.Logf("✓ PV has Hetzner volume handle: %s", pv.Spec.CSI.VolumeHandle)
					}
				}
			}
		}

		// Wait for Pod to be Running (confirms volume is attached)
		t.Log("Waiting for test Pod to be Running...")
		podRunning := false
		for i := 0; i < 60; i++ { // Wait up to 5 minutes
			cmd = exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"get", "pod", testPodName, "-n", "default", "-o", "jsonpath={.status.phase}")
			output, err = cmd.CombinedOutput()
			if err == nil && string(output) == "Running" {
				podRunning = true
				break
			}
			t.Logf("Test Pod not running yet (phase: %s), waiting...", string(output))
			time.Sleep(5 * time.Second)
		}
		if !podRunning {
			t.Error("Timeout: Test Pod failed to reach Running state")
		} else {
			t.Log("✓ Test Pod is Running (volume successfully attached)")
		}

		// Cleanup: Delete Pod and PVC
		t.Log("Cleaning up test resources...")

		// Delete Pod first
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"delete", "pod", testPodName, "-n", "default", "--grace-period=0", "--force")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Logf("Warning: Failed to delete test Pod: %v", err)
		} else {
			t.Log("✓ Test Pod deleted")
		}

		// Delete PVC
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"delete", "pvc", testPVCName, "-n", "default")
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Logf("Warning: Failed to delete PVC: %v", err)
		} else {
			t.Log("✓ PVC deleted")
		}

		// Wait for PV to be deleted (ReclaimPolicy: Delete)
		if pvName != "" {
			t.Log("Waiting for PV to be deleted (ReclaimPolicy: Delete)...")
			pvDeleted := false
			for i := 0; i < 30; i++ { // Wait up to 2.5 minutes
				cmd = exec.CommandContext(context.Background(), "kubectl",
					"--kubeconfig", kubeconfigPath,
					"get", "pv", pvName)
				output, err = cmd.CombinedOutput()
				if err != nil && strings.Contains(string(output), "not found") {
					pvDeleted = true
					break
				}
				t.Logf("PV still exists, waiting for deletion...")
				time.Sleep(5 * time.Second)
			}
			if !pvDeleted {
				t.Error("Timeout: PV was not deleted within 2.5 minutes")
			} else {
				t.Log("✓ PV deleted (Hetzner volume should be deleted)")
			}
		}

		t.Log("✓ CSI volume lifecycle test complete (create/mount/delete verified)")
	}

	// Verify Resources using Interface Getters

	// Check Network
	net, err := hClient.GetNetwork(ctx, clusterName)
	assert.NoError(t, err)
	assert.NotNil(t, net)

	// Check Firewall
	fw, err := hClient.GetFirewall(ctx, clusterName)
	assert.NoError(t, err)
	assert.NotNil(t, fw)

	// Check LB
	lb, err = hClient.GetLoadBalancer(ctx, clusterName+"-kube-api")
	assert.NoError(t, err)
	assert.NotNil(t, lb)

	// Check Servers
	srvID, err := hClient.GetServerID(ctx, clusterName+"-control-plane-1")
	assert.NoError(t, err)
	assert.NotEmpty(t, srvID)
}
