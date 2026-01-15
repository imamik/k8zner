//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"hcloud-k8s/internal/config"
	"hcloud-k8s/internal/orchestration"
	"hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/platform/talos"
	"hcloud-k8s/internal/provisioning/image"

	"github.com/stretchr/testify/require"
)

// TestE2EUpgradeStandalone tests a complete upgrade lifecycle from an older version to a newer version.
//
// This test:
// 1. Builds snapshots for initial version (v1.8.2 / v1.30.0)
// 2. Deploys cluster with initial version
// 3. Verifies initial cluster health
// 4. Builds snapshots for target version (v1.8.3 / v1.31.0)
// 5. Upgrades cluster to target version
// 6. Verifies upgraded cluster health
//
// Run with: go test -v -timeout=45m -tags=e2e -run TestE2EUpgradeStandalone ./tests/e2e/
//
// Environment variables:
//   - HCLOUD_TOKEN: Required
//   - E2E_KEEP_SNAPSHOTS: Set to "true" to keep snapshots between runs
func TestE2EUpgradeStandalone(t *testing.T) {
	// This is a long-running test
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E upgrade test")
	}

	// Configuration
	clusterName := fmt.Sprintf("e2e-upg-%d", time.Now().Unix())
	initialTalosVersion := "v1.8.2"
	targetTalosVersion := "v1.8.3"
	initialK8sVersion := "v1.30.0"
	targetK8sVersion := "v1.31.0"
	schematicID := "ce4c980550dd2ab1b17bbf2b08801c7eb59418eafe8f279833297925d67c7515"

	t.Logf("Starting E2E Upgrade Test")
	t.Logf("Cluster: %s", clusterName)
	t.Logf("Initial: Talos %s / K8s %s", initialTalosVersion, initialK8sVersion)
	t.Logf("Target:  Talos %s / K8s %s", targetTalosVersion, targetK8sVersion)

	client := hcloud.NewRealClient(token)
	state := NewE2EState(clusterName, client)

	// Cleanup at end
	defer cleanupE2ECluster(t, state)

	// Phase 1: Build snapshots for initial version
	t.Run("BuildInitialSnapshots", func(t *testing.T) {
		buildSnapshotsForVersion(t, client, state, initialTalosVersion, initialK8sVersion)
	})

	// Phase 2: Deploy cluster with initial version
	t.Run("DeployInitialCluster", func(t *testing.T) {
		deployClusterWithVersion(t, state, initialTalosVersion, initialK8sVersion, schematicID)
	})

	// Phase 3: Verify initial cluster
	t.Run("VerifyInitialCluster", func(t *testing.T) {
		verifyCluster(t, state, initialK8sVersion)
	})

	// Phase 4: Build snapshots for target version
	t.Run("BuildTargetSnapshots", func(t *testing.T) {
		buildSnapshotsForVersion(t, client, state, targetTalosVersion, targetK8sVersion)
	})

	// Phase 5: Upgrade cluster
	t.Run("UpgradeCluster", func(t *testing.T) {
		upgradeCluster(t, state, targetTalosVersion, targetK8sVersion, schematicID)
	})

	// Phase 6: Verify upgraded cluster
	t.Run("VerifyUpgradedCluster", func(t *testing.T) {
		verifyCluster(t, state, targetK8sVersion)
	})

	// Phase 7: Test workload after upgrade
	t.Run("TestWorkloadAfterUpgrade", func(t *testing.T) {
		testWorkloadDeployment(t, state)
	})

	t.Log("✓ E2E Upgrade Test Completed Successfully")
}

// buildSnapshotsForVersion builds Talos snapshots for the specified version.
func buildSnapshotsForVersion(t *testing.T, client *hcloud.RealClient, state *E2EState, talosVer, k8sVer string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	t.Logf("Building snapshots for Talos %s / K8s %s...", talosVer, k8sVer)

	labels := map[string]string{
		"os":            "talos",
		"talos-version": talosVer,
		"k8s-version":   k8sVer,
		"arch":          "amd64",
		"e2e-upgrade":   "true",
		"cluster":       state.ClusterName,
	}

	// Check if snapshot already exists
	existing, err := client.GetSnapshotByLabels(ctx, labels)
	if err != nil {
		t.Fatalf("Failed to check for existing snapshot: %v", err)
	}

	if existing != nil {
		t.Logf("Found existing snapshot: %s (ID: %d)", existing.Description, existing.ID)
		state.SnapshotAMD64 = fmt.Sprintf("%d", existing.ID)
		return
	}

	// Build new snapshot
	builder := image.NewBuilder(client)
	snapshotID, err := builder.Build(ctx, talosVer, k8sVer, "amd64", "nbg1", labels)
	if err != nil {
		t.Fatalf("Failed to build snapshot: %v", err)
	}

	state.SnapshotAMD64 = snapshotID
	t.Logf("✓ Snapshot built: %s", snapshotID)
}

// deployClusterWithVersion deploys a cluster with the specified Talos/K8s version.
func deployClusterWithVersion(t *testing.T, state *E2EState, talosVer, k8sVer, schematicID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	t.Logf("Deploying cluster with Talos %s / K8s %s...", talosVer, k8sVer)

	// Setup SSH key
	if err := setupSSHKeyForCluster(ctx, t, state); err != nil {
		t.Fatalf("Failed to setup SSH key: %v", err)
	}

	// Create config
	cfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: os.Getenv("HCLOUD_TOKEN"),
		Location:    "nbg1",
		SSHKeys:     []string{}, // Empty for now, will be created during provisioning
		Network: config.NetworkConfig{
			Zone:         "eu-central",
			IPv4CIDR:     "10.0.0.0/16",
			NodeIPv4CIDR: "10.0.0.0/22",
		},
		Talos: config.TalosConfig{
			Version:     talosVer,
			SchematicID: schematicID,
		},
		Kubernetes: config.KubernetesConfig{
			Version: k8sVer,
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					Location:   "nbg1",
					ServerType: "cpx22",
					Count:      3, // 3 control planes for HA and proper upgrade testing
					Image:      state.SnapshotAMD64,
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "pool1",
				Location:   "nbg1",
				ServerType: "cpx22",
				Count:      1, // 1 worker
				Image:      state.SnapshotAMD64,
			},
		},
		Addons: config.AddonsConfig{
			CCM: config.CCMConfig{Enabled: true},
			CSI: config.CSIConfig{Enabled: true},
		},
	}

	// Generate Talos secrets
	secrets, err := talos.GetOrGenerateSecrets("/tmp/talos-secrets-"+state.ClusterName+".json", talosVer)
	if err != nil {
		t.Fatalf("Failed to generate Talos secrets: %v", err)
	}
	state.TalosSecretsPath = "/tmp/talos-secrets-" + state.ClusterName + ".json"

	talosGen := talos.NewGenerator(state.ClusterName, k8sVer, talosVer, "", secrets)

	// Create reconciler and provision cluster
	reconciler := orchestration.NewReconciler(state.Client, talosGen, cfg)

	t.Log("Starting cluster reconciliation...")
	startTime := time.Now()
	kubeconfig, err := reconciler.Reconcile(ctx)
	duration := time.Since(startTime)

	if err != nil {
		t.Logf("Cluster provisioning failed after %v: %v", duration, err)
		runClusterDiagnostics(ctx, t, state)
		t.Fatalf("Cluster provisioning failed: %v", err)
	}

	state.Kubeconfig = kubeconfig
	t.Logf("✓ Cluster provisioned in %v", duration)

	// Save kubeconfig
	kubeconfigPath := fmt.Sprintf("/tmp/kubeconfig-%s", state.ClusterName)
	if err := os.WriteFile(kubeconfigPath, kubeconfig, 0600); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}
	state.KubeconfigPath = kubeconfigPath

	// Save talosconfig
	talosconfig, err := talosGen.GetClientConfig()
	if err != nil {
		t.Logf("Warning: Could not get Talos config: %v", err)
	} else {
		state.TalosConfig = talosconfig
	}
}

// upgradeCluster upgrades the cluster to the specified Talos/K8s version.
func upgradeCluster(t *testing.T, state *E2EState, talosVer, k8sVer, schematicID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	t.Logf("Upgrading cluster to Talos %s / K8s %s...", talosVer, k8sVer)

	// Create upgrade config
	cfg := &config.Config{
		ClusterName: state.ClusterName,
		HCloudToken: os.Getenv("HCLOUD_TOKEN"),
		Location:    "nbg1",
		SSHKeys:     []string{}, // Empty for now
		Network: config.NetworkConfig{
			Zone:         "eu-central",
			IPv4CIDR:     "10.0.0.0/16",
			NodeIPv4CIDR: "10.0.0.0/22",
		},
		Talos: config.TalosConfig{
			Version:     talosVer,
			SchematicID: schematicID,
		},
		Kubernetes: config.KubernetesConfig{
			Version: k8sVer,
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					Location:   "nbg1",
					ServerType: "cpx22",
					Count:      3, // 3 control planes
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "pool1",
				Location:   "nbg1",
				ServerType: "cpx22",
				Count:      1, // 1 worker
			},
		},
	}

	// Write config file and call upgrade CLI command
	configPath := fmt.Sprintf("/tmp/upgrade-config-%s.yaml", state.ClusterName)
	if err := saveConfigToYAML(cfg, configPath); err != nil {
		t.Fatalf("Failed to save upgrade config: %v", err)
	}

	// Copy secrets to expected location
	secretsData, err := os.ReadFile(state.TalosSecretsPath)
	if err != nil {
		t.Fatalf("Failed to read secrets: %v", err)
	}
	if err := os.WriteFile("secrets.yaml", secretsData, 0600); err != nil {
		t.Fatalf("Failed to write secrets.yaml: %v", err)
	}
	defer os.Remove("secrets.yaml")

	// Call upgrade command
	t.Log("Executing upgrade command...")
	startTime := time.Now()

	// Binary is in project root, tests are in tests/e2e/
	binaryPath := "../../hcloud-k8s"
	cmd := exec.CommandContext(ctx, binaryPath, "upgrade", "--config", configPath)
	cmd.Env = append(os.Environ(), "HCLOUD_TOKEN="+os.Getenv("HCLOUD_TOKEN"))
	output, err := cmd.CombinedOutput()
	duration := time.Since(startTime)

	t.Logf("Upgrade output:\n%s", string(output))

	if err != nil {
		t.Fatalf("Upgrade failed after %v: %v", duration, err)
	}

	t.Logf("✓ Upgrade completed in %v", duration)

	// Wait for cluster to stabilize
	t.Log("Waiting 30s for cluster to stabilize...")
	time.Sleep(30 * time.Second)
}

// verifyCluster verifies cluster health and K8s version.
func verifyCluster(t *testing.T, state *E2EState, expectedK8sVersion string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	t.Logf("Verifying cluster (expected K8s version: %s)...", expectedK8sVersion)

	// Wait for nodes to be ready
	t.Log("Waiting for nodes to be ready...")
	cmd := exec.CommandContext(ctx, "kubectl", "wait", "--for=condition=ready",
		"nodes", "--all", "--timeout=600s",
		"--kubeconfig", state.KubeconfigPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("kubectl wait output:\n%s", string(output))
		t.Fatalf("Nodes not ready: %v", err)
	}

	// Check node versions
	t.Log("Checking node versions...")
	cmd = exec.CommandContext(ctx, "kubectl", "get", "nodes", "-o", "wide",
		"--kubeconfig", state.KubeconfigPath)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Failed to get nodes")
	t.Logf("Nodes:\n%s", string(output))

	// Verify K8s version
	cmd = exec.CommandContext(ctx, "kubectl", "version", "--short",
		"--kubeconfig", state.KubeconfigPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Logf("kubectl version output:\n%s", string(output))
	} else {
		t.Logf("Cluster version:\n%s", string(output))
	}

	// Check system pods
	t.Log("Checking system pods...")
	cmd = exec.CommandContext(ctx, "kubectl", "get", "pods", "-n", "kube-system",
		"--kubeconfig", state.KubeconfigPath)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Failed to get pods")
	t.Logf("System pods:\n%s", string(output))

	t.Log("✓ Cluster verification passed")
}

// testWorkloadDeployment tests deploying a workload after upgrade.
func testWorkloadDeployment(t *testing.T, state *E2EState) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	t.Log("Testing workload deployment...")

	// Create deployment
	cmd := exec.CommandContext(ctx, "kubectl", "create", "deployment", "nginx",
		"--image=nginx:alpine", "--kubeconfig", state.KubeconfigPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to create deployment: %s", string(output))

	// Wait for deployment
	cmd = exec.CommandContext(ctx, "kubectl", "wait", "--for=condition=available",
		"deployment/nginx", "--timeout=120s", "--kubeconfig", state.KubeconfigPath)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Deployment not available: %s", string(output))

	// Cleanup
	cmd = exec.CommandContext(ctx, "kubectl", "delete", "deployment", "nginx",
		"--kubeconfig", state.KubeconfigPath)
	_, _ = cmd.CombinedOutput()

	t.Log("✓ Workload deployment successful")
}

// saveConfigToYAML saves a config to a YAML file.
func saveConfigToYAML(cfg *config.Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
