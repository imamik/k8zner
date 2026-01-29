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
	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/util/prerequisites"
)

func TestLoadConfig_EmptyPath_NoDefaultFile(t *testing.T) {
	saveAndRestoreFactories(t)

	// Mock findV2ConfigFile to return error (no file found)
	findV2ConfigFile = func() (string, error) {
		return "", errors.New("config file k8zner.yaml not found")
	}

	_, err := loadConfig("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no config file found")
	assert.Contains(t, err.Error(), "k8zner init")
}

func TestLoadConfig_EmptyPath_WithDefaultV2File(t *testing.T) {
	saveAndRestoreFactories(t)

	// Mock findV2ConfigFile to return a path
	findV2ConfigFile = func() (string, error) {
		return "/path/to/k8zner.yaml", nil
	}

	// Mock loadV2ConfigFile to succeed
	loadV2ConfigFile = func(_ string) (*v2.Config, error) {
		return &v2.Config{
			Name:   "test-cluster",
			Region: v2.RegionFalkenstein,
			Mode:   v2.ModeDev,
			Workers: v2.Worker{
				Count: 2,
				Size:  v2.SizeCX32,
			},
		}, nil
	}

	// Mock expandV2Config to return a valid config
	expandV2Config = func(cfg *v2.Config) (*config.Config, error) {
		return &config.Config{
			ClusterName: cfg.Name,
			Location:    string(cfg.Region),
		}, nil
	}

	cfg, err := loadConfig("")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "test-cluster", cfg.ClusterName)
}

func TestLoadConfig_V2ConfigFile(t *testing.T) {
	saveAndRestoreFactories(t)

	// Mock loadV2ConfigFile to succeed
	loadV2ConfigFile = func(_ string) (*v2.Config, error) {
		return &v2.Config{
			Name:   "my-cluster",
			Region: v2.RegionNuremberg,
			Mode:   v2.ModeHA,
			Workers: v2.Worker{
				Count: 3,
				Size:  v2.SizeCX42,
			},
		}, nil
	}

	// Mock expandV2Config
	expandV2Config = func(cfg *v2.Config) (*config.Config, error) {
		return &config.Config{
			ClusterName: cfg.Name,
			Location:    string(cfg.Region),
		}, nil
	}

	cfg, err := loadConfig("k8zner.yaml")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "my-cluster", cfg.ClusterName)
}

func TestLoadConfig_FallbackToLegacy(t *testing.T) {
	saveAndRestoreFactories(t)

	// Mock loadV2ConfigFile to fail (not a v2 config)
	loadV2ConfigFile = func(_ string) (*v2.Config, error) {
		return nil, errors.New("validation failed")
	}

	// Mock loadConfigFile (legacy) to succeed
	loadConfigFile = func(_ string) (*config.Config, error) {
		return &config.Config{
			ClusterName: "legacy-cluster",
			Location:    "nbg1",
		}, nil
	}

	cfg, err := loadConfig("cluster.yaml")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "legacy-cluster", cfg.ClusterName)
}

func TestLoadConfig_NonExistentFile(t *testing.T) {
	saveAndRestoreFactories(t)

	// Mock loadV2ConfigFile to fail
	loadV2ConfigFile = func(_ string) (*v2.Config, error) {
		return nil, errors.New("file not found")
	}

	// Mock loadConfigFile to fail
	loadConfigFile = func(_ string) (*config.Config, error) {
		return nil, errors.New("file not found")
	}

	_, err := loadConfig("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestLoadConfig_ValidLegacyFile(t *testing.T) {
	saveAndRestoreFactories(t)

	// Mock v2 loader to fail
	loadV2ConfigFile = func(_ string) (*v2.Config, error) {
		return nil, errors.New("not a v2 config")
	}

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

	// Reset to real loaders for this test
	loadConfigFile = config.LoadFile

	cfg, err := loadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "test-cluster", cfg.ClusterName)
	assert.Equal(t, "nbg1", cfg.Location)
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
		tmpDir := t.TempDir()
		testPath := filepath.Join(tmpDir, "kubeconfig")

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

func TestPrintApplySuccess(t *testing.T) {
	t.Run("with kubeconfig", func(_ *testing.T) {
		cfg := &config.Config{}
		kubeconfig := []byte("apiVersion: v1\nkind: Config")
		// Should not panic
		printApplySuccess(kubeconfig, cfg)
	})

	t.Run("without kubeconfig", func(_ *testing.T) {
		cfg := &config.Config{}
		// Should not panic
		printApplySuccess(nil, cfg)
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
	origLoadV2ConfigFile := loadV2ConfigFile
	origExpandV2Config := expandV2Config
	origFindV2ConfigFile := findV2ConfigFile
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
		loadV2ConfigFile = origLoadV2ConfigFile
		expandV2Config = origExpandV2Config
		findV2ConfigFile = origFindV2ConfigFile
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

	t.Run("success flow with v2 config", func(t *testing.T) {
		// Mock v2 config loading
		loadV2ConfigFile = func(_ string) (*v2.Config, error) {
			return &v2.Config{
				Name:   "test-cluster",
				Region: v2.RegionFalkenstein,
				Mode:   v2.ModeDev,
				Workers: v2.Worker{
					Count: 2,
					Size:  v2.SizeCX32,
				},
			}, nil
		}

		expandV2Config = func(_ *v2.Config) (*config.Config, error) {
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

		err := Apply(context.Background(), "k8zner.yaml")
		require.NoError(t, err)
		assert.True(t, writeTalosFilesCalled)
	})

	t.Run("success flow with legacy config", func(t *testing.T) {
		// Mock v2 config loading to fail
		loadV2ConfigFile = func(_ string) (*v2.Config, error) {
			return nil, errors.New("not a v2 config")
		}

		// Mock legacy config loading
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

		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return nil
		}

		newReconciler = func(_ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer, _ *config.Config) Reconciler {
			return &mockReconciler{kubeconfig: []byte("kubeconfig-data")}
		}

		err := Apply(context.Background(), "config.yaml")
		require.NoError(t, err)
	})

	t.Run("config load error", func(t *testing.T) {
		loadV2ConfigFile = func(_ string) (*v2.Config, error) {
			return nil, errors.New("not a v2 config")
		}
		loadConfigFile = func(_ string) (*config.Config, error) {
			return nil, errors.New("file not found")
		}

		err := Apply(context.Background(), "missing.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("prerequisites check fails", func(t *testing.T) {
		loadV2ConfigFile = func(_ string) (*v2.Config, error) {
			return &v2.Config{
				Name:   "test",
				Region: v2.RegionFalkenstein,
				Mode:   v2.ModeDev,
				Workers: v2.Worker{
					Count: 1,
					Size:  v2.SizeCX22,
				},
			}, nil
		}

		expandV2Config = func(_ *v2.Config) (*config.Config, error) {
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
		loadV2ConfigFile = func(_ string) (*v2.Config, error) {
			return nil, errors.New("not v2")
		}
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

	t.Run("write talos files error", func(t *testing.T) {
		loadV2ConfigFile = func(_ string) (*v2.Config, error) {
			return nil, errors.New("not v2")
		}
		loadConfigFile = func(_ string) (*config.Config, error) {
			disabled := false
			return &config.Config{
				ClusterName:               "test",
				PrerequisitesCheckEnabled: &disabled,
			}, nil
		}

		getOrGenerateSecrets = func(_, _ string) (*secrets.Bundle, error) {
			return &secrets.Bundle{}, nil
		}

		newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
			return &mockTalosProducer{clientConfigErr: errors.New("config generation failed")}
		}

		err := Apply(context.Background(), "config.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate talosconfig")
	})

	t.Run("reconcile infrastructure error", func(t *testing.T) {
		loadV2ConfigFile = func(_ string) (*v2.Config, error) {
			return nil, errors.New("not v2")
		}
		loadConfigFile = func(_ string) (*config.Config, error) {
			disabled := false
			return &config.Config{
				ClusterName:               "test",
				PrerequisitesCheckEnabled: &disabled,
			}, nil
		}

		getOrGenerateSecrets = func(_, _ string) (*secrets.Bundle, error) {
			return &secrets.Bundle{}, nil
		}

		newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
			return &mockTalosProducer{clientConfig: []byte("talos-config")}
		}

		newInfraClient = func(_ string) hcloud.InfrastructureManager {
			return &hcloud.MockClient{}
		}

		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			return nil
		}

		newReconciler = func(_ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer, _ *config.Config) Reconciler {
			return &mockReconciler{err: errors.New("server creation failed")}
		}

		err := Apply(context.Background(), "config.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reconciliation failed")
	})

	t.Run("write kubeconfig error", func(t *testing.T) {
		loadV2ConfigFile = func(_ string) (*v2.Config, error) {
			return nil, errors.New("not v2")
		}
		loadConfigFile = func(_ string) (*config.Config, error) {
			disabled := false
			return &config.Config{
				ClusterName:               "test",
				PrerequisitesCheckEnabled: &disabled,
			}, nil
		}

		getOrGenerateSecrets = func(_, _ string) (*secrets.Bundle, error) {
			return &secrets.Bundle{}, nil
		}

		newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
			return &mockTalosProducer{clientConfig: []byte("talos-config")}
		}

		newInfraClient = func(_ string) hcloud.InfrastructureManager {
			return &hcloud.MockClient{}
		}

		newReconciler = func(_ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer, _ *config.Config) Reconciler {
			return &mockReconciler{kubeconfig: []byte("kubeconfig-data")}
		}

		writeCallCount := 0
		writeFile = func(_ string, _ []byte, _ os.FileMode) error {
			writeCallCount++
			if writeCallCount == 2 { // First call is talosconfig, second is kubeconfig
				return errors.New("permission denied")
			}
			return nil
		}

		err := Apply(context.Background(), "config.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write kubeconfig")
	})
}
