package commands

import (
	"github.com/spf13/cobra"

	"github.com/sak-d/hcloud-k8s/cmd/hcloud-k8s/handlers"
)

func Image() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Manage Talos images",
	}

	cmd.AddCommand(build())

	return cmd
}

func build() *cobra.Command {
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
