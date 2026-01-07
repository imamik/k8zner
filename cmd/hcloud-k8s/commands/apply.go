package commands

import (
	"github.com/spf13/cobra"

	"github.com/sak-d/hcloud-k8s/cmd/hcloud-k8s/handlers"
)

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

	return cmd
}
