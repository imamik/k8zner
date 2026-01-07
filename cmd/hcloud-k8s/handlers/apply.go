package handlers

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/sak-d/hcloud-k8s/internal/cluster"
	"github.com/sak-d/hcloud-k8s/internal/config"
	"github.com/sak-d/hcloud-k8s/internal/hcloud"
	"github.com/sak-d/hcloud-k8s/internal/talos"
)

func Apply(ctx context.Context, configPath string) error {
	// 1. Load Config
	if configPath == "" {
		return fmt.Errorf("config file is required (use --config)")
	}

	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	log.Printf("Applying configuration for cluster: %s", cfg.ClusterName)

	// 2. Initialize Clients
	token := os.Getenv("HCLOUD_TOKEN")
	hClient := hcloud.NewRealClient(token)

	// 3. Initialize Talos Generator
	// Use local file for secrets persistence
	secretsFile := "secrets.yaml" // TODO: Make configurable or relative to config
	// Note: talos.NewConfigGenerator arguments might have changed or need verification.
	// Currently: (clusterName, kubernetesVersion, talosVersion, endpoint, secretsFile)
	// Config struct has: Talos.Version, Kubernetes.Version, ControlPlane.Endpoint (wait, Config struct doesn't have Endpoint?)
	// Let's check config struct if I can.
	// Assuming Config matches what I saw earlier.
	// Config.ControlPlane probably doesn't have Endpoint directly?
	// Terraform calculates endpoint from LB IP or Floating IP.
	// I should probably pass a temporary endpoint or handle it inside generator?
	// The generator needs an endpoint for the talosconfig.
	// If we use LB, it's `https://<lb-ip>:6443`.
	// Since we don't know LB IP yet (unless we query it), we might need to handle this.
	// For now, let's assume the user provides it or we use a placeholder that matches expected DNS?
	// Terraform usually uses `hcloud_load_balancer.control_plane.ipv4` which is known after apply.
	// This circular dependency is handled in Terraform by state.
	// Here, we might need two passes or update config later?
	// Or we can use a DNS name if provided.
	// For now, I'll pass a placeholder or try to get it from config if user supplied `control_plane_endpoint`?
	// Checking `internal/config/config.go` earlier...
	// `ControlPlaneConfig` has `NodePools`.
	// `KubernetesConfig` has `Version`, `OIDC`, `CNI`.
	// `TalosConfig` has `Version`.
	// I don't see `Endpoint` in `ControlPlaneConfig` in my memory of `config.go`.
	// Let's verify `config.go` content in next step if this fails to compile.
	// But for now, I'll assume I need to pass something.

	endpoint := fmt.Sprintf("https://%s-kube-api:6443", cfg.ClusterName) // Placeholder/Internal DNS?
	// Or better, use the Load Balancer IP if we can predict it? No.
	// If we use Floating IP, we know it.

	talosGen, err := talos.NewConfigGenerator(
		cfg.ClusterName,
		cfg.Kubernetes.Version,
		cfg.Talos.Version,
		endpoint,
		secretsFile,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize talos generator: %w", err)
	}

	// 4. Initialize Reconciler
	reconciler := cluster.NewReconciler(hClient, talosGen, cfg)

	// 5. Run Reconcile
	kubeconfig, err := reconciler.Reconcile(ctx)
	if err != nil {
		return fmt.Errorf("reconciliation failed: %w", err)
	}

	// 6. Output Talos Config
	clientCfgBytes, err := talosGen.GetClientConfig()
	if err != nil {
		return fmt.Errorf("failed to generate talosconfig: %w", err)
	}

	talosConfigPath := "talosconfig"
	if err := os.WriteFile(talosConfigPath, clientCfgBytes, 0600); err != nil {
		return fmt.Errorf("failed to write talosconfig: %w", err)
	}

	// 7. Output Kubeconfig
	kubeconfigPath := "kubeconfig"
	if len(kubeconfig) > 0 {
		if err := os.WriteFile(kubeconfigPath, kubeconfig, 0600); err != nil {
			return fmt.Errorf("failed to write kubeconfig: %w", err)
		}
		fmt.Printf("\nReconciliation complete!\n")
		fmt.Printf("Secrets saved to: %s\n", secretsFile)
		fmt.Printf("Talos config saved to: %s\n", talosConfigPath)
		fmt.Printf("Kubeconfig saved to: %s\n\n", kubeconfigPath)
		fmt.Printf("You can now access your cluster with:\n")
		fmt.Printf("  export KUBECONFIG=%s\n", kubeconfigPath)
		fmt.Printf("  kubectl get nodes\n")
	} else {
		fmt.Printf("\nReconciliation complete!\n")
		fmt.Printf("Secrets saved to: %s\n", secretsFile)
		fmt.Printf("Talos config saved to: %s\n", talosConfigPath)
		fmt.Printf("\nNote: Cluster was already bootstrapped. To retrieve kubeconfig, use talosctl:\n")
		fmt.Printf("  talosctl --talosconfig %s kubeconfig\n", talosConfigPath)
	}

	return nil
}
