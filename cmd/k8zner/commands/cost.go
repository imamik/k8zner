package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Cost returns the command for cluster cost analysis.
func Cost() *cobra.Command {
	var configPath string
	var jsonOutput bool
	var s3StorageGB float64

	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Show current and planned cluster cost",
		Long: `Analyze cluster costs using live Hetzner Cloud pricing.

This command compares:
  - Current monthly cost from real resources discovered by cluster labels
  - Planned monthly cost from your YAML configuration
  - Delta between current and planned totals

Detailed output includes net and gross monthly totals per component.
`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Cost(cmd.Context(), configPath, jsonOutput, s3StorageGB)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file (default: k8zner.yaml)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().Float64Var(&s3StorageGB, "s3-storage-gb", 100, "Estimated object storage usage in GB for backup cost")

	return cmd
}
