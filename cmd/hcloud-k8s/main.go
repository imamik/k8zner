package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/sak-d/hcloud-k8s/internal/cluster"
	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/image"
	"github.com/sak-d/hcloud-k8s/internal/talos"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "hcloud-k8s",
		Short: "Provision Kubernetes on Hetzner Cloud using Talos",
	}

	var configPath string

	// Apply Command
	var applyCmd = &cobra.Command{
		Use:   "apply",
		Short: "Apply configuration to the cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			// 1. Load Config
			if configPath == "" {
				return fmt.Errorf("config file is required (use --config)")
			}

			cfg, err := config.LoadFile(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Override token from env if present, else expect it in config
			if envToken := os.Getenv("HCLOUD_TOKEN"); envToken != "" {
				cfg.HCloudToken = envToken
			}
			if cfg.HCloudToken == "" {
				return fmt.Errorf("hcloud_token is required (in config or env HCLOUD_TOKEN)")
			}

			log.Printf("Applying configuration for cluster: %s", cfg.ClusterName)

			// 2. Initialize Clients
			hClient := hcloud.NewRealClient(cfg.HCloudToken)

			// 3. Initialize Talos Generator
			// Use local file for secrets persistence
			secretsFile := "secrets.yaml" // TODO: Make configurable or relative to config
			talosGen, err := talos.NewConfigGenerator(
				cfg.ClusterName,
				cfg.Talos.K8sVersion,
				cfg.Talos.Version,
				cfg.ControlPlane.Endpoint,
				secretsFile,
			)
			if err != nil {
				return fmt.Errorf("failed to initialize talos generator: %w", err)
			}

			// 4. Initialize Reconciler
			reconciler := cluster.NewReconciler(hClient, talosGen, cfg)

			// 5. Run Reconcile
			if err := reconciler.ReconcileServers(cmd.Context()); err != nil {
				return fmt.Errorf("reconciliation failed: %w", err)
			}

			// 6. Output Talos Config
			clientCfgBytes, err := talosGen.GetClientConfig()
			if err != nil {
				return fmt.Errorf("failed to generate talosconfig: %w", err)
			}

			talosConfigPath := "talosconfig"
			if err := os.WriteFile(talosConfigPath, clientCfgBytes, 0600); err != nil {
				return fmt.Errorf("failed to write talosconfig: %w", err)
			}

			fmt.Printf("Reconciliation complete!\n")
			fmt.Printf("Secrets saved to: %s\n", secretsFile)
			fmt.Printf("Talos config saved to: %s\n", talosConfigPath)

			return nil
		},
	}
	applyCmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file")
	rootCmd.AddCommand(applyCmd)

	// Image Command
	var imageCmd = &cobra.Command{
		Use:   "image",
		Short: "Manage Talos images",
	}
	rootCmd.AddCommand(imageCmd)

	// Image Build Command
	var (
		imageName    string
		talosVersion string
		arch         string
	)
	var buildCmd = &cobra.Command{
		Use:   "build",
		Short: "Build a new Talos image",
		RunE: func(cmd *cobra.Command, args []string) error {
			token := os.Getenv("HCLOUD_TOKEN")
			if token == "" {
				return fmt.Errorf("HCLOUD_TOKEN environment variable is required")
			}

			client := hcloud.NewRealClient(token)
			builder := image.NewBuilder(client, nil) // use default SSH communicator

			log.Printf("Building image %s (Talos %s, Arch %s)...", imageName, talosVersion, arch)

			snapshotID, err := builder.Build(context.Background(), imageName, talosVersion, arch, nil)
			if err != nil {
				return fmt.Errorf("build failed: %w", err)
			}

			fmt.Printf("Image built successfully! Snapshot ID: %s\n", snapshotID)
			return nil
		},
	}
	buildCmd.Flags().StringVar(&imageName, "name", "talos", "Name of the image to create")
	buildCmd.Flags().StringVar(&talosVersion, "version", "v1.7.0", "Talos version to install")
	buildCmd.Flags().StringVar(&arch, "arch", "amd64", "Architecture (amd64 or arm64)")
	imageCmd.AddCommand(buildCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
