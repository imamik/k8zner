//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/platform/hcloud"
)

// TestE2EMonitoring tests the kube-prometheus-stack installation on a fresh cluster.
//
// This test verifies:
// 1. kube-prometheus-stack can be installed
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

	// Create Hetzner client for cleanup
	client := hcloud.NewRealClient(token)
	state := NewE2EState(clusterName, client)
	defer cleanupE2ECluster(t, state)

	// === PHASE 1: Deploy dev cluster ===
	t.Log("=== PHASE 1: Deploying dev cluster ===")
	deployDevClusterForMonitoring(t, state, token)

	// === PHASE 2: Get network ID ===
	ctx := context.Background()
	network, _ := state.Client.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	// === PHASE 3: Test monitoring stack ===
	cfAPIToken := os.Getenv("CF_API_TOKEN")
	cfDomain := os.Getenv("CF_DOMAIN")

	if cfAPIToken != "" && cfDomain != "" {
		t.Log("=== PHASE 3: Installing monitoring with Grafana ingress ===")
		testMonitoringStack(t, state, token)
	} else {
		t.Log("=== PHASE 3: Installing basic monitoring (no ingress) ===")
		testMonitoringWithoutIngress(t, state, token, networkID)
	}

	// === PHASE 4: Verify ServiceMonitors ===
	t.Log("=== PHASE 4: Verifying ServiceMonitors ===")
	verifyServiceMonitors(t, state.KubeconfigPath)

	// === PHASE 5: Verify PrometheusRules ===
	t.Log("=== PHASE 5: Verifying PrometheusRules ===")
	testPrometheusAlerts(t, state.KubeconfigPath)

	t.Log("=== MONITORING E2E TEST PASSED ===")
}

// deployDevClusterForMonitoring deploys a dev cluster for monitoring testing.
func deployDevClusterForMonitoring(t *testing.T, state *E2EState, token string) {
	t.Logf("[Deploy] Deploying Dev cluster for monitoring test...")

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	// Setup SSH key
	if err := setupSSHKeyForFullStack(ctx, t, state); err != nil {
		t.Fatalf("Failed to setup SSH key: %v", err)
	}

	// Create v2 config
	v2Cfg := &v2.Config{
		Name:   state.ClusterName,
		Region: v2.RegionFalkenstein,
		Mode:   v2.ModeDev,
		Workers: v2.Worker{
			Count: 1,
			Size:  v2.SizeCX22,
		},
	}

	// Deploy using the shared deployment function
	_ = deployClusterWithConfig(ctx, t, state, v2Cfg, token)

	t.Log("[Deploy] Cluster deployed successfully")
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
	client := hcloud.NewRealClient(token)
	state := NewE2EState(clusterName, client)

	// Load existing kubeconfig
	kubeconfig, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		t.Fatalf("Failed to read kubeconfig: %v", err)
	}
	state.Kubeconfig = kubeconfig
	state.KubeconfigPath = kubeconfigPath

	// Get network ID
	ctx := context.Background()
	network, _ := state.Client.GetNetwork(ctx, state.ClusterName)
	networkID := int64(0)
	if network != nil {
		networkID = network.ID
	}

	// Run monitoring test
	cfAPIToken := os.Getenv("CF_API_TOKEN")
	cfDomain := os.Getenv("CF_DOMAIN")

	if cfAPIToken != "" && cfDomain != "" {
		t.Log("Testing monitoring with Grafana ingress...")
		testMonitoringStack(t, state, token)
	} else {
		t.Log("Testing basic monitoring (no ingress)...")
		testMonitoringWithoutIngress(t, state, token, networkID)
	}

	// Verify components
	verifyServiceMonitors(t, state.KubeconfigPath)
	testPrometheusAlerts(t, state.KubeconfigPath)

	t.Log("=== QUICK MONITORING E2E TEST PASSED ===")
}
