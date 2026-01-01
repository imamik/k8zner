package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/hcloud-k8s/internal/config"
	"github.com/hcloud-k8s/internal/hcloud"
	"github.com/hcloud-k8s/internal/infra"
)

func TestInfraReconciliation(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set")
	}

	runID := fmt.Sprintf("e2e-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Intn(10000))
	clusterName := fmt.Sprintf("test-cluster-%s", runID)

	// Setup Config
	cfg := &config.Config{
		ClusterName: clusterName,
		Hetzner: config.Hetzner{
			Region:      "hel1",
			NetworkZone: "eu-central",
			Firewall: config.Firewall{
				APISource: []string{"0.0.0.0/0"},
			},
		},
	}

	// Init Manager
	client := hcloud.NewClient(token)
	mgr := infra.NewManager(client, cfg)
	ctx := context.Background()

	// Cleanup func
	defer func() {
		t.Log("Cleaning up resources...")
		// Delete Firewall
		fw, _, err := client.Firewall().Get(ctx, clusterName+"-firewall")
		if err == nil && fw != nil {
			client.Firewall().Delete(ctx, fw)
		}
		// Delete Network
		net, _, err := client.Network().Get(ctx, clusterName)
		if err == nil && net != nil {
			client.Network().Delete(ctx, net)
		}
	}()

	// 1. First Pass: Create
	t.Log("Running first pass (creation)...")
	if err := mgr.EnsureNetwork(ctx); err != nil {
		t.Fatalf("First pass EnsureNetwork failed: %v", err)
	}
	if err := mgr.EnsureFirewall(ctx); err != nil {
		t.Fatalf("First pass EnsureFirewall failed: %v", err)
	}

	// Verify existence
	net, _, err := client.Network().Get(ctx, clusterName)
	if err != nil || net == nil {
		t.Errorf("Network not found after creation")
	}
	fw, _, err := client.Firewall().Get(ctx, clusterName+"-firewall")
	if err != nil || fw == nil {
		t.Errorf("Firewall not found after creation")
	}

	// 2. Second Pass: Idempotency (Update)
	t.Log("Running second pass (idempotency)...")
	if err := mgr.EnsureNetwork(ctx); err != nil {
		t.Fatalf("Second pass EnsureNetwork failed: %v", err)
	}

	// Modify config to force a rule update
	cfg.Hetzner.Firewall.APISource = []string{"1.1.1.1/32"}
	if err := mgr.EnsureFirewall(ctx); err != nil {
		t.Fatalf("Second pass EnsureFirewall failed: %v", err)
	}

	// Verify update
	fw, _, _ = client.Firewall().Get(ctx, clusterName+"-firewall")
	foundIP := false
	for _, rule := range fw.Rules {
		for _, ip := range rule.SourceIPs {
			if ip.String() == "1.1.1.1/32" {
				foundIP = true
			}
		}
	}
	if !foundIP {
		t.Error("Firewall rule not updated with new IP")
	}
}
