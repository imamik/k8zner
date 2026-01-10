package cluster

import (
	"context"
	"testing"

	"hcloud-k8s/internal/config"
	hcloud_internal "hcloud-k8s/internal/platform/hcloud"
	"hcloud-k8s/internal/provisioning"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/stretchr/testify/assert"
)

func TestBootstrap_StateMarkerPresent(t *testing.T) {
	mockInfra := new(hcloud_internal.MockClient)
	p := NewProvisioner()

	ctx := context.Background()
	clusterName := "test-cluster"

	// Mock GetCertificate to return a cert (marker exists)
	mockInfra.GetCertificateFunc = func(_ context.Context, name string) (*hcloud.Certificate, error) {
		if name == "test-cluster-state" {
			return &hcloud.Certificate{ID: 123}, nil
		}
		return nil, nil
	}

	observer := provisioning.NewConsoleObserver()
	pCtx := &provisioning.Context{
		Context:  ctx,
		Config:   &config.Config{ClusterName: clusterName},
		State:    provisioning.NewState(),
		Infra:    mockInfra,
		Observer: observer,
		Logger:   observer,
	}
	pCtx.State.ControlPlaneIPs = map[string]string{
		"test-cluster-control-plane-1": "1.2.3.4",
	}
	pCtx.State.TalosConfig = []byte("client-config")

	err := p.BootstrapCluster(pCtx)
	assert.NoError(t, err)
}
