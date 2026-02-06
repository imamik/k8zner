package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Migrate returns the command for converting legacy CLI clusters to operator-managed.
//
// This command detects existing resources from a CLI-provisioned cluster and
// creates the necessary K8znerCluster CRD and credentials Secret to enable
// operator management.
//
// Optional flags:
//
//	--config, -c: Path to cluster configuration YAML file (default: auto-detect k8zner.yaml)
//	--dry-run: Show what would be migrated without making changes
//
// Environment variables:
//
//	HCLOUD_TOKEN: Hetzner Cloud API token (required)
func Migrate() *cobra.Command {
	var configPath string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Convert legacy CLI cluster to operator-managed",
		Long: `Convert a CLI-provisioned cluster to operator management.

This command:
  1. Detects existing resources by cluster label
  2. Reads local secrets.yaml and talosconfig files
  3. Creates a credentials Secret in the cluster
  4. Creates a K8znerCluster CRD with detected state
  5. Installs the operator (if not present)

After migration, the operator takes over health monitoring and self-healing.
Use 'k8zner apply' to make configuration changes.

Prerequisites:
  - Cluster must be running and accessible
  - secrets.yaml and talosconfig files must exist locally
  - HCLOUD_TOKEN environment variable must be set

Examples:
  # Migrate cluster to operator management
  k8zner migrate

  # Migrate using specific config file
  k8zner migrate -c production.yaml

  # Preview what would be migrated
  k8zner migrate --dry-run`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return handlers.Migrate(cmd.Context(), configPath, dryRun)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file (default: k8zner.yaml)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be migrated without making changes")

	return cmd
}
