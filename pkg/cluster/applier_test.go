package cluster

import (
	"context"
	"testing"

	"github.com/hcloud-k8s/hcloud-k8s/pkg/cloud"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/cloud/fakes"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/config"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/talos"
    "github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type FakeImageBuilder struct {}

func (f *FakeImageBuilder) EnsureImage(ctx context.Context, clusterName, talosVersion, schematicID, arch, serverType, location, imageURL string) error {
    return nil
}

func TestApplier_Apply_Integration(t *testing.T) {
	// Setup Fakes
	fakeNetwork := fakes.NewFakeNetworkClient()
	fakeFirewall := fakes.NewFakeFirewallClient()
	fakeLB := fakes.NewFakeLoadBalancerClient()
	fakeServer := fakes.NewFakeServerClient()
	fakeImage := fakes.NewFakeImageClient()
	fakePG := fakes.NewFakePlacementGroupClient()
	fakeAction := &fakes.FakeActionClient{}

    // Pre-populate an image so finding it succeeds
    fakeImage.Images = append(fakeImage.Images, &hcloud.Image{ID: 12345, Description: "Talos Linux AMD64 for test-cluster"})

	cfg := &config.ClusterConfig{
		ClusterName:      "test-cluster",
		TalosVersion:     "v1.30.0",
		TalosSchematicID: "schem123",
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
				Count:      2,
			},
		},
	}

	c := &cloud.Cloud{
        Config:         cfg, // Injected Config
		Network:        fakeNetwork,
		Firewall:       fakeFirewall,
		LoadBalancer:   fakeLB,
		Server:         fakeServer,
		Image:          fakeImage,
		PlacementGroup: fakePG,
		Action:         fakeAction,
	}

    talosGen := talos.NewGenerator(cfg)
    imgBuilder := &FakeImageBuilder{}

    applier := NewApplier(cfg, c, imgBuilder, talosGen)

    // Run Apply
    if err := applier.Apply(context.Background()); err != nil {
        t.Fatalf("Apply failed: %v", err)
    }

    // Verifications

    // 1. Network Created
    netw, _, _ := fakeNetwork.GetByName(context.Background(), "test-cluster")
    if netw == nil {
        t.Error("Network was not created")
    }

    // 2. Firewall Created
    fw, _, _ := fakeFirewall.GetByName(context.Background(), "test-cluster")
    if fw == nil {
        t.Error("Firewall was not created")
    }

    // 3. Load Balancer Created
    lb, _, _ := fakeLB.GetByName(context.Background(), "test-cluster-api")
    if lb == nil {
        t.Error("Load Balancer was not created")
    }

    // 4. Control Plane Servers Created
    // FakeServerClient keeps track of all created servers.
    // 3 CP + 2 Workers = 5
    if len(fakeServer.Servers) != 5 {
        t.Errorf("Expected 5 servers, got %d", len(fakeServer.Servers))
    }

    // 5. Placement Groups Created
    pgCP, _, _ := fakePG.GetByName(context.Background(), "test-cluster-controlplane")
    if pgCP == nil {
        t.Error("ControlPlane Placement Group was not created")
    }
    pgWk, _, _ := fakePG.GetByName(context.Background(), "test-cluster-worker")
    if pgWk == nil {
        t.Error("Worker Placement Group was not created")
    }
}
