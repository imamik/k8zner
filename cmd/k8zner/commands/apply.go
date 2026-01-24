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
// Required flags:
//
//	--config, -c: Path to cluster configuration YAML file
//
// Environment variables:
//
//	HCLOUD_TOKEN: Hetzner Cloud API token (required)
func Apply() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply configuration to the cluster",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Apply(cmd.Context(), configPath)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file")
	// MarkFlagRequired cannot fail for flags defined on the same command
	_ = cmd.MarkFlagRequired("config")

	return cmd
}
