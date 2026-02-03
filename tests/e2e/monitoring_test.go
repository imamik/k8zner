//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestE2EMonitoring tests the kube-prometheus-stack installation on a fresh cluster.
//
// This test verifies:
// 1. kube-prometheus-stack can be installed via operator
// 2. Prometheus, Grafana, and Alertmanager are healthy
// 3. ServiceMonitors and PrometheusRules are created
// 4. (Optional) Grafana ingress with TLS works if CF_API_TOKEN/CF_DOMAIN are set
//
// Prerequisites:
//   - HCLOUD_TOKEN - Hetzner Cloud API token
//
// Optional for Grafana ingress test:
//   - CF_API_TOKEN - Cloudflare API token
//   - CF_DOMAIN - Domain managed by Cloudflare (e.g., k8zner.org)
//   - CF_GRAFANA_SUBDOMAIN - (optional) Subdomain for Grafana (default: "grafana")
//
// Example:
//
//	# Basic test (no ingress)
//	HCLOUD_TOKEN=xxx go test -v -timeout=45m -tags=e2e -run TestE2EMonitoring ./tests/e2e/
//
//	# Full test with Grafana ingress
//	HCLOUD_TOKEN=xxx CF_API_TOKEN=yyy CF_DOMAIN=example.com \
//	go test -v -timeout=45m -tags=e2e -run TestE2EMonitoring ./tests/e2e/
func TestE2EMonitoring(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	// Generate unique cluster name
	timestamp := time.Now().Unix()
	clusterName := fmt.Sprintf("e2e-mon-%d", timestamp)

	t.Logf("=== Starting Monitoring E2E Test: %s ===", clusterName)

	// Create configuration with monitoring enabled
	configPath := CreateTestConfig(t, clusterName, ModeDev,
		WithWorkers(1),
		WithRegion("fsn1"),
		WithMonitoring(true),
	)
	defer os.Remove(configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	// Create cluster via operator
	var state *OperatorTestContext
	var err error

	t.Run("Create", func(t *testing.T) {
		state, err = CreateClusterViaOperator(ctx, t, configPath)
		require.NoError(t, err, "Cluster creation should succeed")
	})

	// Ensure cleanup
	defer func() {
		if state != nil {
			DestroyCluster(context.Background(), t, state)
		}
	}()

	// Wait for cluster to be ready
	t.Run("WaitForClusterReady", func(t *testing.T) {
		err := WaitForClusterReady(ctx, t, state, 30*time.Minute)
		require.NoError(t, err, "Cluster should become ready")
	})

	// Get legacy state for addon tests
	legacyState := state.ToE2EState()

	// Get network ID
	network, _ := state.HCloudClient.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	// Test monitoring stack
	cfAPIToken := os.Getenv("CF_API_TOKEN")
	cfDomain := os.Getenv("CF_DOMAIN")

	if cfAPIToken != "" && cfDomain != "" {
		t.Log("=== Installing monitoring with Grafana ingress ===")
		t.Run("MonitoringWithIngress", func(t *testing.T) {
			testMonitoringStack(t, legacyState, token)
		})
	} else {
		t.Log("=== Installing basic monitoring (no ingress) ===")
		t.Run("MonitoringBasic", func(t *testing.T) {
			testMonitoringWithoutIngress(t, legacyState, token, networkID)
		})
	}

	// Verify ServiceMonitors
	t.Run("VerifyServiceMonitors", func(t *testing.T) {
		verifyServiceMonitors(t, state.KubeconfigPath)
	})

	// Verify PrometheusRules
	t.Run("VerifyPrometheusRules", func(t *testing.T) {
		testPrometheusAlerts(t, state.KubeconfigPath)
	})

	t.Log("=== MONITORING E2E TEST PASSED ===")
}

// TestE2EMonitoringQuick tests monitoring installation on an existing cluster.
// This test reuses an existing cluster to save time.
//
// Prerequisites:
//   - HCLOUD_TOKEN - Hetzner Cloud API token
//   - E2E_CLUSTER_NAME - Name of existing cluster
//   - E2E_KUBECONFIG_PATH - Path to kubeconfig
//
// Example:
//
//	HCLOUD_TOKEN=xxx E2E_CLUSTER_NAME=my-cluster E2E_KUBECONFIG_PATH=./kubeconfig \
//	go test -v -timeout=20m -tags=e2e -run TestE2EMonitoringQuick ./tests/e2e/
func TestE2EMonitoringQuick(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	clusterName := os.Getenv("E2E_CLUSTER_NAME")
	kubeconfigPath := os.Getenv("E2E_KUBECONFIG_PATH")

	if clusterName == "" || kubeconfigPath == "" {
		t.Skip("E2E_CLUSTER_NAME and E2E_KUBECONFIG_PATH required for quick monitoring test")
	}

	t.Logf("=== Starting Quick Monitoring E2E Test on cluster: %s ===", clusterName)

	// Create state for existing cluster
	client := sharedCtx.Client
	legacyState := NewE2EState(clusterName, client)

	// Load existing kubeconfig
	kubeconfig, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		t.Fatalf("Failed to read kubeconfig: %v", err)
	}
	legacyState.Kubeconfig = kubeconfig
	legacyState.KubeconfigPath = kubeconfigPath

	// Get network ID
	ctx := context.Background()
	network, _ := legacyState.Client.GetNetwork(ctx, legacyState.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	// Run monitoring test
	cfAPIToken := os.Getenv("CF_API_TOKEN")
	cfDomain := os.Getenv("CF_DOMAIN")

	if cfAPIToken != "" && cfDomain != "" {
		t.Log("Testing monitoring with Grafana ingress...")
		testMonitoringStack(t, legacyState, token)
	} else {
		t.Log("Testing basic monitoring (no ingress)...")
		testMonitoringWithoutIngress(t, legacyState, token, networkID)
	}

	// Verify components
	verifyServiceMonitors(t, legacyState.KubeconfigPath)
	testPrometheusAlerts(t, legacyState.KubeconfigPath)

	t.Log("=== QUICK MONITORING E2E TEST PASSED ===")
}
