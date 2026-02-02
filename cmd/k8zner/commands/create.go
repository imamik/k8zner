package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Create returns the command for bootstrapping a new cluster with operator management.
//
// This command creates the minimal bootstrap infrastructure, deploys the operator,
// and creates a K8znerCluster CRD. The operator then takes over management of
// CNI, addons, additional control planes, and workers.
//
// Optional flags:
//
//	--config, -c: Path to cluster configuration YAML file (default: auto-detect k8zner.yaml)
//	--wait: Wait for operator to complete provisioning
//
// Environment variables:
//
//	HCLOUD_TOKEN: Hetzner Cloud API token (required)
func Create() *cobra.Command {
	var configPath string
	var wait bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Bootstrap a new operator-managed cluster",
		Long: `Bootstrap a new Kubernetes cluster with operator management.

This command creates the minimal infrastructure needed to bootstrap:
  1. Talos image snapshot
  2. Load balancer (stable API endpoint)
  3. Network and subnets
  4. Placement group
  5. Firewall
  6. First control plane node

Once the first control plane is ready, the operator is deployed and takes
over management of CNI (Cilium), addons, additional control planes, and workers.

The config.yaml file serves as the source of truth. Use 'k8zner apply' to
update the configuration after the cluster is created.

Examples:
  # Create cluster using k8zner.yaml in current directory
  k8zner create

  # Create cluster using specific config file
  k8zner create -c production.yaml

  # Create and wait for full provisioning
  k8zner create --wait`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Create(cmd.Context(), configPath, wait)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file (default: k8zner.yaml)")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for operator to complete provisioning")

	return cmd
}
