package commands

import (
	"github.com/spf13/cobra"

	"k8zner/cmd/k8zner/handlers"
)

// Upgrade returns the command for upgrading Talos OS and Kubernetes.
//
// This command handles upgrading an existing cluster to new versions:
// - Talos OS version upgrade (sequential for control plane, parallel for workers)
// - Kubernetes version upgrade (after Talos upgrade)
// - Health checks between upgrades to ensure cluster stability
//
// Required flags:
//
//	--config, -c: Path to cluster configuration YAML file
//
// Optional flags:
//
//	--dry-run: Show what would be upgraded without executing
//	--skip-health-check: Skip health checks (dangerous, use with caution)
//	--k8s-version: Override Kubernetes version from config
//
// Environment variables:
//
//	HCLOUD_TOKEN: Hetzner Cloud API token (required)
func Upgrade() *cobra.Command {
	var configPath string
	var dryRun bool
	var skipHealthCheck bool
	var k8sVersion string

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade Talos OS and Kubernetes versions",
		Long: `Upgrade an existing cluster to new Talos OS and/or Kubernetes versions.

The upgrade process:
1. Validates configuration and checks compatibility
2. Upgrades control plane nodes sequentially (maintains quorum)
3. Upgrades worker nodes in parallel (faster)
4. Upgrades Kubernetes version (if changed)
5. Performs health checks between each step

Use --dry-run to see what would be upgraded without making changes.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := handlers.UpgradeOptions{
				ConfigPath:      configPath,
				DryRun:          dryRun,
				SkipHealthCheck: skipHealthCheck,
				K8sVersion:      k8sVersion,
			}
			return handlers.Upgrade(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be upgraded without executing")
	cmd.Flags().BoolVar(&skipHealthCheck, "skip-health-check", false, "Skip health checks between upgrades (dangerous)")
	cmd.Flags().StringVar(&k8sVersion, "k8s-version", "", "Override Kubernetes version from config")

	// MarkFlagRequired cannot fail for flags defined on the same command
	_ = cmd.MarkFlagRequired("config")

	return cmd
}
