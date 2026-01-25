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
//	--full, -f: Output full YAML with all options (default: minimal output)
func Init() *cobra.Command {
	var (
		outputPath string
		advanced   bool
		fullOutput bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Interactively create a cluster configuration",
		Long: `Interactively create a cluster configuration file.

This command guides you through configuring your Kubernetes cluster
step by step. It will ask about:

  - Cluster identity (name and location)
  - SSH access keys (optional)
  - Server architecture (x86 or ARM)
  - Server category (shared, dedicated, or cost-optimized)
  - Control plane configuration
  - Worker node configuration
  - CNI selection (Cilium, Talos default, or none)
  - Cluster addons
  - Talos and Kubernetes versions

Use --advanced for additional options like network CIDRs,
disk encryption, and Cilium features.

Use --full to output the complete YAML with all configuration
options (useful for manual editing). By default, a minimal
YAML is generated with only essential values.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Init(cmd.Context(), outputPath, advanced, fullOutput)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "cluster.yaml", "Output file path")
	cmd.Flags().BoolVarP(&advanced, "advanced", "a", false, "Show advanced configuration options")
	cmd.Flags().BoolVarP(&fullOutput, "full", "f", false, "Output full YAML with all options")

	return cmd
}
