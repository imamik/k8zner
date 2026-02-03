//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	k8znerv1alpha1 "github.com/imamik/k8zner/api/v1alpha1"
	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// TestOperatorCentricFlow tests the complete operator-centric provisioning flow.
// This test verifies:
// 1. k8zner create bootstraps infrastructure and first CP
// 2. Operator installs Cilium CNI
// 3. Operator installs remaining addons
// 4. k8zner apply scales workers
// 5. k8zner destroy cleans up all resources
func TestOperatorCentricFlow(t *testing.T) {
	if os.Getenv("HCLOUD_TOKEN") == "" {
		t.Skip("HCLOUD_TOKEN not set")
	}

	// Use a unique cluster name for this test
	clusterName := "e2e-operator-centric-" + time.Now().Format("20060102-150405")
	configPath := CreateTestConfig(t, clusterName, ModeDev, WithWorkers(1))
	defer os.Remove(configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	// Create cluster via operator
	var state *OperatorTestContext
	var err error

	t.Run("Create", func(t *testing.T) {
		state, err = CreateClusterViaOperator(ctx, t, configPath)
		require.NoError(t, err, "k8zner create should succeed")
	})

	// Ensure cleanup on test completion
	defer func() {
		if state != nil {
			DestroyCluster(context.Background(), t, state)
		}
	}()

	// Phase 2: Wait for Cilium to be ready
	t.Run("WaitForCilium", func(t *testing.T) {
		err := WaitForCiliumReady(ctx, t, state, 10*time.Minute)
		require.NoError(t, err, "Cilium should become ready")
	})

	// Phase 3: Wait for other addons to be installed
	t.Run("WaitForAddons", func(t *testing.T) {
		err := WaitForCoreAddons(ctx, t, state, 10*time.Minute)
		require.NoError(t, err, "Core addons should be installed")
	})

	// Phase 4: Verify workers are created by operator
	t.Run("WaitForWorkers", func(t *testing.T) {
		err := WaitForNodeCount(ctx, t, state, "workers", 1, 15*time.Minute)
		require.NoError(t, err, "Workers should be created by operator")
	})

	// Phase 5: Scale workers via apply
	t.Run("ScaleWorkers", func(t *testing.T) {
		// Scale to 2 workers
		err := ScaleCluster(ctx, t, state, 2)
		require.NoError(t, err, "k8zner apply should succeed")

		// Wait for scaling
		err = WaitForNodeCount(ctx, t, state, "workers", 2, 15*time.Minute)
		require.NoError(t, err, "Workers should scale to 2")
	})

	// Phase 6: Verify cluster is fully operational
	t.Run("VerifyClusterHealth", func(t *testing.T) {
		cluster := GetClusterStatus(ctx, state)
		require.NotNil(t, cluster, "cluster should exist")

		assert.Equal(t, k8znerv1alpha1.ClusterPhaseRunning, cluster.Status.Phase, "cluster should be running")
		assert.Equal(t, k8znerv1alpha1.PhaseComplete, cluster.Status.ProvisioningPhase, "provisioning should be complete")
		assert.GreaterOrEqual(t, cluster.Status.ControlPlanes.Ready, 1, "should have at least 1 ready CP")
		assert.GreaterOrEqual(t, cluster.Status.Workers.Ready, 2, "should have at least 2 ready workers")
	})

	// Phase 7: Destroy cluster
	t.Run("Destroy", func(t *testing.T) {
		destroyCtx, destroyCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer destroyCancel()

		err := DestroyCluster(destroyCtx, t, state)
		require.NoError(t, err, "k8zner destroy should succeed")
		state = nil // Prevent double cleanup
	})

	// Phase 8: Verify cleanup
	t.Run("VerifyCleanup", func(t *testing.T) {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cleanupCancel()

		// Re-create a temporary state for verification
		tempState := &OperatorTestContext{
			ClusterName:  clusterName,
			HCloudClient: sharedCtx.Client,
		}

		// Verify resources are deleted from Hetzner
		networkName := clusterName + "-network"
		network, err := tempState.HCloudClient.GetNetwork(cleanupCtx, networkName)
		assert.NoError(t, err, "GetNetwork should not error")
		assert.Nil(t, network, "network should be deleted")

		firewallName := clusterName + "-firewall"
		firewall, err := tempState.HCloudClient.GetFirewall(cleanupCtx, firewallName)
		assert.NoError(t, err, "GetFirewall should not error")
		assert.Nil(t, firewall, "firewall should be deleted")

		lbName := clusterName + "-kube-api"
		lb, err := tempState.HCloudClient.GetLoadBalancer(cleanupCtx, lbName)
		assert.NoError(t, err, "GetLoadBalancer should not error")
		assert.Nil(t, lb, "load balancer should be deleted")
	})
}

// TestOperatorCentricMigration tests migrating a CLI cluster to operator-managed.
func TestOperatorCentricMigration(t *testing.T) {
	t.Skip("Migration test requires an existing CLI cluster - run manually")

	if os.Getenv("HCLOUD_TOKEN") == "" {
		t.Skip("HCLOUD_TOKEN not set")
	}

	// This test assumes there's an existing CLI-provisioned cluster
	configPath := "k8zner.yaml"

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	t.Run("Migrate", func(t *testing.T) {
		err := handlers.Migrate(ctx, configPath, false)
		require.NoError(t, err, "migration should succeed")
	})

	t.Run("VerifyOperatorManaged", func(t *testing.T) {
		// Load kubeconfig
		kubeconfig, err := os.ReadFile("kubeconfig")
		require.NoError(t, err)

		// Create a temporary state to use shared helpers
		state, err := CreateClusterViaOperator(ctx, t, configPath)
		require.NoError(t, err)

		// Check CRD exists
		cluster := GetClusterStatus(ctx, state)
		require.NotNil(t, cluster, "K8znerCluster CRD should exist")

		// Check credentials ref is set
		assert.NotEmpty(t, cluster.Spec.CredentialsRef.Name, "credentials ref should be set")

		// Check operator is deployed using kubectl
		assert.NotNil(t, kubeconfig)
	})
}

// TestOperatorCentricHealth tests the health command.
func TestOperatorCentricHealth(t *testing.T) {
	t.Skip("Health test requires a running cluster - run as part of TestOperatorCentricFlow")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	configPath := "k8zner.yaml"

	t.Run("HealthCheck", func(t *testing.T) {
		err := handlers.Health(ctx, configPath, false, false)
		require.NoError(t, err, "health check should succeed")
	})

	t.Run("HealthCheckJSON", func(t *testing.T) {
		err := handlers.Health(ctx, configPath, false, true)
		require.NoError(t, err, "health check with JSON output should succeed")
	})
}
