package main

import (
	"context"
	"fmt"
	"os"
	"log"

	"github.com/hcloud-k8s/hcloud-k8s/pkg/cloud"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/config"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/image"
	"github.com/hcloud-k8s/hcloud-k8s/pkg/talos"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
    "github.com/hetznercloud/hcloud-go/v2/hcloud"
)

var (
	configPath string
    token      string
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to configuration file")
    rootCmd.PersistentFlags().StringVarP(&token, "token", "t", "", "Hetzner Cloud Token")

	rootCmd.AddCommand(applyCmd)
}

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply configuration to create/update cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
        if token == "" {
            token = os.Getenv("HCLOUD_TOKEN")
        }
        if token == "" {
            return fmt.Errorf("HCLOUD_TOKEN is required")
        }

		// 1. Load Config
		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("failed to read config: %w", err)
		}
		var cfg config.ClusterConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}

		ctx := context.Background()
		c := cloud.NewCloud(token, &cfg)
		imgBuilder := image.NewBuilder(token)
		talosGen := talos.NewGenerator(&cfg)

		// 2. Ensure Images
        talosImageURL := fmt.Sprintf("https://factory.talos.dev/image/%s/%s/nocloud-amd64.raw.xz", cfg.TalosSchematicID, cfg.TalosVersion)

        if err := imgBuilder.EnsureImage(ctx, cfg.ClusterName, cfg.TalosVersion, cfg.TalosSchematicID, "x86", "cpx11", cfg.ControlPlane.Location, talosImageURL); err != nil {
            return fmt.Errorf("failed to ensure image: %w", err)
        }
        // Fetch image ID by label
        images, err := c.Client.Image.AllWithOpts(ctx, hcloud.ImageListOpts{
            ListOpts: hcloud.ListOpts{LabelSelector: fmt.Sprintf("cluster=%s,os=talos,talos_version=%s", cfg.ClusterName, cfg.TalosVersion)},
            Architecture: []hcloud.Architecture{hcloud.ArchitectureX86},
        })
        if err != nil || len(images) == 0 {
             return fmt.Errorf("failed to find image")
        }
        imageID := images[0].ID


		// 3. Ensure Network
		netw, err := c.EnsureNetwork(ctx)
		if err != nil {
			return fmt.Errorf("failed to ensure network: %w", err)
		}
        log.Printf("Network ensured: %s", netw.Name)

		// 4. Ensure Firewall
		fw, err := c.EnsureFirewall(ctx)
		if err != nil {
			return fmt.Errorf("failed to ensure firewall: %w", err)
		}
        log.Printf("Firewall ensured: %s", fw.Name)

        // 5. Ensure Load Balancer
        // We create a Load Balancer for the Control Plane to provide a stable endpoint
        lb, err := c.CreateLoadBalancer(ctx, cfg.ClusterName + "-api", cfg.ControlPlane.Location, netw)
        if err != nil {
             return fmt.Errorf("failed to create load balancer: %w", err)
        }
        log.Printf("Load Balancer ensured: %s (IP: %s)", lb.Name, lb.PublicNet.IPv4.IP.String())

        controlPlaneEndpoint := fmt.Sprintf("https://%s:6443", lb.PublicNet.IPv4.IP.String())

        // 5. Ensure Placement Groups
        pgCP, err := c.EnsurePlacementGroup(ctx, cfg.ClusterName + "-controlplane", hcloud.PlacementGroupTypeSpread)
        if err != nil {
            return fmt.Errorf("failed to ensure controlplane placement group: %w", err)
        }

        // 6. Generate Talos Secrets
        secrets, err := talos.GenerateSecrets(cfg.TalosVersion)
        if err != nil {
            return fmt.Errorf("failed to generate secrets: %w", err)
        }

        // 7. Ensure Control Plane Servers
        var cpServers []*hcloud.Server
        for i := 0; i < cfg.ControlPlane.Count; i++ {
            name := fmt.Sprintf("%s-control-%d", cfg.ClusterName, i+1)

            // Generate Config
            // We pass the actual endpoint now
            talosGen.Endpoint = controlPlaneEndpoint

            talosCfg, err := talosGen.Generate("controlplane", name, secrets)
            if err != nil {
                return fmt.Errorf("failed to generate talos config: %w", err)
            }

            cfgBytes, err := yaml.Marshal(talosCfg)
            if err != nil {
                 return fmt.Errorf("failed to marshal talos config: %w", err)
            }

            server, err := c.EnsureServer(ctx, name, cfg.ControlPlane.ServerType, cfg.ControlPlane.Location, imageID, netw, fw, pgCP, string(cfgBytes))
            if err != nil {
                return fmt.Errorf("failed to ensure server %s: %w", name, err)
            }
            log.Printf("Server ensured: %s", name)
            cpServers = append(cpServers, server)
        }

        // Attach Control Plane nodes to Load Balancer
        for _, server := range cpServers {
             _, _, err := c.Client.LoadBalancer.AddServerTarget(ctx, lb, hcloud.LoadBalancerAddServerTargetOpts{
                 Server: server,
                 UsePrivateIP: hcloud.Ptr(true),
             })
             if err != nil {
                 log.Printf("Warning: Failed to add server %s to LB: %v (might already exist)", server.Name, err)
             }
        }

        // 8. Ensure Worker Servers
        pgWorker, err := c.EnsurePlacementGroup(ctx, cfg.ClusterName + "-worker", hcloud.PlacementGroupTypeSpread)
        if err != nil {
             return fmt.Errorf("failed to ensure worker placement group: %w", err)
        }

        for i, workerPool := range cfg.Workers {
            for j := 0; j < workerPool.Count; j++ {
                name := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, workerPool.Name, j+1)
                 talosGen.Endpoint = controlPlaneEndpoint

                 talosCfg, err := talosGen.Generate("worker", name, secrets)
                 if err != nil {
                    return fmt.Errorf("failed to generate talos config for worker: %w", err)
                 }

                 cfgBytes, err := yaml.Marshal(talosCfg)
                 if err != nil {
                     return fmt.Errorf("failed to marshal talos config: %w", err)
                 }

                 _, err = c.EnsureServer(ctx, name, workerPool.ServerType, workerPool.Location, imageID, netw, fw, pgWorker, string(cfgBytes)) // Workers use same image for now
                 if err != nil {
                    return fmt.Errorf("failed to ensure worker %s: %w", name, err)
                 }
                 log.Printf("Worker ensured: %s", name)
            }
            _ = i
        }

        log.Println("Apply completed successfully.")

		return nil
	},
}
