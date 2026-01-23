package commands

import (
	"github.com/spf13/cobra"

	"k8zner/cmd/k8zner/handlers"
)

// Image returns the parent command for managing Talos Linux images.
//
// This command provides subcommands for building and managing custom
// Talos Linux snapshots that can be used for cluster node provisioning.
func Image() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Manage Talos images",
	}

	cmd.AddCommand(Build())

	return cmd
}

// Build returns the command for building custom Talos Linux snapshots.
//
// This command creates a Hetzner Cloud snapshot with a specific Talos Linux
// version and architecture. The snapshot can then be referenced in cluster
// configuration files for node provisioning.
//
// Flags:
//
//	--name: Snapshot name (default: "talos")
//	--version: Talos version to install (default: "v1.7.0")
//	--arch: CPU architecture - amd64 or arm64 (default: "amd64")
//	--location: Hetzner datacenter location (default: "nbg1")
//
// Environment variables:
//
//	HCLOUD_TOKEN: Hetzner Cloud API token (required)
func Build() *cobra.Command {
	var (
		imageName    string
		talosVersion string
		arch         string
		location     string
	)

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a new Talos image",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Build(cmd.Context(), imageName, talosVersion, arch, location)
		},
	}

	cmd.Flags().StringVar(&imageName, "name", "talos", "Name of the image to create")
	cmd.Flags().StringVar(&talosVersion, "version", "v1.7.0", "Talos version to install")
	cmd.Flags().StringVar(&location, "location", "nbg1", "Hetzner datacenter location (e.g., nbg1, fsn1, hel1)")
	cmd.Flags().StringVar(&arch, "arch", "amd64", "Architecture (amd64 or arm64)")

	return cmd
}
