package commands

import (
	"github.com/spf13/cobra"

	"github.com/imamik/k8zner/cmd/k8zner/handlers"
)

// Bootstrap returns the command for minimal cluster bootstrap with operator handoff.
//
// This command creates a single control plane node and hands off to the k8zner
// operator for full cluster provisioning. This is the recommended way to create
// new clusters as it enables operator-driven infrastructure management.
//
// The bootstrap process:
//  1. Creates a single control plane server (public IP only)
//  2. Generates Talos secrets and applies configuration
//  3. Bootstraps etcd and waits for API server
//  4. Installs k8zner operator and CRDs
//  5. Creates K8znerCluster CRD for operator to take over
//
// The operator then handles:
//   - Network, firewall, and load balancer creation
//   - Remaining control plane nodes (for HA)
//   - Worker nodes provisioning
//   - Addon installation (Cilium, CCM, CSI)
//   - Health monitoring and self-healing
//
// Optional flags:
//
//	--config, -c: Path to cluster configuration YAML file (default: auto-detect k8zner.yaml)
//	--operator-image: Custom operator image (default: ghcr.io/imamik/k8zner-operator:main)
//	--skip-operator: Skip operator installation (for manual installation)
//	--dry-run: Print what would be done without making changes
//
// Environment variables:
//
//	HCLOUD_TOKEN: Hetzner Cloud API token (required)
func Bootstrap() *cobra.Command {
	var (
		configPath    string
		operatorImage string
		skipOperator  bool
		dryRun        bool
	)

	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap a minimal cluster for operator management",
		Long: `Bootstrap a minimal Kubernetes cluster and hand off to the k8zner operator.

This command creates a single control plane node with a public IP only (no network,
firewall, or load balancer). After the cluster is bootstrapped, the k8zner operator
takes over and provisions the remaining infrastructure and nodes.

This approach provides:
  - Minimal initial footprint (single server)
  - Operator-driven infrastructure management
  - Self-healing and automated scaling
  - Consistent state management via Kubernetes CRD

The operator will automatically:
  - Create private network, firewall, and load balancer
  - Attach the bootstrap node to infrastructure
  - Provision additional control planes (if HA mode)
  - Provision worker nodes
  - Install cluster addons

Examples:
  # Bootstrap using k8zner.yaml in current directory
  k8zner bootstrap

  # Bootstrap using specific config file
  k8zner bootstrap -c production.yaml

  # Bootstrap without installing operator (manual installation)
  k8zner bootstrap --skip-operator

  # See what would be done without making changes
  k8zner bootstrap --dry-run

After bootstrap completes, monitor operator progress with:
  kubectl logs -f -n k8zner-system deploy/k8zner-operator
  kubectl get k8znerclusters -n k8zner-system -w`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := handlers.BootstrapOptions{
				ConfigPath:    configPath,
				OperatorImage: operatorImage,
				SkipOperator:  skipOperator,
				DryRun:        dryRun,
			}
			return handlers.Bootstrap(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to configuration file (default: k8zner.yaml)")
	cmd.Flags().StringVar(&operatorImage, "operator-image", "ghcr.io/imamik/k8zner-operator:main", "Operator container image")
	cmd.Flags().BoolVar(&skipOperator, "skip-operator", false, "Skip operator installation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would be done without making changes")

	return cmd
}
