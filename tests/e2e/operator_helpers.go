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

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/cmd/k8zner/handlers"
	"github.com/imamik/k8zner/internal/platform/hcloud"
)

// OperatorTestContext holds state for operator-centric tests.
// This replaces E2EState for tests using the operator pattern.
type OperatorTestContext struct {
	// Cluster identity
	ClusterName string
	ConfigPath  string

	// Kubernetes access
	KubeconfigPath string
	Kubeconfig     []byte
	K8sClient      client.Client

	// Hetzner client for verification and cleanup
	HCloudClient *hcloud.RealClient

	// Test metadata
	TestID string
	T      *testing.T
}

// ConfigOption is a functional option for CreateTestConfig.
type ConfigOption func(*testConfigOptions)

// testConfigOptions holds all configuration options for cluster creation.
type testConfigOptions struct {
	workerCount   int
	workerSize    string
	cpSize        string
	domain        string
	argoSubdomain string
	backup        bool
	monitoring    bool
	region        string
}

// defaultConfigOptions returns the default configuration options.
func defaultConfigOptions() *testConfigOptions {
	return &testConfigOptions{
		workerCount: 1,
		workerSize:  "cpx22",
		cpSize:      "cpx22",
		region:      "nbg1",
	}
}

// WithWorkers sets the worker count.
func WithWorkers(count int) ConfigOption {
	return func(o *testConfigOptions) {
		o.workerCount = count
	}
}

// WithWorkerSize sets the worker server size.
func WithWorkerSize(size string) ConfigOption {
	return func(o *testConfigOptions) {
		o.workerSize = size
	}
}

// WithCPSize sets the control plane server size.
func WithCPSize(size string) ConfigOption {
	return func(o *testConfigOptions) {
		o.cpSize = size
	}
}

// WithDomain sets the domain for DNS/TLS integration.
func WithDomain(domain string) ConfigOption {
	return func(o *testConfigOptions) {
		o.domain = domain
	}
}

// WithArgoSubdomain sets the ArgoCD subdomain.
func WithArgoSubdomain(subdomain string) ConfigOption {
	return func(o *testConfigOptions) {
		o.argoSubdomain = subdomain
	}
}

// WithBackup enables backup configuration.
func WithBackup(enabled bool) ConfigOption {
	return func(o *testConfigOptions) {
		o.backup = enabled
	}
}

// WithMonitoring enables monitoring configuration.
func WithMonitoring(enabled bool) ConfigOption {
	return func(o *testConfigOptions) {
		o.monitoring = enabled
	}
}

// WithRegion sets the Hetzner region.
func WithRegion(region string) ConfigOption {
	return func(o *testConfigOptions) {
		o.region = region
	}
}

// Mode represents the cluster mode.
type Mode string

const (
	ModeDev Mode = "dev"
	ModeHA  Mode = "ha"
)

// CreateTestConfig creates a YAML configuration file for testing.
func CreateTestConfig(t *testing.T, name string, mode Mode, opts ...ConfigOption) string {
	t.Helper()

	options := defaultConfigOptions()
	for _, opt := range opts {
		opt(options)
	}

	var content strings.Builder
	content.WriteString(fmt.Sprintf("name: %s\n", name))
	content.WriteString(fmt.Sprintf("region: %s\n", options.region))
	content.WriteString(fmt.Sprintf("mode: %s\n", string(mode)))
	content.WriteString("\n")
	content.WriteString("workers:\n")
	content.WriteString(fmt.Sprintf("  count: %d\n", options.workerCount))
	content.WriteString(fmt.Sprintf("  size: %s\n", options.workerSize))
	content.WriteString("\n")
	content.WriteString("control_plane:\n")
	content.WriteString(fmt.Sprintf("  size: %s\n", options.cpSize))

	if options.domain != "" {
		content.WriteString("\n")
		content.WriteString(fmt.Sprintf("domain: %s\n", options.domain))
		if options.argoSubdomain != "" {
			content.WriteString(fmt.Sprintf("argo_subdomain: %s\n", options.argoSubdomain))
		}
	}

	if options.backup {
		content.WriteString("\nbackup: true\n")
	}

	tmpFile, err := os.CreateTemp("", "k8zner-e2e-*.yaml")
	require.NoError(t, err)

	_, err = tmpFile.WriteString(content.String())
	require.NoError(t, err)

	err = tmpFile.Close()
	require.NoError(t, err)

	return tmpFile.Name()
}

// CreateClusterViaOperator creates a cluster using the operator-centric flow.
// It runs `k8zner create`, waits for kubeconfig, and sets up the K8s client.
func CreateClusterViaOperator(ctx context.Context, t *testing.T, configPath string) (*OperatorTestContext, error) {
	t.Helper()

	// Extract cluster name from config
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	clusterName := extractClusterNameFromConfig(string(configContent))

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("HCLOUD_TOKEN not set")
	}

	state := &OperatorTestContext{
		ClusterName:  clusterName,
		ConfigPath:   configPath,
		HCloudClient: hcloud.NewRealClient(token),
		TestID:       clusterName,
		T:            t,
	}

	// Create cluster with operator management
	t.Logf("Creating cluster %s via operator...", clusterName)
	if err := handlers.Create(ctx, configPath, false); err != nil {
		return state, fmt.Errorf("k8zner create failed: %w", err)
	}

	// Wait for kubeconfig to be available
	t.Log("Waiting for kubeconfig...")
	var kubeconfig []byte
	kubeconfigReady := false
	deadline := time.Now().Add(5 * time.Minute)

	for time.Now().Before(deadline) {
		kubeconfig, err = os.ReadFile("kubeconfig")
		if err == nil && len(kubeconfig) > 0 {
			kubeconfigReady = true
			break
		}
		time.Sleep(5 * time.Second)
	}

	if !kubeconfigReady {
		return state, fmt.Errorf("timeout waiting for kubeconfig")
	}

	state.Kubeconfig = kubeconfig
	state.KubeconfigPath = "kubeconfig"

	// Create K8s client
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return state, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return state, fmt.Errorf("failed to create k8s client: %w", err)
	}
	state.K8sClient = k8sClient

	t.Logf("Cluster %s created successfully", clusterName)
	return state, nil
}

// WaitForClusterReady waits for the cluster to reach Running phase with all components ready.
func WaitForClusterReady(ctx context.Context, t *testing.T, state *OperatorTestContext, timeout time.Duration) error {
	t.Helper()

	t.Log("Waiting for cluster to be ready...")
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cluster := GetClusterStatus(ctx, state)
		if cluster == nil {
			t.Log("  Cluster CRD not found yet, waiting...")
			time.Sleep(30 * time.Second)
			continue
		}

		t.Logf("  Cluster phase: %s, provisioning: %s, CP ready: %d, workers ready: %d",
			cluster.Status.Phase,
			cluster.Status.ProvisioningPhase,
			cluster.Status.ControlPlanes.Ready,
			cluster.Status.Workers.Ready)

		if cluster.Status.Phase == k8znerv1alpha1.ClusterPhaseRunning &&
			cluster.Status.ProvisioningPhase == k8znerv1alpha1.PhaseComplete {
			t.Log("Cluster is ready!")
			return nil
		}

		time.Sleep(30 * time.Second)
	}

	return fmt.Errorf("timeout waiting for cluster to be ready")
}

// WaitForCiliumReady waits for Cilium CNI to be ready.
func WaitForCiliumReady(ctx context.Context, t *testing.T, state *OperatorTestContext, timeout time.Duration) error {
	t.Helper()

	t.Log("Waiting for Cilium to be ready...")
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if IsCiliumReady(ctx, state.K8sClient) {
			t.Log("Cilium is ready!")
			return nil
		}

		time.Sleep(30 * time.Second)
	}

	return fmt.Errorf("timeout waiting for Cilium")
}

// WaitForAddonInstalled waits for a specific addon to be installed via CRD status.
func WaitForAddonInstalled(ctx context.Context, t *testing.T, state *OperatorTestContext, addonName string, timeout time.Duration) error {
	t.Helper()

	t.Logf("Waiting for addon %s to be installed...", addonName)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cluster := GetClusterStatus(ctx, state)
		if cluster == nil {
			time.Sleep(30 * time.Second)
			continue
		}

		addon, ok := cluster.Status.Addons[addonName]
		if ok && addon.Installed {
			t.Logf("Addon %s is installed!", addonName)
			return nil
		}

		t.Logf("  Addon %s not ready yet...", addonName)
		time.Sleep(30 * time.Second)
	}

	return fmt.Errorf("timeout waiting for addon %s", addonName)
}

// WaitForCoreAddons waits for core addons (CCM, CSI) to be installed.
func WaitForCoreAddons(ctx context.Context, t *testing.T, state *OperatorTestContext, timeout time.Duration) error {
	t.Helper()

	t.Log("Waiting for core addons (CCM, CSI) to be installed...")
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cluster := GetClusterStatus(ctx, state)
		if cluster == nil {
			time.Sleep(30 * time.Second)
			continue
		}

		ccm, ccmOk := cluster.Status.Addons[k8znerv1alpha1.AddonNameCCM]
		csi, csiOk := cluster.Status.Addons[k8znerv1alpha1.AddonNameCSI]

		if ccmOk && ccm.Installed && csiOk && csi.Installed {
			t.Log("Core addons are installed!")
			return nil
		}

		t.Logf("  CCM: %v, CSI: %v", ccmOk && ccm.Installed, csiOk && csi.Installed)
		time.Sleep(30 * time.Second)
	}

	return fmt.Errorf("timeout waiting for core addons")
}

// WaitForNodeCount waits for a specific node count via CRD status.
func WaitForNodeCount(ctx context.Context, t *testing.T, state *OperatorTestContext, nodeType string, count int, timeout time.Duration) error {
	t.Helper()

	t.Logf("Waiting for %d %s nodes...", count, nodeType)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cluster := GetClusterStatus(ctx, state)
		if cluster == nil {
			time.Sleep(30 * time.Second)
			continue
		}

		var ready int
		switch nodeType {
		case "worker", "workers":
			ready = cluster.Status.Workers.Ready
		case "controlplane", "control-plane", "cp":
			ready = cluster.Status.ControlPlanes.Ready
		default:
			return fmt.Errorf("unknown node type: %s", nodeType)
		}

		if ready >= count {
			t.Logf("%s nodes ready: %d", nodeType, ready)
			return nil
		}

		t.Logf("  %s ready: %d/%d", nodeType, ready, count)
		time.Sleep(30 * time.Second)
	}

	return fmt.Errorf("timeout waiting for %d %s nodes", count, nodeType)
}

// ScaleCluster scales the cluster by updating the config and running apply.
func ScaleCluster(ctx context.Context, t *testing.T, state *OperatorTestContext, workerCount int) error {
	t.Helper()

	t.Logf("Scaling cluster to %d workers...", workerCount)

	// Update config file with new worker count
	UpdateTestConfigWorkers(t, state.ConfigPath, workerCount)

	// Apply the change
	if err := handlers.Apply(ctx, state.ConfigPath); err != nil {
		return fmt.Errorf("k8zner apply failed: %w", err)
	}

	t.Log("Scale request applied successfully")
	return nil
}

// DestroyCluster destroys the cluster using the operator flow.
func DestroyCluster(ctx context.Context, t *testing.T, state *OperatorTestContext) error {
	t.Helper()

	if state == nil || state.ConfigPath == "" {
		t.Log("No cluster to destroy")
		return nil
	}

	t.Logf("Destroying cluster %s...", state.ClusterName)

	destroyCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	if err := handlers.Destroy(destroyCtx, state.ConfigPath); err != nil {
		t.Logf("Warning: k8zner destroy failed: %v", err)
		// Attempt emergency cleanup
		return emergencyCleanup(ctx, t, state)
	}

	t.Log("Cluster destroyed successfully")
	return nil
}

// VerifyClusterHealth verifies the cluster health via CRD status.
func VerifyClusterHealth(t *testing.T, state *OperatorTestContext) {
	t.Helper()

	ctx := context.Background()
	cluster := GetClusterStatus(ctx, state)
	require.NotNil(t, cluster, "cluster CRD should exist")

	require.Equal(t, k8znerv1alpha1.ClusterPhaseRunning, cluster.Status.Phase,
		"cluster should be running")
	require.Equal(t, k8znerv1alpha1.PhaseComplete, cluster.Status.ProvisioningPhase,
		"provisioning should be complete")
	require.GreaterOrEqual(t, cluster.Status.ControlPlanes.Ready, 1,
		"should have at least 1 ready control plane")

	t.Log("Cluster health verified!")
}

// VerifyClusterCleanup verifies that all cluster resources are cleaned up.
func VerifyClusterCleanup(ctx context.Context, t *testing.T, state *OperatorTestContext) {
	t.Helper()

	t.Log("Verifying cluster cleanup...")

	// Check network is gone
	networkName := state.ClusterName + "-network"
	network, err := state.HCloudClient.GetNetwork(ctx, networkName)
	require.NoError(t, err, "GetNetwork should not error")
	require.Nil(t, network, "network should be deleted")

	// Check firewall is gone
	firewallName := state.ClusterName + "-firewall"
	firewall, err := state.HCloudClient.GetFirewall(ctx, firewallName)
	require.NoError(t, err, "GetFirewall should not error")
	require.Nil(t, firewall, "firewall should be deleted")

	// Check LB is gone
	lbName := state.ClusterName + "-kube-api"
	lb, err := state.HCloudClient.GetLoadBalancer(ctx, lbName)
	require.NoError(t, err, "GetLoadBalancer should not error")
	require.Nil(t, lb, "load balancer should be deleted")

	t.Log("Cluster cleanup verified!")
}

// GetClusterStatus fetches the K8znerCluster CRD.
// Returns nil if state or K8sClient is nil (e.g., cluster creation failed).
func GetClusterStatus(ctx context.Context, state *OperatorTestContext) *k8znerv1alpha1.K8znerCluster {
	if state == nil || state.K8sClient == nil {
		return nil
	}
	cluster := &k8znerv1alpha1.K8znerCluster{}
	key := client.ObjectKey{
		Namespace: "k8zner-system",
		Name:      state.ClusterName,
	}
	if err := state.K8sClient.Get(ctx, key, cluster); err != nil {
		return nil
	}
	return cluster
}

// IsCiliumReady checks if Cilium pods are running.
func IsCiliumReady(ctx context.Context, k8sClient client.Client) bool {
	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList,
		client.InNamespace("kube-system"),
		client.MatchingLabels{"k8s-app": "cilium"},
	); err != nil {
		return false
	}

	if len(podList.Items) == 0 {
		return false
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			return false
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status != corev1.ConditionTrue {
				return false
			}
		}
	}

	return true
}

// UpdateTestConfigWorkers updates the worker count in the test config.
func UpdateTestConfigWorkers(t *testing.T, configPath string, count int) {
	t.Helper()

	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	clusterName := extractClusterNameFromConfig(string(content))

	// Create new config with updated count
	// Parse existing config to preserve other settings
	lines := strings.Split(string(content), "\n")
	var newContent strings.Builder
	inWorkers := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "workers:" {
			inWorkers = true
			newContent.WriteString(line + "\n")
			continue
		}

		if inWorkers && strings.HasPrefix(trimmed, "count:") {
			newContent.WriteString(fmt.Sprintf("  count: %d\n", count))
			continue
		}

		if inWorkers && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
			inWorkers = false
		}

		newContent.WriteString(line + "\n")
	}

	err = os.WriteFile(configPath, []byte(newContent.String()), 0644)
	require.NoError(t, err)

	t.Logf("Updated config %s: cluster=%s, workers=%d", configPath, clusterName, count)
}

// extractClusterNameFromConfig extracts cluster name from config content.
func extractClusterNameFromConfig(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name:") {
			return strings.TrimSpace(trimmed[5:])
		}
	}
	return "unknown"
}

// emergencyCleanup performs cleanup when normal destroy fails.
func emergencyCleanup(ctx context.Context, t *testing.T, state *OperatorTestContext) error {
	t.Log("Performing emergency cleanup...")

	if state.HCloudClient == nil {
		return fmt.Errorf("no HCloud client available for cleanup")
	}

	// Use label-based cleanup
	labelSelector := map[string]string{
		"cluster": state.ClusterName,
	}

	if err := state.HCloudClient.CleanupByLabel(ctx, labelSelector); err != nil {
		t.Logf("Warning: label-based cleanup failed: %v", err)
	}

	// Clean up kubeconfig
	if state.KubeconfigPath != "" {
		os.Remove(state.KubeconfigPath)
	}

	return nil
}

// RunFunctionalTests runs functional verification tests.
func RunFunctionalTests(t *testing.T, state *OperatorTestContext) {
	t.Helper()

	// Convert to legacy state for backward compatibility with existing tests
	legacyState := state.ToE2EState()

	t.Run("CCM_LoadBalancer", func(t *testing.T) {
		testCCMLoadBalancer(t, legacyState)
	})

	t.Run("CSI_Volume", func(t *testing.T) {
		testCSIVolume(t, legacyState)
	})

	t.Run("Cilium_Network", func(t *testing.T) {
		testCiliumNetworkConnectivity(t, legacyState)
	})
}

// ToE2EState converts OperatorTestContext to legacy E2EState for backward compatibility.
func (o *OperatorTestContext) ToE2EState() *E2EState {
	ctx := context.Background()

	state := &E2EState{
		ClusterName:     o.ClusterName,
		TestID:          o.TestID,
		Client:          o.HCloudClient,
		Kubeconfig:      o.Kubeconfig,
		KubeconfigPath:  o.KubeconfigPath,
		AddonsInstalled: make(map[string]bool),
	}

	// Collect cluster info
	if o.HCloudClient != nil {
		// Get control plane IPs
		for i := 1; i <= 3; i++ {
			serverName := fmt.Sprintf("%s-control-plane-%d", o.ClusterName, i)
			ip, err := o.HCloudClient.GetServerIP(ctx, serverName)
			if err == nil {
				state.ControlPlaneIPs = append(state.ControlPlaneIPs, ip)
			}
		}

		// Get worker IPs
		for i := 1; i <= 10; i++ {
			serverName := fmt.Sprintf("%s-workers-%d", o.ClusterName, i)
			ip, err := o.HCloudClient.GetServerIP(ctx, serverName)
			if err == nil {
				state.WorkerIPs = append(state.WorkerIPs, ip)
			}
		}

		// Get load balancer IP
		lb, err := o.HCloudClient.GetLoadBalancer(ctx, o.ClusterName+"-kube-api")
		if err == nil && lb != nil {
			state.LoadBalancerIP = hcloud.LoadBalancerIPv4(lb)
		}
	}

	return state
}

// WaitForClusterPhase waits for the K8znerCluster to reach a specific phase.
func WaitForClusterPhase(ctx context.Context, t *testing.T, state *OperatorTestContext, expectedPhase k8znerv1alpha1.ClusterPhase, timeout time.Duration) error {
	t.Helper()

	t.Logf("Waiting for cluster phase: %s", expectedPhase)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cluster := GetClusterStatus(ctx, state)
		if cluster != nil && cluster.Status.Phase == expectedPhase {
			t.Logf("Cluster reached phase: %s", expectedPhase)
			return nil
		}

		if cluster != nil {
			t.Logf("  Current phase: %s (waiting for %s)", cluster.Status.Phase, expectedPhase)
		}

		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("timeout waiting for cluster phase %s", expectedPhase)
}

// SimulateNodeFailure powers off a node to simulate failure.
func SimulateNodeFailure(ctx context.Context, t *testing.T, state *OperatorTestContext, nodeName string) error {
	t.Helper()

	t.Logf("Simulating failure of node: %s", nodeName)

	// Get server by name
	servers, err := state.HCloudClient.GetServersByLabel(ctx, map[string]string{
		"cluster": state.ClusterName,
	})
	if err != nil {
		return fmt.Errorf("failed to get servers: %w", err)
	}

	for _, server := range servers {
		if server.Name == nodeName {
			// Use hcloud CLI to power off
			cmd := exec.CommandContext(ctx, "hcloud", "server", "poweroff", fmt.Sprintf("%d", server.ID))
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to power off server: %w, output: %s", err, string(output))
			}
			t.Logf("Node %s powered off", nodeName)
			return nil
		}
	}

	return fmt.Errorf("server %s not found", nodeName)
}

// WaitForNodeNotReadyK8s waits for a Kubernetes node to become NotReady.
func WaitForNodeNotReadyK8s(ctx context.Context, t *testing.T, kubeconfigPath, nodeName string, timeout time.Duration) error {
	t.Helper()

	t.Logf("Waiting for node %s to become NotReady...", nodeName)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cmd := exec.CommandContext(ctx, "kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "node", nodeName,
			"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Node might not exist anymore
			if strings.Contains(string(output), "NotFound") {
				return nil
			}
			time.Sleep(10 * time.Second)
			continue
		}

		status := strings.TrimSpace(string(output))
		if status != "True" {
			t.Logf("Node %s is NotReady (status: %s)", nodeName, status)
			return nil
		}

		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("timeout waiting for node %s to become NotReady", nodeName)
}

// CountKubernetesNodesViaKubectl returns the number of nodes in the cluster.
func CountKubernetesNodesViaKubectl(t *testing.T, kubeconfigPath string) int {
	t.Helper()

	cmd := exec.Command("kubectl",
		"--kubeconfig", kubeconfigPath,
		"get", "nodes", "--no-headers")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: Failed to get nodes: %v", err)
		return 0
	}

	lines := 0
	for _, b := range output {
		if b == '\n' {
			lines++
		}
	}
	return lines
}
