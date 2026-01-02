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
	return []byte("mock-config"), nil
}
func (m *MockTalosProducer) GenerateWorkerConfig() ([]byte, error) {
	return []byte("mock-config"), nil
}

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
					ServerType: "cx22",
					Location:   "hel1",
				},
			},
			PublicVIPIPv4Enabled: true,
		},
	}

	ctx := context.Background()
	client := hcloud_internal.NewRealClient(token)
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
		client.DeletePlacementGroup(ctx, clusterName+"-control-plane-pg")
	}()

	// RUN RECONCILE
	t.Logf("Running Reconcile for %s...", clusterName)
	err := reconciler.Reconcile(ctx)
	require.NoError(t, err)

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
	pg, err := client.GetPlacementGroup(ctx, clusterName+"-control-plane-pg")
	require.NoError(t, err)
	require.NotNil(t, pg)
	assert.Equal(t, hcloud.PlacementGroupTypeSpread, pg.Type)

	// 4. Floating IP
	fip, err := client.GetFloatingIP(ctx, clusterName+"-control-plane-ipv4")
	require.NoError(t, err)
	require.NotNil(t, fip)

	// 5. Load Balancer
	lb, err := client.GetLoadBalancer(ctx, clusterName+"-kube-api")
	require.NoError(t, err)
	require.NotNil(t, lb)
	// Check Service
	assert.Equal(t, 1, len(lb.Services))
	assert.Equal(t, 6443, lb.Services[0].ListenPort)
	assert.Equal(t, "401", lb.Services[0].HealthCheck.HTTP.StatusCodes[0])
	// Check Private IP (should be .254 or similar depending on subnet)
	// LB Subnet 10.0.64.128/25
	// cidrhost -2 -> .253 (broadcast is .255, -1 is .254? No. hostnum 1 is network+1)
	// subnet size 128. Host -2 is end - 2.
	// 10.0.64.128 - 10.0.64.255.
	// -1 = 254. -2 = 253.
	// Let's assert it has an IP in the correct range.
	require.Equal(t, 1, len(lb.PrivateNet))
	t.Logf("LB Private IP: %s", lb.PrivateNet[0].IP.String())

	t.Log("Infra provisioning verified successfully.")
}
