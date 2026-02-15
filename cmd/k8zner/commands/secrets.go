package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Secrets returns the command for retrieving cluster secrets.
func Secrets() *cobra.Command {
	var configPath string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Retrieve cluster secrets and credentials",
		Long: `Retrieve secrets and credentials from the running cluster.

Shows passwords and access details for installed services:
  - ArgoCD admin password
  - Grafana admin password
  - Kubeconfig and Talosconfig file paths

Requires a running cluster with a valid kubeconfig file.
`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Secrets(cmd.Context(), configPath, jsonOutput)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file (default: k8zner.yaml)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}
