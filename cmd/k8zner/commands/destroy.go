package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Destroy returns the destroy command.
//
// The destroy command removes all cluster resources from Hetzner Cloud.
// It deletes resources in dependency order: servers, load balancers,
// floating IPs, firewalls, networks, placement groups, and certificates.
func Destroy() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy a Kubernetes cluster and all associated resources",
		Long: `Destroy removes all cluster resources from Hetzner Cloud.

This command deletes all resources associated with the cluster including:
  - Servers (control plane and worker nodes)
  - Load balancers
  - Floating IPs
  - Firewalls
  - Networks and subnets
  - Placement groups
  - SSH keys
  - Certificates (Talos state markers)

Resources are deleted in dependency order to ensure clean teardown.

Example:
  k8zner destroy -c k8zner.yaml

WARNING: This operation is irreversible. All cluster data will be lost.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Destroy(cmd.Context(), configPath)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to cluster configuration file (required)")
	_ = cmd.MarkFlagRequired("config")

	return cmd
}
