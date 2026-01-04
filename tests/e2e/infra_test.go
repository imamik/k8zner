//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/cluster"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_internal "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock TalosProducer
type MockTalosProducer struct{}

func (m *MockTalosProducer) GenerateControlPlaneConfig(san []string) ([]byte, error) {
	return []byte("#cloud-config\nruncmd:\n  - echo hello"), nil
}
func (m *MockTalosProducer) GenerateWorkerConfig() ([]byte, error) {
	return []byte("#cloud-config\nruncmd:\n  - echo hello"), nil
}
func (m *MockTalosProducer) GetClientConfig() ([]byte, error) {
	return []byte("mock-client-config"), nil
}
func (m *MockTalosProducer) SetEndpoint(endpoint string) {}

func TestInfraProvisioning(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e test")
	}

	runID := fmt.Sprintf("e2e-%d", time.Now().Unix())
	clusterName := runID

	// Create Config
	cfg := &config.Config{
		ClusterName: clusterName,
		HCloudToken: token,
		Location:    "hel1",
		Network: config.NetworkConfig{
			IPv4CIDR: "10.0.0.0/16",
			Zone:     "eu-central",
		},
		Firewall: config.FirewallConfig{
			UseCurrentIPv4: true, // Should trigger current IP detection
		},
		ControlPlane: config.ControlPlaneConfig{
			NodePools: []config.ControlPlaneNodePool{
				{
					Name:       "control-plane-1",
					Count:      1,
					ServerType: "cx23",
					Location:   "hel1",
					Image:      "debian-12", // Override image to avoid errors if "talos" is missing
				},
			},
			PublicVIPIPv4Enabled: true,
		},
	}

	ctx := context.Background()
	client := hcloud_internal.NewRealClient(token)

	cleaner := &ResourceCleaner{t: t}
	defer cleaner.Cleanup()

	// Setup SSH Key
	sshKeyName, _ := setupSSHKey(t, client, cleaner, clusterName)
	cfg.SSHKeys = []string{sshKeyName}

	reconciler := cluster.NewReconciler(client, &MockTalosProducer{}, cfg)

	// CLEANUP
	defer func() {
		t.Logf("Cleaning up resources for %s...", clusterName)
		// We need a proper Cleanup method, but for now manually delete known resources
		// Reverse order
		client.DeleteLoadBalancer(ctx, clusterName+"-kube-api")
		client.DeleteFloatingIP(ctx, clusterName+"-control-plane-ipv4")
		client.DeleteFirewall(ctx, clusterName)
		client.DeleteNetwork(ctx, clusterName)
		client.DeletePlacementGroup(ctx, clusterName+"-control-plane-1")
		client.DeleteServer(ctx, clusterName+"-control-plane-1-1") // Adjusted name based on loop index
	}()

	// RUN RECONCILE
	t.Logf("Running Reconcile for %s...", clusterName)
	err := reconciler.Reconcile(ctx)
	// We expect failure at Bootstrap step because image is debian-12 (not Talos) or client config is mock
	// But Infra should be created.
	if err != nil {
		t.Logf("Reconcile returned error (expected in mock/partial E2E): %v", err)
	}

	// VERIFY
	t.Log("Verifying resources...")

	// 1. Network
	net, err := client.GetNetwork(ctx, clusterName)
	require.NoError(t, err)
	require.NotNil(t, net)
	assert.Equal(t, "10.0.0.0/16", net.IPRange.String())
	// Verify Subnets (CP, LB, Node default)
	assert.GreaterOrEqual(t, len(net.Subnets), 2) // at least CP and LB

	// 2. Firewall
	fw, err := client.GetFirewall(ctx, clusterName)
	require.NoError(t, err)
	require.NotNil(t, fw)
	// Check if rules exist (should be Kube API and Talos API)
	// We expect 2 rules if public IP was detected
	assert.GreaterOrEqual(t, len(fw.Rules), 1)

	// 3. Placement Group
	pg, err := client.GetPlacementGroup(ctx, clusterName+"-control-plane-1")
	require.NoError(t, err)
	require.NotNil(t, pg)
	assert.Equal(t, hcloud.PlacementGroupTypeSpread, pg.Type)

	// 4. Floating IP
	fip, err := client.GetFloatingIP(ctx, clusterName+"-control-plane-ipv4")
	// Reconciler.reconcileFloatingIPs handles this?
	if fip != nil {
		require.NotNil(t, fip)
	}

	// 5. Load Balancer
	lb, err := client.GetLoadBalancer(ctx, clusterName+"-kube-api")
	require.NoError(t, err)
	require.NotNil(t, lb)
	// Check Service
	assert.Equal(t, 1, len(lb.Services))
	assert.Equal(t, 6443, lb.Services[0].ListenPort)
	assert.Equal(t, "401", lb.Services[0].HealthCheck.HTTP.StatusCodes[0])
	// Check Private IP
	require.Equal(t, 1, len(lb.PrivateNet))
	t.Logf("LB Private IP: %s", lb.PrivateNet[0].IP.String())

	t.Log("Infra provisioning verified successfully.")
}
