package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Health returns the command for displaying cluster health status.
//
// This command shows the current status of all cluster components including
// infrastructure, control planes, workers, and addons.
//
// Optional flags:
//
//	--config, -c: Path to cluster configuration YAML file (default: auto-detect k8zner.yaml)
//	--watch, -w: Continuously watch status updates
//	--json: Output in JSON format
func Health() *cobra.Command {
	var configPath string
	var watch bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Show cluster health status",
		Long: `Display the current health status of your k8zner cluster.

Shows the status of:
  - Infrastructure (Network, Firewall, Load Balancer)
  - Control Planes (count, readiness)
  - Workers (count, readiness)
  - Addons (Cilium, CCM, CSI, Traefik, etc.)

This command works with both operator-managed and CLI-managed clusters.

Examples:
  # Show cluster health
  k8zner health

  # Watch cluster health continuously
  k8zner health --watch

  # Get health status in JSON format
  k8zner health --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Health(cmd.Context(), configPath, watch, jsonOutput)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file (default: k8zner.yaml)")
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "Continuously watch status updates")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}
