package cluster

import (
	"context"
	"testing"

	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/talos"
)

func TestReconcile(t *testing.T) {
	// Mock InfrastructureManager
	mockClient := &hcloud.MockClient{}

	// Create a minimal ConfigGenerator (requires creating a dummy one)
	tGen := talos.NewConfigGenerator("test-cluster", "1.2.3.4")

	reconciler := NewReconciler(mockClient, tGen)

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Network: config.NetworkConfig{
			CIDR: "10.0.0.0/16",
		},
		ControlPlane: config.ControlPlane{
			Count:      1,
			ServerType: "cx21",
			Image:      "debian-11",
		},
	}

	if err := cfg.CalculateSubnets(); err != nil {
		t.Fatalf("failed to calc subnets: %v", err)
	}

	if err := reconciler.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
