package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Init returns the command for interactively creating a cluster configuration.
//
// This command guides users through creating a cluster configuration YAML file
// using an interactive wizard with text inputs, single-select, and multi-select
// prompts.
//
// Flags:
//
//	--output, -o: Path to output file (default "cluster.yaml")
//	--advanced, -a: Show advanced configuration options
func Init() *cobra.Command {
	var (
		outputPath string
		advanced   bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Interactively create a cluster configuration",
		Long: `Interactively create a cluster configuration file.

This command guides you through configuring your Kubernetes cluster
step by step. It will ask about:

  - Cluster identity (name and location)
  - SSH access keys
  - Control plane configuration
  - Worker node configuration
  - Cluster addons
  - Talos and Kubernetes versions

Use --advanced for additional options like network CIDRs,
disk encryption, and Cilium features.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Init(cmd.Context(), outputPath, advanced)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "cluster.yaml", "Output file path")
	cmd.Flags().BoolVarP(&advanced, "advanced", "a", false, "Show advanced configuration options")

	return cmd
}
