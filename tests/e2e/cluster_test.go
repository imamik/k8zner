//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/cluster"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_client "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/talos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClusterProvisioning is an end-to-end test that provisions a cluster and verifies resources.
// It requires HCLOUD_TOKEN to be set.
func TestClusterProvisioning(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	// Use a unique cluster name to avoid conflicts
	clusterName := fmt.Sprintf("e2e-test-%d", time.Now().Unix())

	// Create a minimal config
	cfg := &config.Config{
		ClusterName: clusterName,
		HCloudToken: token,
		Location:    "hel1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		Firewall: config.FirewallConfig{
			UseCurrentIPv4: true,
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane",
					ServerType: "cx22",
					Location:   "hel1",
					Count:      1,
					Image:      "debian-12",
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "worker",
				ServerType: "cx22",
				Location:   "hel1",
				Count:      1,
				Image:      "debian-12",
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.7.0",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "1.30.0",
		},
	}

	// Initialize Real Client
	hClient := hcloud_client.NewRealClient(token)

	// Clean up before and after
	cleanup := func() {
		ctx := context.Background()
		logger := func(msg string) { fmt.Printf("[Cleanup] %s\n", msg) }

		// Delete Servers
		hClient.DeleteServer(ctx, clusterName+"-control-plane-1")
		hClient.DeleteServer(ctx, clusterName+"-worker-1")
		logger("Deleted Servers")

		// Delete LBs
		hClient.DeleteLoadBalancer(ctx, clusterName+"-kube-api")
		logger("Deleted LB")

		// Delete Firewalls
		hClient.DeleteFirewall(ctx, clusterName)
		logger("Deleted FW")

		// Delete Networks
		hClient.DeleteNetwork(ctx, clusterName)
		logger("Deleted Network")

		// Delete Placement Groups
		hClient.DeletePlacementGroup(ctx, clusterName+"-worker")

		// Delete Certificates
		hClient.DeleteCertificate(ctx, clusterName+"-state")
	}
	defer cleanup()

	// Initialize Managers
	secretsFile := filepath.Join(t.TempDir(), "secrets.yaml")
	talosGen, err := talos.NewConfigGenerator(
		cfg.ClusterName,
		cfg.Kubernetes.Version,
		cfg.Talos.Version,
		"https://1.1.1.1:6443", // Dummy endpoint
		secretsFile,
	)
	require.NoError(t, err)

	reconciler := cluster.NewReconciler(hClient, talosGen, cfg)

	// Run Reconcile
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	err = reconciler.Reconcile(ctx)
	if err != nil {
		t.Logf("Reconcile returned error (expected as we can't bootstrap debian): %v", err)
	}

	// Verify Resources using Interface Getters

	// Check Network
	net, err := hClient.GetNetwork(ctx, clusterName)
	assert.NoError(t, err)
	assert.NotNil(t, net)

	// Check Firewall
	fw, err := hClient.GetFirewall(ctx, clusterName)
	assert.NoError(t, err)
	assert.NotNil(t, fw)

	// Check LB
	lb, err := hClient.GetLoadBalancer(ctx, clusterName+"-kube-api")
	assert.NoError(t, err)
	assert.NotNil(t, lb)

	// Check Servers
	srvID, err := hClient.GetServerID(ctx, clusterName+"-control-plane-1")
	assert.NoError(t, err)
	assert.NotEmpty(t, srvID)
}
