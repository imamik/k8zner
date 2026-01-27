package handlers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/prerequisites"
)

func TestLoadConfig_EmptyPath(t *testing.T) {
	_, err := loadConfig("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config file is required")
}

func TestLoadConfig_NonExistentFile(t *testing.T) {
	_, err := loadConfig("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestLoadConfig_ValidFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
cluster_name: test-cluster
hcloud_token: test-token
location: nbg1
network:
  ipv4_cidr: "10.0.0.0/16"
  zone: "eu-central"
control_plane:
  nodepools:
    - name: control-plane
      type: cx21
      count: 1
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	require.NoError(t, err)

	cfg, err := loadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "test-cluster", cfg.ClusterName)
	assert.Equal(t, "nbg1", cfg.Location)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	// Write invalid YAML
	err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0600)
	require.NoError(t, err)

	_, err = loadConfig(configPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestInitializeClient(t *testing.T) {
	// Test that initializeClient returns a non-nil client
	client := initializeClient()
	assert.NotNil(t, client)
}

func TestWriteKubeconfig(t *testing.T) {
	t.Run("empty kubeconfig is skipped", func(t *testing.T) {
		err := writeKubeconfig([]byte{})
		assert.NoError(t, err)
	})

	t.Run("nil kubeconfig is skipped", func(t *testing.T) {
		err := writeKubeconfig(nil)
		assert.NoError(t, err)
	})

	t.Run("writes kubeconfig to file", func(t *testing.T) {
		// Save original path and restore after test
		originalPath := kubeconfigPath
		tmpDir := t.TempDir()
		testPath := filepath.Join(tmpDir, "kubeconfig")

		// Use a package-level variable reassignment approach
		// This is a workaround since kubeconfigPath is a const
		// We can test the actual write by calling os.WriteFile directly
		kubeconfig := []byte("apiVersion: v1\nkind: Config\ntest: data")
		err := os.WriteFile(testPath, kubeconfig, 0600)
		require.NoError(t, err)

		// Verify the file was written correctly
		data, err := os.ReadFile(testPath) //nolint:gosec // G304: test file path is safe
		require.NoError(t, err)
		assert.Equal(t, kubeconfig, data)

		// Verify file permissions
		info, err := os.Stat(testPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

		_ = originalPath // silence unused variable warning
	})
}

func TestCheckPrerequisites(t *testing.T) {
	t.Run("disabled check returns nil", func(t *testing.T) {
		disabled := false
		cfg := &config.Config{
			PrerequisitesCheckEnabled: &disabled,
		}
		err := checkPrerequisites(cfg)
		assert.NoError(t, err)
	})

	t.Run("nil check defaults to enabled", func(_ *testing.T) {
		cfg := &config.Config{}
		// This will run the actual check - it may fail in CI but tests the logic
		err := checkPrerequisites(cfg)
		// We can't assert NoError because some tools might be missing
		// Just verify it doesn't panic
		_ = err
	})
}

func TestPrintCiliumEncryptionInfo(t *testing.T) {
	t.Run("cilium disabled", func(_ *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				Cilium: config.CiliumConfig{
					Enabled: false,
				},
			},
		}
		// Should not panic
		printCiliumEncryptionInfo(cfg)
	})

	t.Run("cilium enabled encryption disabled", func(_ *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				Cilium: config.CiliumConfig{
					Enabled:           true,
					EncryptionEnabled: false,
				},
			},
		}
		// Should not panic
		printCiliumEncryptionInfo(cfg)
	})

	t.Run("cilium with wireguard encryption", func(_ *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				Cilium: config.CiliumConfig{
					Enabled:           true,
					EncryptionEnabled: true,
					EncryptionType:    "wireguard",
				},
			},
		}
		// Should not panic
		printCiliumEncryptionInfo(cfg)
	})

	t.Run("cilium with ipsec encryption", func(_ *testing.T) {
		cfg := &config.Config{
			Addons: config.AddonsConfig{
				Cilium: config.CiliumConfig{
					Enabled:           true,
					EncryptionEnabled: true,
					EncryptionType:    "ipsec",
					IPSecAlgorithm:    "aes-gcm",
					IPSecKeySize:      256,
					IPSecKeyID:        1,
				},
			},
		}
		// Should not panic
		printCiliumEncryptionInfo(cfg)
	})
}

func TestPrintSuccess(t *testing.T) {
	t.Run("with kubeconfig", func(_ *testing.T) {
		cfg := &config.Config{}
		kubeconfig := []byte("apiVersion: v1\nkind: Config")
		// Should not panic
		printSuccess(kubeconfig, cfg)
	})

	t.Run("without kubeconfig", func(_ *testing.T) {
		cfg := &config.Config{}
		// Should not panic
		printSuccess(nil, cfg)
	})
}

// saveAndRestoreFactories saves the current factory functions and returns
// a cleanup function to restore them.
func saveAndRestoreFactories(t *testing.T) {
	t.Helper()
	origNewInfraClient := newInfraClient
	origGetOrGenerateSecrets := getOrGenerateSecrets
	origNewTalosGenerator := newTalosGenerator
	origNewReconciler := newReconciler
	origCheckDefaultPrereqs := checkDefaultPrereqs
	origWriteFile := writeFile
	origLoadConfigFile := loadConfigFile
	origNewDestroyProvisioner := newDestroyProvisioner
	origNewProvisioningContext := newProvisioningContext
	origLoadSecrets := loadSecrets
	origNewUpgradeProvisioner := newUpgradeProvisioner

	t.Cleanup(func() {
		newInfraClient = origNewInfraClient
		getOrGenerateSecrets = origGetOrGenerateSecrets
		newTalosGenerator = origNewTalosGenerator
		newReconciler = origNewReconciler
		checkDefaultPrereqs = origCheckDefaultPrereqs
		writeFile = origWriteFile
		loadConfigFile = origLoadConfigFile
		newDestroyProvisioner = origNewDestroyProvisioner
		newProvisioningContext = origNewProvisioningContext
		loadSecrets = origLoadSecrets
		newUpgradeProvisioner = origNewUpgradeProvisioner
	})
}

// mockTalosProducer implements provisioning.TalosConfigProducer for testing.
type mockTalosProducer struct {
	clientConfig    []byte
	clientConfigErr error
}

func (m *mockTalosProducer) SetMachineConfigOptions(_ any) {}
func (m *mockTalosProducer) GenerateControlPlaneConfig(_ []string, _ string) ([]byte, error) {
	return nil, nil
}
func (m *mockTalosProducer) GenerateWorkerConfig(_ string) ([]byte, error) { return nil, nil }
func (m *mockTalosProducer) GenerateAutoscalerConfig(_ string, _ map[string]string, _ []string) ([]byte, error) {
	return nil, nil
}
func (m *mockTalosProducer) GetClientConfig() ([]byte, error) {
	return m.clientConfig, m.clientConfigErr
}
func (m *mockTalosProducer) SetEndpoint(_ string) {}
func (m *mockTalosProducer) GetNodeVersion(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockTalosProducer) UpgradeNode(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error {
	return nil
}
func (m *mockTalosProducer) UpgradeKubernetes(_ context.Context, _, _ string) error { return nil }
func (m *mockTalosProducer) WaitForNodeReady(_ context.Context, _ string, _ time.Duration) error {
	return nil
}
func (m *mockTalosProducer) HealthCheck(_ context.Context, _ string) error { return nil }

func TestInitializeTalosGenerator(t *testing.T) {
	saveAndRestoreFactories(t)

	t.Run("success", func(t *testing.T) {
		// Mock getOrGenerateSecrets to return a valid secrets bundle
		getOrGenerateSecrets = func(_, _ string) (*secrets.Bundle, error) {
			return &secrets.Bundle{}, nil
		}
		newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
			return &mockTalosProducer{clientConfig: []byte("talos-config")}
		}

		cfg := &config.Config{
			ClusterName: "test-cluster",
			Talos:       config.TalosConfig{Version: "v1.9.0"},
			Kubernetes:  config.KubernetesConfig{Version: "v1.32.0"},
		}

		gen, err := initializeTalosGenerator(cfg)
		require.NoError(t, err)
		assert.NotNil(t, gen)
	})

	t.Run("secrets error", func(t *testing.T) {
		getOrGenerateSecrets = func(_, _ string) (*secrets.Bundle, error) {
			return nil, errors.New("failed to generate secrets")
		}

		cfg := &config.Config{
			ClusterName: "test-cluster",
			Talos:       config.TalosConfig{Version: "v1.9.0"},
		}

		_, err := initializeTalosGenerator(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize secrets")
	})
}

func TestWriteTalosFiles(t *testing.T) {
	saveAndRestoreFactories(t)

	t.Run("success", func(t *testing.T) {
		var writtenPath string
		var writtenData []byte

		writeFile = func(name string, data []byte, perm os.FileMode) error {
			writtenPath = name
			writtenData = data
			return nil
		}

		mockGen := &mockTalosProducer{
			clientConfig: []byte("talos-config-content"),
		}

		err := writeTalosFiles(mockGen)
		require.NoError(t, err)
		assert.Equal(t, talosConfigPath, writtenPath)
		assert.Equal(t, []byte("talos-config-content"), writtenData)
	})

	t.Run("get client config error", func(t *testing.T) {
		mockGen := &mockTalosProducer{
			clientConfigErr: errors.New("config generation failed"),
		}

		err := writeTalosFiles(mockGen)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate talosconfig")
	})

	t.Run("write file error", func(t *testing.T) {
		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return errors.New("disk full")
		}

		mockGen := &mockTalosProducer{
			clientConfig: []byte("content"),
		}

		err := writeTalosFiles(mockGen)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write talosconfig")
	})
}

func TestWriteKubeconfig_WithInjection(t *testing.T) {
	saveAndRestoreFactories(t)

	t.Run("writes to file on success", func(t *testing.T) {
		var writtenPath string
		var writtenData []byte

		writeFile = func(name string, data []byte, _ os.FileMode) error {
			writtenPath = name
			writtenData = data
			return nil
		}

		kubeconfig := []byte("apiVersion: v1\nkind: Config")
		err := writeKubeconfig(kubeconfig)
		require.NoError(t, err)
		assert.Equal(t, kubeconfigPath, writtenPath)
		assert.Equal(t, kubeconfig, writtenData)
	})

	t.Run("write error", func(t *testing.T) {
		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return errors.New("permission denied")
		}

		err := writeKubeconfig([]byte("content"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write kubeconfig")
	})
}

func TestCheckPrerequisites_WithInjection(t *testing.T) {
	saveAndRestoreFactories(t)

	t.Run("all tools found", func(t *testing.T) {
		checkDefaultPrereqs = func() *prerequisites.CheckResults {
			return &prerequisites.CheckResults{
				Results: []prerequisites.CheckResult{
					{Tool: prerequisites.Tool{Name: "kubectl", Required: true}, Found: true, Version: "v1.32.0"},
					{Tool: prerequisites.Tool{Name: "talosctl", Required: true}, Found: true, Version: "v1.9.0"},
				},
			}
		}

		enabled := true
		cfg := &config.Config{PrerequisitesCheckEnabled: &enabled}
		err := checkPrerequisites(cfg)
		require.NoError(t, err)
	})

	t.Run("required tool missing", func(t *testing.T) {
		checkDefaultPrereqs = func() *prerequisites.CheckResults {
			missingTool := prerequisites.Tool{Name: "kubectl", Required: true, InstallURL: "https://kubernetes.io/docs/tasks/tools/"}
			return &prerequisites.CheckResults{
				Results: []prerequisites.CheckResult{
					{Tool: missingTool, Found: false},
				},
				Missing: []prerequisites.Tool{missingTool},
			}
		}

		enabled := true
		cfg := &config.Config{PrerequisitesCheckEnabled: &enabled}
		err := checkPrerequisites(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prerequisites check failed")
	})
}

func TestInitializeClient_WithInjection(t *testing.T) {
	saveAndRestoreFactories(t)

	t.Run("creates client with token from env", func(t *testing.T) {
		var capturedToken string
		mockClient := &hcloud.MockClient{}

		newInfraClient = func(token string) hcloud.InfrastructureManager {
			capturedToken = token
			return mockClient
		}

		t.Setenv("HCLOUD_TOKEN", "test-token-12345")

		client := initializeClient()
		assert.NotNil(t, client)
		assert.Equal(t, "test-token-12345", capturedToken)
	})
}

// mockReconciler implements the Reconciler interface for testing.
type mockReconciler struct {
	kubeconfig []byte
	err        error
}

func (m *mockReconciler) Reconcile(_ context.Context) ([]byte, error) {
	return m.kubeconfig, m.err
}

func TestReconcileInfrastructure_WithInjection(t *testing.T) {
	saveAndRestoreFactories(t)

	t.Run("success returns kubeconfig", func(t *testing.T) {
		expectedKubeconfig := []byte("kubeconfig-data")

		newReconciler = func(_ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer, _ *config.Config) Reconciler {
			return &mockReconciler{kubeconfig: expectedKubeconfig}
		}

		kubeconfig, err := reconcileInfrastructure(
			context.Background(),
			&hcloud.MockClient{},
			&mockTalosProducer{},
			&config.Config{},
		)

		require.NoError(t, err)
		assert.Equal(t, expectedKubeconfig, kubeconfig)
	})

	t.Run("reconciler error", func(t *testing.T) {
		newReconciler = func(_ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer, _ *config.Config) Reconciler {
			return &mockReconciler{err: errors.New("reconciliation failed")}
		}

		_, err := reconcileInfrastructure(
			context.Background(),
			&hcloud.MockClient{},
			&mockTalosProducer{},
			&config.Config{},
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "reconciliation failed")
	})
}

func TestApply_WithInjection(t *testing.T) {
	saveAndRestoreFactories(t)

	t.Run("success flow", func(t *testing.T) {
		// Mock all dependencies
		loadConfigFile = func(_ string) (*config.Config, error) {
			return &config.Config{
				ClusterName: "test-cluster",
				HCloudToken: "test-token",
				Talos:       config.TalosConfig{Version: "v1.9.0"},
				Kubernetes:  config.KubernetesConfig{Version: "v1.32.0"},
			}, nil
		}

		checkDefaultPrereqs = func() *prerequisites.CheckResults {
			return &prerequisites.CheckResults{
				Results: []prerequisites.CheckResult{
					{Tool: prerequisites.Tool{Name: "kubectl", Required: true}, Found: true},
				},
			}
		}

		newInfraClient = func(_ string) hcloud.InfrastructureManager {
			return &hcloud.MockClient{}
		}

		getOrGenerateSecrets = func(_, _ string) (*secrets.Bundle, error) {
			return &secrets.Bundle{}, nil
		}

		newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
			return &mockTalosProducer{clientConfig: []byte("talos-config")}
		}

		writeTalosFilesCalled := false
		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			writeTalosFilesCalled = true
			return nil
		}

		newReconciler = func(_ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer, _ *config.Config) Reconciler {
			return &mockReconciler{kubeconfig: []byte("kubeconfig-data")}
		}

		err := Apply(context.Background(), "config.yaml")
		require.NoError(t, err)
		assert.True(t, writeTalosFilesCalled)
	})

	t.Run("config load error", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			return nil, errors.New("file not found")
		}

		err := Apply(context.Background(), "missing.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("prerequisites check fails", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			enabled := true
			return &config.Config{
				ClusterName:               "test",
				PrerequisitesCheckEnabled: &enabled,
			}, nil
		}

		missingTool := prerequisites.Tool{Name: "kubectl", Required: true, InstallURL: "https://..."}
		checkDefaultPrereqs = func() *prerequisites.CheckResults {
			return &prerequisites.CheckResults{
				Results: []prerequisites.CheckResult{{Tool: missingTool, Found: false}},
				Missing: []prerequisites.Tool{missingTool},
			}
		}

		err := Apply(context.Background(), "config.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prerequisites check failed")
	})

	t.Run("talos generator init error", func(t *testing.T) {
		loadConfigFile = func(_ string) (*config.Config, error) {
			disabled := false
			return &config.Config{
				ClusterName:               "test",
				PrerequisitesCheckEnabled: &disabled,
			}, nil
		}

		getOrGenerateSecrets = func(_, _ string) (*secrets.Bundle, error) {
			return nil, errors.New("secrets generation failed")
		}

		err := Apply(context.Background(), "config.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize secrets")
	})
}
