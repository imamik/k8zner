// Package main provides the entry point for the hcloud-k8s CLI.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/image"
)

var (
	token        string
	talosVersion string
	arch         string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "hcloud-k8s",
		Short: "hcloud-k8s - Kubernetes on Hetzner Cloud with Talos",
		Long: `hcloud-k8s is a CLI tool for provisioning and managing Kubernetes clusters
on Hetzner Cloud using Talos Linux.`,
	}

	rootCmd.PersistentFlags().StringVar(&token, "token", "", "Hetzner Cloud API Token")

	imageCmd := &cobra.Command{
		Use:   "image",
		Short: "Manage disk images",
	}

	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Build a Talos disk image",
		RunE: func(_ *cobra.Command, args []string) error {
			if token == "" {
				return fmt.Errorf("token is required")
			}
			if len(args) < 1 {
				return fmt.Errorf("image name is required")
			}
			imageName := args[0]

			client := hcloud.NewRealClient(token)

			// We pass nil for factory so Builder uses default SSHCommunicator with generated keys.
			builder := image.NewBuilder(client, nil)
			// Pass nil for labels for now as CLI doesn't support them yet.
			snapshotID, err := builder.Build(context.Background(), imageName, talosVersion, arch, nil)
			if err != nil {
				return err
			}

			fmt.Printf("Successfully built image. Snapshot ID: %s\n", snapshotID)
			return nil
		},
	}

	buildCmd.Flags().StringVar(&talosVersion, "talos-version", "v1.8.0", "Talos version to install")
	buildCmd.Flags().StringVar(&arch, "arch", "amd64", "Architecture (amd64/arm64)")

	imageCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(imageCmd)

	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}
