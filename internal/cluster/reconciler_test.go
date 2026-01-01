package cluster

import (
	"context"
	"testing"

	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/hcloud"
)

func TestReconcile(t *testing.T) {
	// Mock InfrastructureManager
	mockClient := &hcloud.MockClient{}

	reconciler := NewReconciler(mockClient, nil) // nil talos generator for now

	cfg := &config.Config{
		ClusterName: "test-cluster",
		Network: config.NetworkConfig{
			CIDR: "10.0.0.0/16",
		},
	}

	if err := reconciler.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
