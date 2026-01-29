package handlers

import (
	"context"
	"errors"
	"os"
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

func TestLoadConfig(t *testing.T) {
	t.Run("no config file found", func(t *testing.T) {
		orig := findV2ConfigFile
		defer func() { findV2ConfigFile = orig }()

		findV2ConfigFile = func() (string, error) {
			return "", errors.New("not found")
		}

		_, err := loadConfig("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no config file found")
	})

	t.Run("loads v2 config", func(t *testing.T) {
		origFind := findV2ConfigFile
		origLoad := loadV2ConfigFile
		origExpand := expandV2Config
		defer func() {
			findV2ConfigFile = origFind
			loadV2ConfigFile = origLoad
			expandV2Config = origExpand
		}()

		findV2ConfigFile = func() (string, error) { return "k8zner.yaml", nil }
		loadV2ConfigFile = func(_ string) (*v2.Config, error) {
			return &v2.Config{Name: "test", Region: v2.RegionFalkenstein, Mode: v2.ModeDev, Workers: v2.Worker{Count: 1, Size: v2.SizeCX22}}, nil
		}
		expandV2Config = func(cfg *v2.Config) (*config.Config, error) {
			return &config.Config{ClusterName: cfg.Name}, nil
		}

		cfg, err := loadConfig("")
		require.NoError(t, err)
		assert.Equal(t, "test", cfg.ClusterName)
	})
}

func TestApply(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		origLoad := loadV2ConfigFile
		origExpand := expandV2Config
		origPrereqs := checkDefaultPrereqs
		origInfra := newInfraClient
		origSecrets := getOrGenerateSecrets
		origTalos := newTalosGenerator
		origWrite := writeFile
		origReconciler := newReconciler
		defer func() {
			loadV2ConfigFile = origLoad
			expandV2Config = origExpand
			checkDefaultPrereqs = origPrereqs
			newInfraClient = origInfra
			getOrGenerateSecrets = origSecrets
			newTalosGenerator = origTalos
			writeFile = origWrite
			newReconciler = origReconciler
		}()

		loadV2ConfigFile = func(_ string) (*v2.Config, error) {
			return &v2.Config{Name: "test", Region: v2.RegionFalkenstein, Mode: v2.ModeDev, Workers: v2.Worker{Count: 1, Size: v2.SizeCX22}}, nil
		}
		expandV2Config = func(_ *v2.Config) (*config.Config, error) {
			return &config.Config{ClusterName: "test", Talos: config.TalosConfig{Version: "v1.9.0"}, Kubernetes: config.KubernetesConfig{Version: "1.32.0"}}, nil
		}
		checkDefaultPrereqs = func() *prerequisites.CheckResults {
			return &prerequisites.CheckResults{Results: []prerequisites.CheckResult{{Tool: prerequisites.Tool{Name: "kubectl", Required: true}, Found: true}}}
		}
		newInfraClient = func(_ string) hcloud.InfrastructureManager { return &hcloud.MockClient{} }
		getOrGenerateSecrets = func(_, _ string) (*secrets.Bundle, error) { return &secrets.Bundle{}, nil }
		newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
			return &mockTalosProducer{clientConfig: []byte("talos-config")}
		}
		writeFile = func(_ string, _ []byte, _ os.FileMode) error { return nil }
		newReconciler = func(_ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer, _ *config.Config) Reconciler {
			return &mockReconciler{kubeconfig: []byte("kubeconfig")}
		}

		err := Apply(context.Background(), "k8zner.yaml")
		require.NoError(t, err)
	})
}

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

type mockReconciler struct {
	kubeconfig []byte
	err        error
}

func (m *mockReconciler) Reconcile(_ context.Context) ([]byte, error) { return m.kubeconfig, m.err }
