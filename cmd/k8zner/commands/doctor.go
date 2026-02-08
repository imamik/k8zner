package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Doctor returns the command for diagnosing cluster status.
//
// This command validates configuration and shows live cluster status
// with ASCII table output and emoji indicators.
//
// Optional flags:
//
//	--config, -c: Path to cluster configuration YAML file (default: auto-detect k8zner.yaml)
//	--watch, -w: Continuously watch status updates
//	--json: Output in JSON format
func Doctor() *cobra.Command {
	var configPath string
	var watch bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose cluster configuration and status",
		Long: `Diagnose your k8zner cluster configuration and status.

Pre-cluster mode (no kubeconfig):
  - Validates configuration file
  - Checks Hetzner API connectivity
  - Shows what would be created

Cluster mode (kubeconfig exists):
  - Shows infrastructure, control planes, workers, and addons
  - Displays cluster phase and provisioning progress
  - Shows detailed node and addon health

Examples:
  # Diagnose cluster
  k8zner doctor

  # Watch cluster status continuously
  k8zner doctor --watch

  # Get status in JSON format
  k8zner doctor --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Doctor(cmd.Context(), configPath, watch, jsonOutput)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file (default: k8zner.yaml)")
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "Continuously watch status updates")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}
