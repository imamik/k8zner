//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"k8zner/internal/platform/hcloud"
)

// TestE2ELifecycle runs the complete E2E test lifecycle sequentially.
// This test provisions a full Kubernetes cluster on Hetzner Cloud and validates
// all functionality from snapshots through addons and scaling.
//
// Phases:
//  1. Snapshots - Build and verify Talos snapshots
//  2. Cluster - Provision infrastructure and bootstrap Kubernetes
//  3. Addons - Install and test each addon sequentially
//     3b. Advanced Addons - Test advanced addon configurations (Gateway API, Prometheus CRDs, etc.)
//  4. Scale - Scale cluster and verify operation
//  5. Upgrade - Upgrade Talos and Kubernetes versions
//
// Environment variables:
//
//	HCLOUD_TOKEN - Required
//	E2E_KEEP_SNAPSHOTS - Set to "true" to cache snapshots between runs
//	E2E_SKIP_SNAPSHOTS - Set to "true" to skip snapshot building
//	E2E_SKIP_CLUSTER - Set to "true" to skip cluster provisioning
//	E2E_SKIP_ADDONS - Set to "true" to skip addon testing
//	E2E_SKIP_ADDONS_ADVANCED - Set to "true" to skip advanced addon testing
//	E2E_SKIP_SCALE - Set to "true" to skip scale testing
//	E2E_SKIP_UPGRADE - Set to "true" to skip upgrade testing
//	E2E_REUSE_CLUSTER - Set to "true" to reuse existing cluster
//	E2E_CLUSTER_NAME - Name of cluster to reuse (if E2E_REUSE_CLUSTER=true)
//	E2E_KUBECONFIG_PATH - Path to kubeconfig (if E2E_REUSE_CLUSTER=true)
//
// Examples:
//
//	# Full test
//	HCLOUD_TOKEN=xxx go test -v -timeout=1h -tags=e2e -run TestE2ELifecycle ./tests/e2e/
//
//	# Skip scale and upgrade (faster)
//	E2E_SKIP_SCALE=true E2E_SKIP_UPGRADE=true go test -v -timeout=30m -tags=e2e -run TestE2ELifecycle ./tests/e2e/
//
//	# Only test upgrade on existing cluster
//	E2E_REUSE_CLUSTER=true E2E_CLUSTER_NAME=my-cluster E2E_KUBECONFIG_PATH=./kubeconfig \
//	E2E_SKIP_SNAPSHOTS=true E2E_SKIP_CLUSTER=true E2E_SKIP_ADDONS=true E2E_SKIP_SCALE=true \
//	go test -v -timeout=30m -tags=e2e -run TestE2ELifecycle ./tests/e2e/
func TestE2ELifecycle(t *testing.T) {
	// This test is inherently sequential and long-running
	// Do not mark as parallel

	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	// Load E2E configuration
	e2eConfig := LoadE2EConfig()

	// Initialize state
	var state *E2EState
	var clusterName string

	if e2eConfig.ReuseCluster {
		// Reuse existing cluster
		if e2eConfig.ClusterName == "" {
			t.Fatal("E2E_CLUSTER_NAME must be set when E2E_REUSE_CLUSTER=true")
		}
		if e2eConfig.KubeconfigPath == "" {
			t.Fatal("E2E_KUBECONFIG_PATH must be set when E2E_REUSE_CLUSTER=true")
		}
		clusterName = e2eConfig.ClusterName
		t.Logf("Reusing existing cluster: %s", clusterName)

		client := hcloud.NewRealClient(token)
		state = NewE2EState(clusterName, client)

		// Load existing kubeconfig
		kubeconfig, err := os.ReadFile(e2eConfig.KubeconfigPath)
		if err != nil {
			t.Fatalf("Failed to read kubeconfig: %v", err)
		}
		state.Kubeconfig = kubeconfig
		state.KubeconfigPath = e2eConfig.KubeconfigPath
	} else {
		// Create new cluster
		clusterName = fmt.Sprintf("e2e-seq-%d", time.Now().Unix())
		t.Logf("Starting E2E lifecycle test for cluster: %s", clusterName)

		client := hcloud.NewRealClient(token)
		state = NewE2EState(clusterName, client)

		// Register cleanup to run at end (or on failure/timeout)
		defer cleanupE2ECluster(t, state)
	}

	t.Logf("Running phases: %v", e2eConfig.RunPhases())

	// Phase 1: Snapshots
	if !e2eConfig.SkipSnapshots {
		t.Run("Phase1_Snapshots", func(t *testing.T) {
			phaseSnapshots(t, state)
		})
	} else {
		t.Log("Skipping Phase 1: Snapshots (E2E_SKIP_SNAPSHOTS=true)")
	}

	// Phase 2: Cluster Provisioning
	if !e2eConfig.SkipCluster {
		t.Run("Phase2_Cluster", func(t *testing.T) {
			phaseCluster(t, state)
		})
	} else {
		t.Log("Skipping Phase 2: Cluster (E2E_SKIP_CLUSTER=true)")
	}

	// Phase 3: Addons
	if !e2eConfig.SkipAddons {
		t.Run("Phase3_Addons", func(t *testing.T) {
			phaseAddons(t, state)
		})
	} else {
		t.Log("Skipping Phase 3: Addons (E2E_SKIP_ADDONS=true)")
	}

	// Phase 3b: Advanced Addons (tests new gap analysis features)
	if !e2eConfig.SkipAddonsAdvanced {
		t.Run("Phase3b_AddonsAdvanced", func(t *testing.T) {
			phaseAddonsAdvanced(t, state)
		})
	} else {
		t.Log("Skipping Phase 3b: Advanced Addons (E2E_SKIP_ADDONS_ADVANCED=true)")
	}

	// Phase 4: Scale
	if !e2eConfig.SkipScale {
		t.Run("Phase4_Scale", func(t *testing.T) {
			phaseScale(t, state)
		})
	} else {
		t.Log("Skipping Phase 4: Scale (E2E_SKIP_SCALE=true)")
	}

	// Phase 5: Upgrade
	if !e2eConfig.SkipUpgrade {
		t.Run("Phase5_Upgrade", func(t *testing.T) {
			phaseUpgrade(t, state)
		})
	} else {
		t.Log("Skipping Phase 5: Upgrade (E2E_SKIP_UPGRADE=true)")
	}

	t.Log("✓ E2E Lifecycle Complete")
	t.Logf("Cluster: %s", clusterName)
	if !e2eConfig.SkipSnapshots {
		t.Logf("Snapshots: amd64=%s, arm64=%s", state.SnapshotAMD64, state.SnapshotARM64)
	}
	if !e2eConfig.SkipCluster {
		t.Logf("Load Balancer: %s", state.LoadBalancerIP)
	}
	if !e2eConfig.SkipAddons {
		t.Logf("Addons installed: %v", getInstalledAddonsList(state))
	}
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
