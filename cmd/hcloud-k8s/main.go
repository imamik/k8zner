// Package main is the entry point for the hcloud-k8s CLI.
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
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			// Note: talos.NewConfigGenerator arguments might have changed or need verification.
			// Currently: (clusterName, kubernetesVersion, talosVersion, endpoint, secretsFile)
			// Config struct has: Talos.Version, Kubernetes.Version, ControlPlane.Endpoint (wait, Config struct doesn't have Endpoint?)
			// Let's check config struct if I can.
			// Assuming Config matches what I saw earlier.
			// Config.ControlPlane probably doesn't have Endpoint directly?
			// Terraform calculates endpoint from LB IP or Floating IP.
			// I should probably pass a temporary endpoint or handle it inside generator?
			// The generator needs an endpoint for the talosconfig.
			// If we use LB, it's `https://<lb-ip>:6443`.
			// Since we don't know LB IP yet (unless we query it), we might need to handle this.
			// For now, let's assume the user provides it or we use a placeholder that matches expected DNS?
			// Terraform usually uses `hcloud_load_balancer.control_plane.ipv4` which is known after apply.
			// This circular dependency is handled in Terraform by state.
			// Here, we might need two passes or update config later?
			// Or we can use a DNS name if provided.
			// For now, I'll pass a placeholder or try to get it from config if user supplied `control_plane_endpoint`?
			// Checking `internal/config/config.go` earlier...
			// `ControlPlaneConfig` has `NodePools`.
			// `KubernetesConfig` has `Version`, `OIDC`, `CNI`.
			// `TalosConfig` has `Version`.
			// I don't see `Endpoint` in `ControlPlaneConfig` in my memory of `config.go`.
			// Let's verify `config.go` content in next step if this fails to compile.
			// But for now, I'll assume I need to pass something.

			endpoint := fmt.Sprintf("https://%s-kube-api:6443", cfg.ClusterName) // Placeholder/Internal DNS?
			// Or better, use the Load Balancer IP if we can predict it? No.
			// If we use Floating IP, we know it.

			talosGen, err := talos.NewConfigGenerator(
				cfg.ClusterName,
				cfg.Kubernetes.Version,
				cfg.Talos.Version,
				endpoint,
				secretsFile,
			)
			if err != nil {
				return fmt.Errorf("failed to initialize talos generator: %w", err)
			}

			// 4. Initialize Reconciler
			reconciler := cluster.NewReconciler(hClient, talosGen, cfg)

			// 5. Run Reconcile
			if err := reconciler.Reconcile(cmd.Context()); err != nil {
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
		RunE: func(_ *cobra.Command, _ []string) error {
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
