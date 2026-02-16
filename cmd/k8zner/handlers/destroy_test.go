package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
	"github.com/imamik/k8zner/internal/platform/hcloud"
	"github.com/imamik/k8zner/internal/provisioning"
)

func TestDestroy(t *testing.T) {
	t.Parallel()
	origLoad := loadV2ConfigFile
	origExpand := expandV2Config
	origInfra := newInfraClient
	origCtx := newProvisioningContext
	defer func() {
		loadV2ConfigFile = origLoad
		expandV2Config = origExpand
		newInfraClient = origInfra
		newProvisioningContext = origCtx
	}()

	loadV2ConfigFile = func(_ string) (*config.Spec, error) {
		return &config.Spec{Name: "test", Region: config.RegionFalkenstein, Mode: config.ModeDev, Workers: config.WorkerSpec{Count: 1, Size: config.SizeCX22}}, nil
	}
	expandV2Config = func(_ *config.Spec) (*config.Config, error) {
		return &config.Config{ClusterName: "test"}, nil
	}
	newInfraClient = func(_ string) hcloud.InfrastructureManager { return &hcloud.MockClient{} }
	newProvisioningContext = func(_ context.Context, _ *config.Config, _ hcloud.InfrastructureManager, _ provisioning.TalosConfigProducer) *provisioning.Context {
		return &provisioning.Context{
			Config:   &config.Config{ClusterName: "test"},
			Infra:    &hcloud.MockClient{},
			Observer: &provisioning.ConsoleObserver{},
		}
	}

	err := Destroy(context.Background(), "k8zner.yaml")
	require.NoError(t, err)
}
