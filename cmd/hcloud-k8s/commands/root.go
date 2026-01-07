package commands

import "github.com/spf13/cobra"

func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hcloud-k8s",
		Short: "Provision Kubernetes on Hetzner Cloud using Talos",
	}

	cmd.AddCommand(Apply())
	cmd.AddCommand(Image())

	return cmd
}
