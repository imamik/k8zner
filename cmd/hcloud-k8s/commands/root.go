// Package commands defines the CLI command structure and flag bindings.
//
// This package contains cobra command definitions that handle argument parsing,
// flag binding, and validation. Command execution is delegated to handler
// functions in the handlers package.
package commands

import "github.com/spf13/cobra"

// Root returns the root command for the hcloud-k8s CLI.
//
// The root command serves as the entry point and parent for all subcommands.
// It provides basic CLI metadata and organizes the command hierarchy.
func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hcloud-k8s",
		Short: "Provision Kubernetes on Hetzner Cloud using Talos",
	}

	cmd.AddCommand(Apply())
	cmd.AddCommand(Image())
	cmd.AddCommand(Destroy())

	return cmd
}
