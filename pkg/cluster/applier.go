package cluster

import (
	"context"
	"fmt"
	"log"

	"github.com/hcloud-k8s/hcloud-k8s/pkg/cloud"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/config"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/talos"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"gopkg.in/yaml.v3"
)

// ImageBuilderClient abstracts the image building process
type ImageBuilderClient interface {
    EnsureImage(ctx context.Context, clusterName, talosVersion, schematicID, arch, serverType, location, imageURL string) error
}

type Applier struct {
	Config       *config.ClusterConfig
	Cloud        *cloud.Cloud
	ImageBuilder ImageBuilderClient
	TalosGen     *talos.Generator
}

func NewApplier(cfg *config.ClusterConfig, c *cloud.Cloud, ib ImageBuilderClient, tg *talos.Generator) *Applier {
	return &Applier{
		Config:       cfg,
		Cloud:        c,
		ImageBuilder: ib,
		TalosGen:     tg,
	}
}

func (a *Applier) Apply(ctx context.Context) error {
	// 1. Ensure Images
	talosImageURL := fmt.Sprintf("https://factory.talos.dev/image/%s/%s/nocloud-amd64.raw.xz", a.Config.TalosSchematicID, a.Config.TalosVersion)

	if err := a.ImageBuilder.EnsureImage(ctx, a.Config.ClusterName, a.Config.TalosVersion, a.Config.TalosSchematicID, "x86", "cpx11", a.Config.ControlPlane.Location, talosImageURL); err != nil {
		return fmt.Errorf("failed to ensure image: %w", err)
	}

	// Fetch image ID by label
	// Using the ImageClient interface from Cloud
	images, err := a.Cloud.Image.AllWithOpts(ctx, hcloud.ImageListOpts{
		ListOpts:     hcloud.ListOpts{LabelSelector: fmt.Sprintf("cluster=%s,os=talos,talos_version=%s", a.Config.ClusterName, a.Config.TalosVersion)},
		Architecture: []hcloud.Architecture{hcloud.ArchitectureX86},
	})
	if err != nil || len(images) == 0 {
		return fmt.Errorf("failed to find image")
	}
	imageID := images[0].ID

	// 2. Ensure Network
	netw, err := a.Cloud.EnsureNetwork(ctx)
	if err != nil {
		return fmt.Errorf("failed to ensure network: %w", err)
	}
	log.Printf("Network ensured: %s", netw.Name)

	// 3. Ensure Firewall
	fw, err := a.Cloud.EnsureFirewall(ctx)
	if err != nil {
		return fmt.Errorf("failed to ensure firewall: %w", err)
	}
	log.Printf("Firewall ensured: %s", fw.Name)

	// 4. Ensure Load Balancer
	lb, err := a.Cloud.CreateLoadBalancer(ctx, a.Config.ClusterName+"-api", a.Config.ControlPlane.Location, netw)
	if err != nil {
		return fmt.Errorf("failed to create load balancer: %w", err)
	}
	log.Printf("Load Balancer ensured: %s (IP: %s)", lb.Name, lb.PublicNet.IPv4.IP.String())

	controlPlaneEndpoint := fmt.Sprintf("https://%s:6443", lb.PublicNet.IPv4.IP.String())

	// 5. Ensure Placement Groups
	pgCP, err := a.Cloud.EnsurePlacementGroup(ctx, a.Config.ClusterName+"-controlplane", hcloud.PlacementGroupTypeSpread)
	if err != nil {
		return fmt.Errorf("failed to ensure controlplane placement group: %w", err)
	}

	// 6. Generate Talos Secrets
	secrets, err := talos.GenerateSecrets(a.Config.TalosVersion)
	if err != nil {
		return fmt.Errorf("failed to generate secrets: %w", err)
	}

	// 7. Ensure Control Plane Servers
	a.TalosGen.Endpoint = controlPlaneEndpoint
	var cpServers []*hcloud.Server
	for i := 0; i < a.Config.ControlPlane.Count; i++ {
		name := fmt.Sprintf("%s-control-%d", a.Config.ClusterName, i+1)

		talosCfg, err := a.TalosGen.Generate("controlplane", name, secrets)
		if err != nil {
			return fmt.Errorf("failed to generate talos config: %w", err)
		}

		cfgBytes, err := yaml.Marshal(talosCfg)
		if err != nil {
			return fmt.Errorf("failed to marshal talos config: %w", err)
		}

		server, err := a.Cloud.EnsureServer(ctx, name, a.Config.ControlPlane.ServerType, a.Config.ControlPlane.Location, imageID, netw, fw, pgCP, string(cfgBytes))
		if err != nil {
			return fmt.Errorf("failed to ensure server %s: %w", name, err)
		}
		log.Printf("Server ensured: %s", name)
		cpServers = append(cpServers, server)
	}

	// Attach Control Plane nodes to Load Balancer
	for _, server := range cpServers {
		_, _, err := a.Cloud.LoadBalancer.AddServerTarget(ctx, lb, hcloud.LoadBalancerAddServerTargetOpts{
			Server:       server,
			UsePrivateIP: hcloud.Ptr(true),
		})
		if err != nil {
			// Check if already attached? The API returns error if already attached.
			// Ideally checking error code or status.
			log.Printf("Warning: Failed to add server %s to LB: %v (might already exist)", server.Name, err)
		}
	}

	// 8. Ensure Worker Servers
	pgWorker, err := a.Cloud.EnsurePlacementGroup(ctx, a.Config.ClusterName+"-worker", hcloud.PlacementGroupTypeSpread)
	if err != nil {
		return fmt.Errorf("failed to ensure worker placement group: %w", err)
	}

	for i, workerPool := range a.Config.Workers {
		for j := 0; j < workerPool.Count; j++ {
			name := fmt.Sprintf("%s-%s-%d", a.Config.ClusterName, workerPool.Name, j+1)

			talosCfg, err := a.TalosGen.Generate("worker", name, secrets)
			if err != nil {
				return fmt.Errorf("failed to generate talos config for worker: %w", err)
			}

			cfgBytes, err := yaml.Marshal(talosCfg)
			if err != nil {
				return fmt.Errorf("failed to marshal talos config: %w", err)
			}

			_, err = a.Cloud.EnsureServer(ctx, name, workerPool.ServerType, workerPool.Location, imageID, netw, fw, pgWorker, string(cfgBytes))
			if err != nil {
				return fmt.Errorf("failed to ensure worker %s: %w", name, err)
			}
			log.Printf("Worker ensured: %s", name)
		}
		_ = i
	}

	log.Println("Apply completed successfully.")
	return nil
}

func (a *Applier) Destroy(ctx context.Context) error {
    log.Println("Starting destruction...")

    // Destroy in reverse order of creation roughly

    // 1. Delete Worker Servers
    for i, workerPool := range a.Config.Workers {
        for j := 0; j < workerPool.Count; j++ {
            name := fmt.Sprintf("%s-%s-%d", a.Config.ClusterName, workerPool.Name, j+1)
            if err := a.Cloud.DeleteServer(ctx, name); err != nil {
                log.Printf("Failed to delete worker server %s: %v", name, err)
            } else {
                log.Printf("Deleted server: %s", name)
            }
        }
        _ = i
    }

    // 2. Delete Worker Placement Group
    if err := a.Cloud.DeletePlacementGroup(ctx, a.Config.ClusterName+"-worker"); err != nil {
        log.Printf("Failed to delete worker placement group: %v", err)
    } else {
        log.Printf("Deleted worker placement group")
    }

    // 3. Delete Control Plane Servers
    for i := 0; i < a.Config.ControlPlane.Count; i++ {
        name := fmt.Sprintf("%s-control-%d", a.Config.ClusterName, i+1)
        if err := a.Cloud.DeleteServer(ctx, name); err != nil {
            log.Printf("Failed to delete control plane server %s: %v", name, err)
        } else {
            log.Printf("Deleted server: %s", name)
        }
    }

    // 4. Delete Control Plane Placement Group
    if err := a.Cloud.DeletePlacementGroup(ctx, a.Config.ClusterName+"-controlplane"); err != nil {
        log.Printf("Failed to delete control plane placement group: %v", err)
    } else {
        log.Printf("Deleted control plane placement group")
    }

    // 5. Delete Load Balancer
    if err := a.Cloud.DeleteLoadBalancer(ctx, a.Config.ClusterName+"-api"); err != nil {
        log.Printf("Failed to delete load balancer: %v", err)
    } else {
        log.Printf("Deleted load balancer")
    }

    // 6. Delete Firewall
    if err := a.Cloud.DeleteFirewall(ctx); err != nil {
        log.Printf("Failed to delete firewall: %v", err)
    } else {
        log.Printf("Deleted firewall")
    }

    // 7. Delete Network
    if err := a.Cloud.DeleteNetwork(ctx); err != nil {
        log.Printf("Failed to delete network: %v", err)
    } else {
        log.Printf("Deleted network")
    }

    log.Println("Destroy completed.")
    return nil
}
