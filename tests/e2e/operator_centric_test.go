//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	configPath := createTestConfig(t, clusterName)
	defer os.Remove(configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	// Phase 1: Create cluster with operator management
	t.Run("Create", func(t *testing.T) {
		err := handlers.Create(ctx, configPath, false)
		require.NoError(t, err, "k8zner create should succeed")
	})

	// Wait for kubeconfig to be available
	var kubeconfig []byte
	t.Run("WaitForKubeconfig", func(t *testing.T) {
		require.Eventually(t, func() bool {
			var err error
			kubeconfig, err = os.ReadFile("kubeconfig")
			return err == nil && len(kubeconfig) > 0
		}, 2*time.Minute, 5*time.Second, "kubeconfig should be created")
	})

	// Create k8s client
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	require.NoError(t, err, "should parse kubeconfig")

	scheme := k8znerv1alpha1.Scheme
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	require.NoError(t, err, "should create k8s client")

	// Phase 2: Wait for Cilium to be ready
	t.Run("WaitForCilium", func(t *testing.T) {
		require.Eventually(t, func() bool {
			return isCiliumReady(ctx, k8sClient)
		}, 10*time.Minute, 30*time.Second, "Cilium should become ready")
	})

	// Phase 3: Wait for other addons to be installed
	t.Run("WaitForAddons", func(t *testing.T) {
		require.Eventually(t, func() bool {
			cluster := getCluster(ctx, k8sClient, clusterName)
			if cluster == nil {
				return false
			}
			// Check for CCM and CSI at minimum
			ccm, ccmOk := cluster.Status.Addons[k8znerv1alpha1.AddonNameCCM]
			csi, csiOk := cluster.Status.Addons[k8znerv1alpha1.AddonNameCSI]
			return ccmOk && ccm.Installed && csiOk && csi.Installed
		}, 10*time.Minute, 30*time.Second, "Core addons should be installed")
	})

	// Phase 4: Verify workers are created by operator
	t.Run("WaitForWorkers", func(t *testing.T) {
		require.Eventually(t, func() bool {
			cluster := getCluster(ctx, k8sClient, clusterName)
			if cluster == nil {
				return false
			}
			return cluster.Status.Workers.Ready >= 1
		}, 15*time.Minute, 30*time.Second, "Workers should be created by operator")
	})

	// Phase 5: Scale workers via apply
	t.Run("ScaleWorkers", func(t *testing.T) {
		// Update config to scale workers
		updateTestConfigWorkers(t, configPath, 2)

		// Apply the change
		err := handlers.Apply(ctx, configPath)
		require.NoError(t, err, "k8zner apply should succeed")

		// Wait for scaling
		require.Eventually(t, func() bool {
			cluster := getCluster(ctx, k8sClient, clusterName)
			if cluster == nil {
				return false
			}
			return cluster.Status.Workers.Ready >= 2
		}, 15*time.Minute, 30*time.Second, "Workers should scale to 2")
	})

	// Phase 6: Verify cluster is fully operational
	t.Run("VerifyClusterHealth", func(t *testing.T) {
		cluster := getCluster(ctx, k8sClient, clusterName)
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

		err := handlers.Destroy(destroyCtx, configPath)
		require.NoError(t, err, "k8zner destroy should succeed")
	})

	// Phase 8: Verify cleanup
	t.Run("VerifyCleanup", func(t *testing.T) {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cleanupCancel()

		// Verify resources are deleted from Hetzner
		hcloudClient := sharedCtx.Client

		// Check network is gone
		networkName := clusterName + "-network"
		network, err := hcloudClient.GetNetwork(cleanupCtx, networkName)
		assert.NoError(t, err, "GetNetwork should not error")
		assert.Nil(t, network, "network should be deleted")

		// Check firewall is gone
		firewallName := clusterName + "-firewall"
		firewall, err := hcloudClient.GetFirewall(cleanupCtx, firewallName)
		assert.NoError(t, err, "GetFirewall should not error")
		assert.Nil(t, firewall, "firewall should be deleted")

		// Check LB is gone
		lbName := clusterName + "-kube-api"
		lb, err := hcloudClient.GetLoadBalancer(cleanupCtx, lbName)
		assert.NoError(t, err, "GetLoadBalancer should not error")
		assert.Nil(t, lb, "load balancer should be deleted")
	})
}

// getCluster fetches the K8znerCluster CRD.
func getCluster(ctx context.Context, k8sClient client.Client, name string) *k8znerv1alpha1.K8znerCluster {
	cluster := &k8znerv1alpha1.K8znerCluster{}
	key := client.ObjectKey{
		Namespace: "k8zner-system",
		Name:      name,
	}
	if err := k8sClient.Get(ctx, key, cluster); err != nil {
		return nil
	}
	return cluster
}

// isCiliumReady checks if Cilium pods are running.
func isCiliumReady(ctx context.Context, k8sClient client.Client) bool {
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

// createTestConfig creates a test configuration file.
func createTestConfig(t *testing.T, clusterName string) string {
	t.Helper()

	content := `
cluster_name: ` + clusterName + `
location: nbg1

kubernetes:
  version: "1.32.2"

talos:
  version: "v1.10.2"

control_plane:
  - name: cp
    count: 1
    server_type: cpx22

workers:
  - name: worker
    count: 1
    server_type: cpx22

addons:
  cilium:
    enabled: true
  ccm:
    enabled: true
  csi:
    enabled: true
  metrics_server:
    enabled: true
`

	tmpFile, err := os.CreateTemp("", "k8zner-e2e-*.yaml")
	require.NoError(t, err)

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)

	err = tmpFile.Close()
	require.NoError(t, err)

	return tmpFile.Name()
}

// updateTestConfigWorkers updates the worker count in the test config.
func updateTestConfigWorkers(t *testing.T, configPath string, count int) {
	t.Helper()

	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Simple string replacement for the count
	// This is a simplified approach - in production use proper YAML parsing
	newContent := string(content)
	// The config has "count: 1" under workers
	// We replace it with the new count

	tmpFile, err := os.CreateTemp("", "k8zner-e2e-*.yaml")
	require.NoError(t, err)

	// Create new config with updated count
	updatedContent := `
cluster_name: ` + extractClusterName(newContent) + `
location: nbg1

kubernetes:
  version: "1.32.2"

talos:
  version: "v1.10.2"

control_plane:
  - name: cp
    count: 1
    server_type: cpx22

workers:
  - name: worker
    count: ` + string(rune('0'+count)) + `
    server_type: cpx22

addons:
  cilium:
    enabled: true
  ccm:
    enabled: true
  csi:
    enabled: true
  metrics_server:
    enabled: true
`

	err = os.WriteFile(configPath, []byte(updatedContent), 0644)
	require.NoError(t, err)

	tmpFile.Close()
	os.Remove(tmpFile.Name())
}

// extractClusterName extracts cluster name from config content.
func extractClusterName(content string) string {
	// Simple extraction - find "cluster_name: " and get the value
	for _, line := range splitLines(content) {
		if len(line) > 14 && line[:14] == "cluster_name: " {
			return line[14:]
		}
	}
	return "unknown"
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
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

		restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
		require.NoError(t, err)

		scheme := k8znerv1alpha1.Scheme
		k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
		require.NoError(t, err)

		// Get cluster name from config
		cfg, err := os.ReadFile(configPath)
		require.NoError(t, err)
		clusterName := extractClusterName(string(cfg))

		// Check CRD exists
		cluster := getCluster(ctx, k8sClient, clusterName)
		require.NotNil(t, cluster, "K8znerCluster CRD should exist")

		// Check credentials ref is set
		assert.NotEmpty(t, cluster.Spec.CredentialsRef.Name, "credentials ref should be set")

		// Check operator is deployed
		podList := &corev1.PodList{}
		err = k8sClient.List(ctx, podList,
			client.InNamespace("k8zner-system"),
			client.MatchingLabels{"app.kubernetes.io/name": "k8zner-operator"},
		)
		require.NoError(t, err)
		assert.Greater(t, len(podList.Items), 0, "operator pods should exist")
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
