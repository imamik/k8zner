package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hcloud-k8s/hcloud-k8s/pkg/cloud"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/cluster"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/config"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/image"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/talos"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func TestE2E_FullLifecycle(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping E2E test")
	}

	cfg := &config.ClusterConfig{
		ClusterName:      "e2e-test-cluster-" + fmt.Sprintf("%d", time.Now().Unix()),
		TalosVersion:     "v1.30.0",
		// Use a known valid schematic ID or leave logic to find one?
		// The tool requires one. We should use a default or assume the user provides one valid for v1.30.0.
		// Example: 376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba (default for v1.7.0?)
		// Let's use a dummy one if we can't find one, but the tool will try to download it.
		// Ideally we use a real one.
		TalosSchematicID: "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba", // Placeholder
		ControlPlane: config.NodePool{
			Name:       "control",
			ServerType: "cpx11",
			Location:   "nbg1",
			Count:      3,
		},
		Workers: []config.NodePool{
			{
				Name:       "worker",
				ServerType: "cpx11",
				Location:   "nbg1",
				Count:      1,
			},
		},
	}

	ctx := context.Background()
	c := cloud.NewCloud(token, cfg)
	imgBuilder := image.NewBuilder(token)
	talosGen := talos.NewGenerator(cfg)
	applier := cluster.NewApplier(cfg, c, imgBuilder, talosGen)

	// 1. Apply
	t.Logf("Applying cluster %s...", cfg.ClusterName)
	if err := applier.Apply(ctx); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// 2. Verify Resources
	t.Log("Verifying resources...")
	netw, _, err := c.Client.Network.GetByName(ctx, cfg.ClusterName)
	if err != nil || netw == nil {
		t.Errorf("Network not found")
	}

	servers, _, err := c.Client.Server.List(ctx, hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: "cluster=" + cfg.ClusterName},
	})
	if err != nil {
		t.Errorf("Failed to list servers: %v", err)
	}
	if len(servers) != 4 {
		t.Errorf("Expected 4 servers, got %d", len(servers))
	}

	// 3. Destroy
	t.Log("Destroying cluster...")
	if err := applier.Destroy(ctx); err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}

	// 4. Verify Destruction
	t.Log("Verifying destruction...")
	netw, _, _ = c.Client.Network.GetByName(ctx, cfg.ClusterName)
	if netw != nil {
		t.Error("Network still exists")
	}

	servers, _, _ = c.Client.Server.List(ctx, hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{LabelSelector: "cluster=" + cfg.ClusterName},
	})
	if len(servers) > 0 {
		t.Error("Servers still exist")
	}
}
