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

// TestE2ELifecycle runs the complete E2E test lifecycle using the operator-centric pattern.
// This test provisions a full Kubernetes cluster on Hetzner Cloud and validates
// all functionality from cluster creation through scaling.
//
// Phases:
//  1. Create - Provision cluster via k8zner create (operator-centric)
//  2. Wait for Ready - Wait for cluster to reach Running phase
//  3. Verify Addons - Verify core addons are installed via CRD status
//  4. Scale - Scale workers via k8zner apply
//  5. Functional Tests - Run functional verification tests
//  6. Destroy - Clean up via k8zner destroy
//
// Environment variables:
//
//	HCLOUD_TOKEN - Required
//
// Examples:
//
//	# Full test
//	HCLOUD_TOKEN=xxx go test -v -timeout=1h -tags=e2e -run TestE2ELifecycle ./tests/e2e/
func TestE2ELifecycle(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	// Load E2E configuration
	e2eConfig := LoadE2EConfig()

	// Check if reusing existing cluster
	if e2eConfig.ReuseCluster {
		if e2eConfig.ClusterName == "" {
			t.Fatal("E2E_CLUSTER_NAME must be set when E2E_REUSE_CLUSTER=true")
		}
		if e2eConfig.KubeconfigPath == "" {
			t.Fatal("E2E_KUBECONFIG_PATH must be set when E2E_REUSE_CLUSTER=true")
		}
		t.Logf("Reusing existing cluster: %s", e2eConfig.ClusterName)
		runLifecycleOnExistingCluster(t, e2eConfig)
		return
	}

	// Create new cluster
	clusterName := fmt.Sprintf("e2e-seq-%d", time.Now().Unix())
	t.Logf("Starting E2E lifecycle test for cluster: %s", clusterName)

	// Create configuration
	configPath := CreateTestConfig(t, clusterName, ModeDev, WithWorkers(1))
	defer os.Remove(configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	// Create cluster via operator
	var state *OperatorTestContext
	var err error

	// Phase 1: Create
	t.Run("Phase1_Create", func(t *testing.T) {
		state, err = CreateClusterViaOperator(ctx, t, configPath)
		require.NoError(t, err, "Cluster creation should succeed")
	})

	// Ensure cleanup
	defer func() {
		if state != nil {
			t.Log("=== Phase: Destroy ===")
			DestroyCluster(context.Background(), t, state)
		}
	}()

	// Phase 2: Wait for cluster to be ready
	t.Run("Phase2_WaitForReady", func(t *testing.T) {
		err := WaitForClusterReady(ctx, t, state, 30*time.Minute)
		require.NoError(t, err, "Cluster should become ready")
	})

	// Phase 3: Verify addons via CRD status
	if !e2eConfig.SkipAddons {
		t.Run("Phase3_VerifyAddons", func(t *testing.T) {
			// Wait for Cilium
			err := WaitForCiliumReady(ctx, t, state, 10*time.Minute)
			require.NoError(t, err, "Cilium should be ready")

			// Wait for core addons
			err = WaitForCoreAddons(ctx, t, state, 10*time.Minute)
			require.NoError(t, err, "Core addons should be installed")
		})
	} else {
		t.Log("Skipping Phase 3: Addons (E2E_SKIP_ADDONS=true)")
	}

	// Phase 4: Scale
	if !e2eConfig.SkipScale {
		t.Run("Phase4_Scale", func(t *testing.T) {
			// Scale to 2 workers
			err := ScaleCluster(ctx, t, state, 2)
			require.NoError(t, err, "Scale request should succeed")

			// Wait for scaling
			err = WaitForNodeCount(ctx, t, state, "workers", 2, 15*time.Minute)
			require.NoError(t, err, "Workers should scale to 2")
		})
	} else {
		t.Log("Skipping Phase 4: Scale (E2E_SKIP_SCALE=true)")
	}

	// Phase 5: Functional Tests
	t.Run("Phase5_FunctionalTests", func(t *testing.T) {
		VerifyClusterHealth(t, state)
		RunFunctionalTests(t, state)
	})

	t.Log("=== E2E Lifecycle Complete ===")
	t.Logf("Cluster: %s", clusterName)
}

// runLifecycleOnExistingCluster runs tests on an existing cluster.
func runLifecycleOnExistingCluster(t *testing.T, config *E2EConfig) {
	t.Log("Running lifecycle tests on existing cluster...")

	// Create a minimal state for the existing cluster
	kubeconfig, err := os.ReadFile(config.KubeconfigPath)
	if err != nil {
		t.Fatalf("Failed to read kubeconfig: %v", err)
	}

	legacyState := NewE2EState(config.ClusterName, sharedCtx.Client)
	legacyState.Kubeconfig = kubeconfig
	legacyState.KubeconfigPath = config.KubeconfigPath

	// Run functional tests only
	t.Run("FunctionalTests", func(t *testing.T) {
		// Test CCM
		t.Run("CCM_LoadBalancer", func(t *testing.T) {
			testCCMLoadBalancer(t, legacyState)
		})

		// Test CSI
		t.Run("CSI_Volume", func(t *testing.T) {
			testCSIVolume(t, legacyState)
		})

		// Test Cilium
		t.Run("Cilium_Network", func(t *testing.T) {
			testCiliumNetworkConnectivity(t, legacyState)
		})
	})

	t.Log("=== Lifecycle tests on existing cluster complete ===")
}

// getInstalledAddonsList returns a list of installed addon names.
func getInstalledAddonsList(state *E2EState) []string {
	addons := []string{}
	for name, installed := range state.AddonsInstalled {
		if installed {
			addons = append(addons, name)
		}
	}
	return addons
}
