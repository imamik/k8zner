package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
	"github.com/imamik/k8zner/internal/provisioning/upgrade"
)

func TestUpgrade(t *testing.T) {
	origLoad := loadV2ConfigFile
	origExpand := expandV2Config
	origInfra := newInfraClient
	origSecrets := loadSecrets
	origTalos := newTalosGenerator
	origCtx := newProvisioningContext
	origUpgrade := newUpgradeProvisioner
	defer func() {
		loadV2ConfigFile = origLoad
		expandV2Config = origExpand
		newInfraClient = origInfra
		loadSecrets = origSecrets
		newTalosGenerator = origTalos
		newProvisioningContext = origCtx
		newUpgradeProvisioner = origUpgrade
	}()

	loadV2ConfigFile = func(_ string) (*v2.Config, error) {
		return &v2.Config{Name: "test", Region: v2.RegionFalkenstein, Mode: v2.ModeDev, Workers: v2.Worker{Count: 1, Size: v2.SizeCX22}}, nil
	}
	expandV2Config = func(_ *v2.Config) (*config.Config, error) {
		return &config.Config{ClusterName: "test", Talos: config.TalosConfig{Version: "v1.9.0"}, Kubernetes: config.KubernetesConfig{Version: "1.32.0"}}, nil
	}
	newInfraClient = func(_ string) hcloud.InfrastructureManager { return &hcloud.MockClient{} }
	loadSecrets = func(_ string) (*secrets.Bundle, error) { return &secrets.Bundle{}, nil }
	newTalosGenerator = func(_, _, _, _ string, _ *secrets.Bundle) provisioning.TalosConfigProducer {
		return &upgradeMockTalos{}
	}
	newProvisioningContext = func(_ context.Context, _ *config.Config, _ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer) *provisioning.Context {
		return &provisioning.Context{}
	}
	newUpgradeProvisioner = func(_ upgrade.ProvisionerOptions) Provisioner { return &upgradeMock{} }

	err := Upgrade(context.Background(), UpgradeOptions{ConfigPath: "k8zner.yaml"})
	require.NoError(t, err)
}

type upgradeMock struct{}

func (m *upgradeMock) Provision(_ *provisioning.Context) error { return nil }

type upgradeMockTalos struct{}

func (m *upgradeMockTalos) SetMachineConfigOptions(_ any)                                                   {}
func (m *upgradeMockTalos) GenerateControlPlaneConfig(_ []string, _ string) ([]byte, error)                 { return nil, nil }
func (m *upgradeMockTalos) GenerateWorkerConfig(_ string) ([]byte, error)                                   { return nil, nil }
func (m *upgradeMockTalos) GenerateAutoscalerConfig(_ string, _ map[string]string, _ []string) ([]byte, error) { return nil, nil }
func (m *upgradeMockTalos) GetClientConfig() ([]byte, error)                                                { return nil, nil }
func (m *upgradeMockTalos) SetEndpoint(_ string)                                                            {}
func (m *upgradeMockTalos) GetNodeVersion(_ context.Context, _ string) (string, error)                      { return "", nil }
func (m *upgradeMockTalos) UpgradeNode(_ context.Context, _, _ string, _ provisioning.UpgradeOptions) error { return nil }
func (m *upgradeMockTalos) UpgradeKubernetes(_ context.Context, _, _ string) error                          { return nil }
func (m *upgradeMockTalos) WaitForNodeReady(_ context.Context, _ string, _ time.Duration) error             { return nil }
func (m *upgradeMockTalos) HealthCheck(_ context.Context, _ string) error                                   { return nil }
