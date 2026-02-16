package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Init returns the command for interactively creating a cluster configuration.
//
// This command guides users through creating a cluster configuration YAML file
// using a simplified wizard with just 6 questions.
//
// Flags:
//
//	--output, -o: Path to output file (default "k8zner.yaml")
func Init() *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a cluster configuration interactively",
		Long: `Create a cluster configuration file with a simple wizard.

This command asks just 6 questions:

  1. Cluster name     (DNS-safe identifier)
  2. Region           (fsn1, nbg1, or hel1)
  3. Mode             (dev or ha)
  4. Worker count     (1-5 workers)
  5. Worker size      (smallest to largest dedicated vCPU)
  6. Domain           (optional, for DNS + TLS)

The output is a full, explicit configuration (advanced-mode style):

  - Talos Linux for immutable, secure nodes
  - IPv6-only nodes (saves cost, improves security)
  - Cilium CNI with eBPF networking
  - Full addon stack (ArgoCD, Traefik, cert-manager, etc.)

Examples:
  # Create config in current directory
  k8zner init

  # Create config with custom path
  k8zner init -o production.yaml`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Init(cmd.Context(), outputPath)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "k8zner.yaml", "Output file path")

	return cmd
}
