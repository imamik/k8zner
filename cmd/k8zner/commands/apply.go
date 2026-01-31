package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Apply returns the command for provisioning and managing Kubernetes clusters.
//
// This command handles the complete lifecycle of cluster provisioning:
// loading configuration, initializing infrastructure, generating secrets,
// and bootstrapping Kubernetes using Talos Linux.
//
// Optional flags:
//
//	--config, -c: Path to cluster configuration YAML file (default: auto-detect k8zner.yaml)
//
// Environment variables:
//
//	HCLOUD_TOKEN: Hetzner Cloud API token (required)
func Apply() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Create or update the cluster",
		Long: `Create or update your Kubernetes cluster.

This command provisions all infrastructure on Hetzner Cloud and bootstraps
Kubernetes using Talos Linux.

If no config file is specified, it looks for k8zner.yaml in the current
directory. Use 'k8zner init' to create a configuration file.

Examples:
  # Create cluster using k8zner.yaml in current directory
  k8zner apply

  # Create cluster using specific config file
  k8zner apply -c production.yaml

  # Re-apply after configuration changes
  k8zner apply`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Apply(cmd.Context(), configPath)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file (default: k8zner.yaml)")

	return cmd
}
