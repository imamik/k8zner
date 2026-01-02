//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/cluster"
	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/keygen"
)

// MockTalosProducer implements TalosConfigProducer for testing.
type MockTalosProducer struct {
	ValidCloudInit bool
}

func (m *MockTalosProducer) GenerateControlPlaneConfig(san []string) ([]byte, error) {
	if m.ValidCloudInit {
		return []byte("#cloud-config\npackage_update: true"), nil
	}
	return []byte("invalid-yaml"), nil
}

func (m *MockTalosProducer) GenerateWorkerConfig() ([]byte, error) {
	if m.ValidCloudInit {
		return []byte("#cloud-config\npackage_update: true"), nil
	}
	return []byte("invalid-yaml"), nil
}

func TestClusterApply(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	client := hcloud.NewRealClient(token)
	ctx := context.Background()

	// 1. Setup SSH Key
	sshKeyName := fmt.Sprintf("e2e-cluster-key-%s", time.Now().Format("20060102-150405"))
	t.Logf("Generating SSH key %s...", sshKeyName)
	keyPair, err := keygen.GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	_, err = client.CreateSSHKey(ctx, sshKeyName, string(keyPair.PublicKey))
	if err != nil {
		t.Fatalf("Failed to upload ssh key: %v", err)
	}

	defer func() {
		t.Logf("Deleting SSH key %s...", sshKeyName)
		if err := client.DeleteSSHKey(context.Background(), sshKeyName); err != nil {
			t.Errorf("Failed to delete ssh key %s: %v", sshKeyName, err)
		}
	}()

	// Unique cluster name per test
	clusterName := fmt.Sprintf("e2e-cluster-%s", time.Now().Format("20060102-150405"))

	// Config
	cfg := &config.Config{
		ClusterName: clusterName,
		HCloudToken: token,
		Location:    "hel1", // Helsinki
		SSHKeys:     []string{sshKeyName}, // Pass the generated key
		ControlPlane: config.ControlPlaneConfig{
			Endpoint:   "https://127.0.0.1:6443", // Dummy endpoint
			Image:      "debian-12",              // Known image
			ServerType: "cx23",                   // Available Intel instance
		},
		Talos: config.TalosConfig{
			Version:    "v1.7.0",
			K8sVersion: "v1.30.0",
		},
	}

	// Cleanup defer
	defer func() {
		t.Logf("Cleaning up cluster resources for %s...", clusterName)
		for i := 1; i <= 3; i++ {
			name := fmt.Sprintf("%s-control-plane-%d", clusterName, i)
			t.Logf("Deleting server %s...", name)
			_ = client.DeleteServer(ctx, name)
		}
		// Cleanup local secrets file if generated (though Mock doesn't use it, real app would)
		// Here we use MockTalosProducer, so no secrets file is created by it.
	}()

	// Use Mock Talos Generator that returns valid cloud-init for Debian image
	talosGen := &MockTalosProducer{ValidCloudInit: true}

	// Init Reconciler
	reconciler := cluster.NewReconciler(client, talosGen, cfg)

	// Run Reconcile
	t.Logf("Reconciling servers for %s...", clusterName)
	err = reconciler.ReconcileServers(ctx)
	if err != nil {
		t.Fatalf("ReconcileServers failed: %v", err)
	}

	// Verify
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("%s-control-plane-%d", clusterName, i)
		id, err := client.GetServerID(ctx, name)
		if err != nil {
			t.Errorf("Failed to find server %s: %v", name, err)
		} else {
			t.Logf("Server %s exists with ID %s", name, id)
		}
	}

	// Idempotency check
	t.Log("Running ReconcileServers again (idempotency check)...")
	err = reconciler.ReconcileServers(ctx)
	if err != nil {
		t.Errorf("Second run of ReconcileServers failed: %v", err)
	}
}
