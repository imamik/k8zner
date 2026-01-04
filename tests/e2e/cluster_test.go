//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/sak-d/hcloud-k8s/internal/cluster"
	"github.com/sak-d/hcloud-k8s/internal/config"
	hcloud_client "github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/talos"
	"github.com/stretchr/testify/assert"
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
					ServerType: "cx23",
					Location:   "hel1",
					Count:      1,
					Image:      "talos",
				},
			},
		},
		Workers: []config.WorkerNodePool{
			{
				Name:       "worker",
				ServerType: "cx23",
				Location:   "hel1",
				Count:      1,
				Image:      "talos",
			},
		},
		Talos: config.TalosConfig{
			Version: "v1.8.3",
		},
		Kubernetes: config.KubernetesConfig{
			Version: "1.30.0",
		},
	}

	// Initialize Real Client
	hClient := hcloud_client.NewRealClient(token)

	cleaner := &ResourceCleaner{t: t}
	defer cleaner.Cleanup()

	// Setup SSH Key
	sshKeyName, _ := setupSSHKey(t, hClient, cleaner, clusterName)
	cfg.SSHKeys = []string{sshKeyName}

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
		hClient.DeletePlacementGroup(ctx, clusterName+"-control-plane-pg")
		hClient.DeletePlacementGroup(ctx, clusterName+"-control-plane")
		hClient.DeletePlacementGroup(ctx, clusterName+"-worker-pg-1")
		logger("Deleted PGs")

		// Delete Certificates
		hClient.DeleteCertificate(ctx, clusterName+"-state")
		logger("Deleted Certificates")
	}
	defer cleanup()

	// Initialize Managers
	// Use real Talos generator for a "real" E2E run
	talosGen, err := talos.NewConfigGenerator(clusterName, cfg.Kubernetes.Version, cfg.Talos.Version, "", "")
	assert.NoError(t, err)

	reconciler := cluster.NewReconciler(hClient, talosGen, cfg)

	// Run Reconcile
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	err = reconciler.Reconcile(ctx)
	assert.NoError(t, err)

	// Verify APIs are reachable
	// We check for Talos API on one of the servers
	cp1IP, err := hClient.GetServerIP(ctx, clusterName+"-control-plane-1")
	assert.NoError(t, err)
	assert.NotEmpty(t, cp1IP)

	// Kube API through Load Balancer
	lb, err := hClient.GetLoadBalancer(ctx, clusterName+"-kube-api")
	assert.NoError(t, err)
	lbIP := lb.PublicNet.IPv4.IP.String()
	assert.NotEmpty(t, lbIP)

	t.Logf("Verifying APIs: Talos=%s:50000, Kube=%s:6443", cp1IP, lbIP)

	// Connectivity Check: Talos API
	if err := waitForPort(cp1IP, 50000, 2*time.Minute); err != nil {
		t.Errorf("Talos API not reachable: %v", err)
	} else {
		t.Log("Talos API is reachable!")
	}

	// Connectivity Check: Kube API
	if err := waitForPort(lbIP, 6443, 10*time.Minute); err != nil {
		t.Errorf("Kube API not reachable: %v", err)
	} else {
		t.Log("Kube API is reachable!")
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
	lb, err = hClient.GetLoadBalancer(ctx, clusterName+"-kube-api")
	assert.NoError(t, err)
	assert.NotNil(t, lb)

	// Check Servers
	srvID, err := hClient.GetServerID(ctx, clusterName+"-control-plane-1")
	assert.NoError(t, err)
	assert.NotEmpty(t, srvID)
}

func waitForPort(ip string, port int, timeout time.Duration) error {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %s", address)
}
