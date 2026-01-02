package cluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/talos"
)

func TestReconcileControlPlane(t *testing.T) {
	// Setup Mocks
	mockProvisioner := &hcloud.MockClient{}

	// Config
	cfg := &config.Config{
		ClusterName: "test-cluster",
	}

	// Talos Generator
	gen, err := talos.NewConfigGenerator("test-cluster", "v1.30.0", "v1.7.0", "https://api:6443", "secrets.yaml")
	assert.NoError(t, err)

	reconciler := NewReconciler(mockProvisioner, gen, cfg)

	// Expectations
	// We expect 3 control plane nodes to be created.
	// We verify GetServerID is called (and returns error -> not found)
	// Then CreateServer is called.

	mockProvisioner.GetServerIDFunc = func(ctx context.Context, name string) (string, error) {
		return "", assert.AnError // Simulate not found
	}

	callCount := 0
	mockProvisioner.CreateServerFunc = func(ctx context.Context, name, imageType, serverType string, sshKeys []string, labels map[string]string, userData string) (string, error) {
		callCount++
		// Verify basic checks
		assert.Contains(t, name, "test-cluster-control-plane-")
		// Verify UserData is present (simple check for now)
		assert.NotEmpty(t, userData)
		return "server-id", nil
	}

	// Run
	err = reconciler.ReconcileServers(context.Background())
	assert.NoError(t, err)

	// Assertions
	assert.Equal(t, 3, callCount)
}

func TestReconcileControlPlane_Idempotency(t *testing.T) {
	mockProvisioner := &hcloud.MockClient{}
	cfg := &config.Config{ClusterName: "test-cluster"}
	gen, _ := talos.NewConfigGenerator("test-cluster", "v1.30.0", "v1.7.0", "https://api:6443", "secrets.yaml")
	reconciler := NewReconciler(mockProvisioner, gen, cfg)

	// Simulate servers ALREADY exist
	mockProvisioner.GetServerIDFunc = func(ctx context.Context, name string) (string, error) {
		return "existing-id", nil
	}

	createCalled := false
	mockProvisioner.CreateServerFunc = func(ctx context.Context, name, imageType, serverType string, sshKeys []string, labels map[string]string, userData string) (string, error) {
		createCalled = true
		return "server-id", nil
	}

	err := reconciler.ReconcileServers(context.Background())
	assert.NoError(t, err)
	assert.False(t, createCalled, "CreateServer should not be called if server exists")
}
