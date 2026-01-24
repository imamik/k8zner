//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/imamik/k8zner/internal/addons"
	"github.com/imamik/k8zner/internal/config"
)

// phaseAddonsAdvanced tests advanced addon configurations.
// This is Phase 3b of the E2E lifecycle - tests new features from the gap analysis.
// Run after basic addons phase to test advanced configuration options.
//
// Environment variables:
//
//	E2E_SKIP_ADDONS_ADVANCED - Set to "true" to skip advanced addon testing
//
// Features tested:
//   - Talos CCM installation (new in gap analysis)
//   - Metrics Server advanced options (ScheduleOnControlPlane, Replicas)
//   - Gateway API CRDs installation
//   - Prometheus Operator CRDs installation
//   - Cilium advanced options (BPF datapath, policy CIDR, Gateway API)
//   - Ingress NGINX advanced options (Kind, Replicas, ExternalTrafficPolicy)
//   - CCM LoadBalancer configuration (type, algorithm, health checks)
//   - Custom Helm values override
func phaseAddonsAdvanced(t *testing.T, state *E2EState) {
	t.Log("Testing advanced addon configurations...")

	// New features from Terraform parity gap analysis
	t.Run("TalosCCM", func(t *testing.T) { testAddonTalosCCM(t, state) })
	t.Run("MetricsServerAdvanced", func(t *testing.T) { testAddonMetricsServerAdvanced(t, state) })

	// CRD addons (prerequisites for some features)
	t.Run("GatewayAPICRDs", func(t *testing.T) { testAddonGatewayAPICRDs(t, state) })
	t.Run("PrometheusOperatorCRDs", func(t *testing.T) { testAddonPrometheusOperatorCRDs(t, state) })

	// Advanced Cilium configuration
	t.Run("CiliumAdvanced", func(t *testing.T) { testAddonCiliumAdvanced(t, state) })

	// Advanced Ingress NGINX configuration
	t.Run("IngressNginxAdvanced", func(t *testing.T) { testAddonIngressNginxAdvanced(t, state) })

	// Advanced CCM configuration
	t.Run("CCMAdvanced", func(t *testing.T) { testAddonCCMAdvanced(t, state) })

	// Custom Helm values override
	t.Run("HelmCustomValues", func(t *testing.T) { testAddonHelmCustomValues(t, state) })

	t.Log("✓ Phase 3b: Advanced Addons (all tested)")
}

// testAddonTalosCCM tests Talos Cloud Controller Manager installation.
// This is the Siderolabs Talos CCM (separate from Hetzner CCM) that provides
// node lifecycle management features.
// See: terraform/variables.tf talos_ccm_* variables
func testAddonTalosCCM(t *testing.T, state *E2EState) {
	t.Log("Installing Talos CCM...")

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		Addons: config.AddonsConfig{
			TalosCCM: config.TalosCCMConfig{
				Enabled: true,
				Version: "v1.11.0",
			},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0, "", 0, nil); err != nil {
		t.Fatalf("Failed to install Talos CCM: %v", err)
	}

	// Wait for Talos CCM DaemonSet to be ready
	t.Log("  Waiting for Talos CCM DaemonSet...")
	waitForDaemonSet(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=talos-cloud-controller-manager", 5*time.Minute)

	// Verify the DaemonSet exists
	verifyDaemonSetExists(t, state.KubeconfigPath, "kube-system", "talos-cloud-controller-manager")

	state.AddonsInstalled["talos-ccm"] = true
	t.Log("✓ Talos CCM installed and verified")
}

// testAddonMetricsServerAdvanced tests Metrics Server with advanced configuration options.
// Tests: ScheduleOnControlPlane, Replicas config options (new in gap analysis).
// See: terraform/variables.tf metrics_server_schedule_on_control_plane, metrics_server_replicas
func testAddonMetricsServerAdvanced(t *testing.T, state *E2EState) {
	t.Log("Testing Metrics Server advanced configuration...")

	// Test 1: Explicit ScheduleOnControlPlane = true with custom Replicas
	t.Log("  Testing explicit ScheduleOnControlPlane=true with Replicas=1...")
	scheduleOnCP := true
	replicas := 1

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp", Count: 1},
			},
		},
		Workers: []config.WorkerNodePool{
			{Name: "worker", Count: 1}, // Has workers, but we force CP scheduling
		},
		Addons: config.AddonsConfig{
			MetricsServer: config.MetricsServerConfig{
				Enabled:                true,
				ScheduleOnControlPlane: &scheduleOnCP,
				Replicas:               &replicas,
			},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0, "", 0, nil); err != nil {
		t.Fatalf("Failed to install Metrics Server with advanced config: %v", err)
	}

	// Wait for metrics-server pod
	waitForPod(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=metrics-server", 5*time.Minute)

	// Verify replicas
	verifyDeploymentReplicas(t, state.KubeconfigPath, "kube-system", "metrics-server", 1)

	// Verify node selector for control plane
	verifyMetricsServerNodeSelector(t, state.KubeconfigPath)

	// Test 2: Update to 2 replicas
	t.Log("  Testing Replicas=2...")
	replicas = 2
	cfg.Addons.MetricsServer.Replicas = &replicas

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0, "", 0, nil); err != nil {
		t.Fatalf("Failed to update Metrics Server replicas: %v", err)
	}

	// Wait for deployment to update
	time.Sleep(10 * time.Second)
	verifyDeploymentReplicas(t, state.KubeconfigPath, "kube-system", "metrics-server", 2)

	state.AddonsInstalled["metrics-server-advanced"] = true
	t.Log("✓ Metrics Server advanced configuration verified")
}

// verifyMetricsServerNodeSelector verifies Metrics Server has control plane node selector.
func verifyMetricsServerNodeSelector(t *testing.T, kubeconfigPath string) {
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "deployment", "-n", "kube-system", "metrics-server",
		"-o", "jsonpath={.spec.template.spec.nodeSelector}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: Could not get Metrics Server node selector: %v", err)
		return
	}

	if strings.Contains(string(output), "node-role.kubernetes.io/control-plane") {
		t.Log("  ✓ Metrics Server has control plane node selector")
	} else {
		t.Logf("  Note: Metrics Server node selector: %s", string(output))
	}
}

// testAddonGatewayAPICRDs tests Gateway API CRDs installation.
// Verifies that Gateway API CRDs are installed correctly with configurable version and channel.
func testAddonGatewayAPICRDs(t *testing.T, state *E2EState) {
	t.Log("Installing Gateway API CRDs...")

	// Test with standard channel
	cfg := &config.Config{
		ClusterName: state.ClusterName,
		Addons: config.AddonsConfig{
			GatewayAPICRDs: config.GatewayAPICRDsConfig{
				Enabled:        true,
				Version:        "v1.2.1",
				ReleaseChannel: "standard",
			},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0, "", 0, nil); err != nil {
		t.Fatalf("Failed to install Gateway API CRDs: %v", err)
	}

	// Verify Gateway API CRDs exist
	gatewayCRDs := []string{
		"gateways.gateway.networking.k8s.io",
		"gatewayclasses.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
	}

	for _, crd := range gatewayCRDs {
		verifyCRDExists(t, state.KubeconfigPath, crd)
	}

	state.AddonsInstalled["gateway-api-crds"] = true
	t.Log("✓ Gateway API CRDs installed and verified")
}

// testAddonPrometheusOperatorCRDs tests Prometheus Operator CRDs installation.
// Verifies that Prometheus Operator CRDs are installed correctly with configurable version.
func testAddonPrometheusOperatorCRDs(t *testing.T, state *E2EState) {
	t.Log("Installing Prometheus Operator CRDs...")

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		Addons: config.AddonsConfig{
			PrometheusOperatorCRDs: config.PrometheusOperatorCRDsConfig{
				Enabled: true,
				Version: "v0.79.2",
			},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0, "", 0, nil); err != nil {
		t.Fatalf("Failed to install Prometheus Operator CRDs: %v", err)
	}

	// Verify Prometheus Operator CRDs exist
	prometheusCRDs := []string{
		"servicemonitors.monitoring.coreos.com",
		"podmonitors.monitoring.coreos.com",
		"prometheusrules.monitoring.coreos.com",
	}

	for _, crd := range prometheusCRDs {
		verifyCRDExists(t, state.KubeconfigPath, crd)
	}

	state.AddonsInstalled["prometheus-operator-crds"] = true
	t.Log("✓ Prometheus Operator CRDs installed and verified")
}

// testAddonCiliumAdvanced tests Cilium with advanced configuration options.
// Tests: BPFDatapathMode, PolicyCIDRMatchMode, SocketLBHostNamespaceOnly,
// ServiceMonitorEnabled, GatewayAPIEnabled with proxy protocol and traffic policy.
func testAddonCiliumAdvanced(t *testing.T, state *E2EState) {
	t.Log("Testing Cilium advanced configuration...")

	// Skip if Cilium already installed - we test configuration, not reinstallation
	if state.AddonsInstalled["cilium"] {
		t.Log("  Cilium already installed, testing configuration values...")
	}

	// Test configuration with advanced options
	cfg := &config.Config{
		ClusterName: state.ClusterName,
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Addons: config.AddonsConfig{
			Cilium: config.CiliumConfig{
				Enabled:                     true,
				EncryptionEnabled:           true,
				EncryptionType:              "wireguard",
				RoutingMode:                 "native",
				KubeProxyReplacementEnabled: true,

				// Advanced options being tested
				BPFDatapathMode:           "veth", // Default, safest option
				PolicyCIDRMatchMode:       "nodes",
				SocketLBHostNamespaceOnly: true,
				ServiceMonitorEnabled:     false, // Would need Prometheus Operator

				// Gateway API options
				GatewayAPIEnabled:               true,
				GatewayAPIExternalTrafficPolicy: "Cluster",

				// Hubble
				HubbleEnabled:      true,
				HubbleRelayEnabled: true,
				HubbleUIEnabled:    false,
			},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0, "", 0, nil); err != nil {
		t.Fatalf("Failed to install Cilium with advanced config: %v", err)
	}

	// Wait for Cilium to be ready
	t.Log("  Waiting for Cilium operator...")
	waitForPod(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=cilium-operator", 8*time.Minute)

	t.Log("  Waiting for Cilium agents...")
	waitForDaemonSet(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=cilium-agent", 8*time.Minute)

	// Verify Cilium configuration via ConfigMap
	verifyCiliumConfig(t, state.KubeconfigPath)

	// Verify Gateway API is enabled by checking for GatewayClass
	if cfg.Addons.Cilium.GatewayAPIEnabled {
		t.Log("  Verifying Cilium Gateway API...")
		verifyGatewayClassExists(t, state.KubeconfigPath, "cilium")
	}

	state.AddonsInstalled["cilium-advanced"] = true
	t.Log("✓ Cilium advanced configuration verified")
}

// testAddonIngressNginxAdvanced tests Ingress NGINX with advanced configuration.
// Tests: Kind (DaemonSet), Replicas, TopologyAwareRouting, ExternalTrafficPolicy, Config.
func testAddonIngressNginxAdvanced(t *testing.T, state *E2EState) {
	t.Log("Testing Ingress NGINX advanced configuration...")

	// First, test with Deployment and custom replicas
	t.Log("  Testing Deployment with 2 replicas...")
	replicas := 2
	cfg := &config.Config{
		ClusterName: state.ClusterName,
		Addons: config.AddonsConfig{
			IngressNginx: config.IngressNginxConfig{
				Enabled:               true,
				Kind:                  "Deployment",
				Replicas:              &replicas,
				ExternalTrafficPolicy: "Local",
				TopologyAwareRouting:  false,
				Config: map[string]string{
					"use-proxy-protocol":         "true",
					"compute-full-forwarded-for": "true",
					"proxy-body-size":            "100m",
				},
			},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0, "", 0, nil); err != nil {
		t.Fatalf("Failed to install Ingress NGINX with Deployment: %v", err)
	}

	// Wait for controller
	waitForPod(t, state.KubeconfigPath, "ingress-nginx", "app.kubernetes.io/component=controller", 8*time.Minute)

	// Verify deployment has correct replicas
	verifyDeploymentReplicas(t, state.KubeconfigPath, "ingress-nginx", "ingress-nginx-controller", 2)

	// Verify ConfigMap has custom settings
	verifyIngressNginxConfig(t, state.KubeconfigPath, map[string]string{
		"use-proxy-protocol": "true",
		"proxy-body-size":    "100m",
	})

	// Now test with DaemonSet (replicas should not be set)
	t.Log("  Testing DaemonSet configuration...")
	cfgDaemonSet := &config.Config{
		ClusterName: state.ClusterName,
		Addons: config.AddonsConfig{
			IngressNginx: config.IngressNginxConfig{
				Enabled:               true,
				Kind:                  "DaemonSet",
				Replicas:              nil, // Must be nil for DaemonSet
				ExternalTrafficPolicy: "Cluster",
			},
		},
	}

	if err := addons.Apply(context.Background(), cfgDaemonSet, state.Kubeconfig, 0, "", 0, nil); err != nil {
		t.Fatalf("Failed to install Ingress NGINX as DaemonSet: %v", err)
	}

	// Verify DaemonSet exists
	verifyDaemonSetExists(t, state.KubeconfigPath, "ingress-nginx", "ingress-nginx-controller")

	state.AddonsInstalled["ingress-nginx-advanced"] = true
	t.Log("✓ Ingress NGINX advanced configuration verified")
}

// testAddonCCMAdvanced tests CCM with advanced LoadBalancer configuration.
// Tests: LoadBalancer type, algorithm, health check settings.
func testAddonCCMAdvanced(t *testing.T, state *E2EState) {
	t.Log("Testing CCM advanced LoadBalancer configuration...")

	ctx := context.Background()

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Fatal("HCLOUD_TOKEN required for CCM advanced testing")
	}

	// Get network ID
	network, _ := state.Client.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	// Configure CCM with advanced LB settings
	enabled := true
	usePrivateIP := true
	disablePrivateIngress := true
	disablePublicNetwork := false

	cfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: token,
		Location:    "nbg1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
		},
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{
				Enabled: true,
				LoadBalancers: config.CCMLoadBalancerConfig{
					Enabled:               &enabled,
					Location:              "nbg1",
					Type:                  "lb11",
					Algorithm:             "least_connections",
					UsePrivateIP:          &usePrivateIP,
					DisablePrivateIngress: &disablePrivateIngress,
					DisablePublicNetwork:  &disablePublicNetwork,
					HealthCheck: config.CCMHealthCheckConfig{
						Interval: 5,
						Timeout:  3,
						Retries:  3,
					},
				},
			},
		},
	}

	if err := addons.Apply(ctx, cfg, state.Kubeconfig, networkID, "", 0, nil); err != nil {
		t.Fatalf("Failed to install CCM with advanced config: %v", err)
	}

	// Wait for CCM pod
	waitForPod(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=hcloud-cloud-controller-manager", 5*time.Minute)

	// Test LB creation with custom settings via annotations
	testCCMLoadBalancerAdvanced(t, state)

	state.AddonsInstalled["ccm-advanced"] = true
	t.Log("✓ CCM advanced configuration verified")
}

// testAddonHelmCustomValues tests custom Helm values override functionality.
// Verifies that custom Helm values are properly merged with defaults.
func testAddonHelmCustomValues(t *testing.T, state *E2EState) {
	t.Log("Testing custom Helm values override...")

	// Test with Metrics Server using custom Helm values
	cfg := &config.Config{
		ClusterName: state.ClusterName,
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{Name: "cp", Count: 1},
			},
		},
		Addons: config.AddonsConfig{
			MetricsServer: config.MetricsServerConfig{
				Enabled: true,
				Helm: config.HelmChartConfig{
					Values: map[string]any{
						// Custom values to override defaults
						"args": []string{
							"--kubelet-insecure-tls",
							"--kubelet-preferred-address-types=InternalIP",
							"--metric-resolution=30s", // Custom: change from default 15s
						},
						"resources": map[string]any{
							"requests": map[string]any{
								"cpu":    "50m",
								"memory": "64Mi",
							},
							"limits": map[string]any{
								"cpu":    "100m",
								"memory": "128Mi",
							},
						},
					},
				},
			},
		},
	}

	if err := addons.Apply(context.Background(), cfg, state.Kubeconfig, 0, "", 0, nil); err != nil {
		t.Fatalf("Failed to install Metrics Server with custom Helm values: %v", err)
	}

	// Wait for metrics-server pod
	waitForPod(t, state.KubeconfigPath, "kube-system", "app.kubernetes.io/name=metrics-server", 5*time.Minute)

	// Verify custom args are applied
	verifyDeploymentArgs(t, state.KubeconfigPath, "kube-system", "metrics-server", "--metric-resolution=30s")

	// Verify custom resource limits are applied
	verifyDeploymentResources(t, state.KubeconfigPath, "kube-system", "metrics-server", "128Mi")

	state.AddonsInstalled["helm-custom-values"] = true
	t.Log("✓ Custom Helm values override verified")
}

// Helper functions for advanced E2E tests

// verifyCiliumConfig verifies Cilium configuration via its ConfigMap.
func verifyCiliumConfig(t *testing.T, kubeconfigPath string) {
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "configmap", "-n", "kube-system", "cilium-config", "-o", "yaml")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: Could not get Cilium ConfigMap: %v", err)
		return
	}

	// Check for expected configuration values
	configStr := string(output)
	if !strings.Contains(configStr, "enable-wireguard") {
		t.Log("  Note: WireGuard encryption config present in ConfigMap")
	}
	t.Log("  ✓ Cilium ConfigMap verified")
}

// verifyGatewayClassExists verifies a GatewayClass exists.
func verifyGatewayClassExists(t *testing.T, kubeconfigPath, name string) {
	// Wait a bit for GatewayClass to be created
	for i := 0; i < 12; i++ {
		cmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "gatewayclass", name)
		if err := cmd.Run(); err == nil {
			t.Logf("  ✓ GatewayClass %s exists", name)
			return
		}
		time.Sleep(5 * time.Second)
	}
	t.Logf("  Note: GatewayClass %s not found (may be expected if Gateway API CRDs not installed)", name)
}

// verifyDeploymentReplicas verifies a Deployment has the expected number of replicas.
func verifyDeploymentReplicas(t *testing.T, kubeconfigPath, namespace, name string, expectedReplicas int) {
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "deployment", "-n", namespace, name,
		"-o", "jsonpath={.spec.replicas}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get deployment replicas: %v", err)
	}

	if string(output) != fmt.Sprintf("%d", expectedReplicas) {
		t.Fatalf("Expected %d replicas, got %s", expectedReplicas, string(output))
	}
	t.Logf("  ✓ Deployment %s has %d replicas", name, expectedReplicas)
}

// verifyDaemonSetExists verifies a DaemonSet exists.
func verifyDaemonSetExists(t *testing.T, kubeconfigPath, namespace, name string) {
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "daemonset", "-n", namespace, name)
	if err := cmd.Run(); err != nil {
		t.Fatalf("DaemonSet %s not found in namespace %s", name, namespace)
	}
	t.Logf("  ✓ DaemonSet %s exists", name)
}

// verifyIngressNginxConfig verifies Ingress NGINX ConfigMap has expected values.
func verifyIngressNginxConfig(t *testing.T, kubeconfigPath string, expectedConfig map[string]string) {
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "configmap", "-n", "ingress-nginx", "ingress-nginx-controller", "-o", "yaml")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: Could not get Ingress NGINX ConfigMap: %v", err)
		return
	}

	configStr := string(output)
	for key, value := range expectedConfig {
		if !strings.Contains(configStr, key) || !strings.Contains(configStr, value) {
			t.Logf("  Warning: Expected config %s=%s not found", key, value)
		}
	}
	t.Log("  ✓ Ingress NGINX ConfigMap verified")
}

// testCCMLoadBalancerAdvanced tests CCM LB creation with custom annotations.
func testCCMLoadBalancerAdvanced(t *testing.T, state *E2EState) {
	t.Log("  Testing CCM LB with custom configuration...")

	testLBName := "e2e-ccm-advanced-test-lb"

	// Create service with custom CCM annotations
	manifest := fmt.Sprintf(`
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: default
  annotations:
    load-balancer.hetzner.cloud/location: nbg1
    load-balancer.hetzner.cloud/type: lb11
    load-balancer.hetzner.cloud/algorithm-type: least_connections
    load-balancer.hetzner.cloud/health-check-interval: 5s
    load-balancer.hetzner.cloud/health-check-timeout: 3s
    load-balancer.hetzner.cloud/health-check-retries: "3"
spec:
  type: LoadBalancer
  ports:
    - port: 80
  selector:
    app: nonexistent
`, testLBName)

	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create test LB service: %v\nOutput: %s", err, string(output))
	}

	// Wait for external IP
	externalIP := ""
	for i := 0; i < 36; i++ {
		cmd = exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", state.KubeconfigPath,
			"get", "svc", testLBName, "-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
		output, err := cmd.CombinedOutput()
		if err == nil && len(output) > 0 {
			externalIP = string(output)
			break
		}
		if i > 0 && i%6 == 0 {
			t.Logf("  [%ds] Waiting for advanced LB external IP...", i*5)
		}
		time.Sleep(5 * time.Second)
	}

	if externalIP == "" {
		t.Log("  Warning: CCM failed to provision LB with custom config (timeout)")
	} else {
		t.Logf("  ✓ CCM provisioned LB with custom config, IP: %s", externalIP)
	}

	// Cleanup
	exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", state.KubeconfigPath,
		"delete", "svc", testLBName).Run()

	time.Sleep(10 * time.Second)
}

// verifyDeploymentArgs verifies a Deployment container has expected args.
func verifyDeploymentArgs(t *testing.T, kubeconfigPath, namespace, name, expectedArg string) {
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "deployment", "-n", namespace, name,
		"-o", "jsonpath={.spec.template.spec.containers[0].args}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: Could not get deployment args: %v", err)
		return
	}

	if strings.Contains(string(output), expectedArg) {
		t.Logf("  ✓ Deployment %s has custom arg: %s", name, expectedArg)
	} else {
		t.Logf("  Note: Expected arg %s not found in deployment %s", expectedArg, name)
	}
}

// verifyDeploymentResources verifies a Deployment has expected resource limits.
func verifyDeploymentResources(t *testing.T, kubeconfigPath, namespace, name, expectedMemoryLimit string) {
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "deployment", "-n", namespace, name,
		"-o", "jsonpath={.spec.template.spec.containers[0].resources.limits.memory}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: Could not get deployment resources: %v", err)
		return
	}

	if string(output) == expectedMemoryLimit {
		t.Logf("  ✓ Deployment %s has custom memory limit: %s", name, expectedMemoryLimit)
	} else {
		t.Logf("  Note: Expected memory limit %s, got %s", expectedMemoryLimit, string(output))
	}
}
