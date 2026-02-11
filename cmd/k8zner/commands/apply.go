package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Apply returns the command for creating or updating Kubernetes clusters.
//
// This is the single entry point for cluster provisioning:
//   - New cluster: bootstraps infrastructure, deploys operator, creates CRD
//   - Existing cluster: updates CRD spec, operator reconciles changes
//
// Optional flags:
//
//	--config, -c: Path to cluster configuration YAML file (default: auto-detect k8zner.yaml)
//	--wait: Wait for operator to complete provisioning
//
// Environment variables:
//
//	HCLOUD_TOKEN: Hetzner Cloud API token (required)
func Apply() *cobra.Command {
	var configPath string
	var wait bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Create or update the cluster",
		Long: `Create or update your Kubernetes cluster.

For new clusters, this command:
  1. Creates infrastructure (network, firewall, LB, placement group)
  2. Bootstraps first control plane with Talos Linux
  3. Deploys the k8zner operator
  4. Creates K8znerCluster CRD (operator provisions remaining nodes & addons)

For existing clusters, this command updates the CRD spec and the
operator reconciles the changes.

If no config file is specified, it looks for k8zner.yaml in the current
directory. Use 'k8zner init' to create a configuration file.

Examples:
  # Create cluster using k8zner.yaml in current directory
  k8zner apply

  # Create cluster and wait for full provisioning
  k8zner apply --wait

  # Update cluster using specific config file
  k8zner apply -c production.yaml`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Apply(cmd.Context(), configPath, wait)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file (default: k8zner.yaml)")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for operator to complete provisioning")

	return cmd
}
