package cluster

import (
	"context"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/stretchr/testify/assert"
)

func TestBootstrap_StateMarkerPresent(t *testing.T) {
	mockInfra := new(hcloud_internal.MockClient)
	bootstrapper := NewBootstrapper(mockInfra)

	ctx := context.Background()
	clusterName := "test-cluster"

	// Mock GetCertificate to return a cert (marker exists)
	mockInfra.GetCertificateFunc = func(_ context.Context, name string) (*hcloud.Certificate, error) {
		if name == "test-cluster-state" {
			return &hcloud.Certificate{ID: 123}, nil
		}
		return nil, nil
	}

	err := bootstrapper.Bootstrap(ctx, clusterName, "1.2.3.4", []byte("config"))
	assert.NoError(t, err)
}
