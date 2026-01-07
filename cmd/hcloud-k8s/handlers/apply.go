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

const (
	secretsFile      = "secrets.yaml"
	talosConfigPath  = "talosconfig"
	kubeconfigPath   = "kubeconfig"
)

func Apply(ctx context.Context, configPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	log.Printf("Applying configuration for cluster: %s", cfg.ClusterName)

	client := initializeClient()
	talosGen, err := initializeTalosGenerator(cfg)
	if err != nil {
		return err
	}

	if err := writeTalosFiles(talosGen); err != nil {
		return err
	}

	kubeconfig, err := reconcileCluster(ctx, client, talosGen, cfg)
	if err != nil {
		return err
	}

	if err := writeKubeconfig(kubeconfig); err != nil {
		return err
	}

	printSuccess(kubeconfig)
	return nil
}

func loadConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		return nil, fmt.Errorf("config file is required (use --config)")
	}

	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return cfg, nil
}

func initializeClient() *hcloud.RealClient {
	token := os.Getenv("HCLOUD_TOKEN")
	return hcloud.NewRealClient(token)
}

func initializeTalosGenerator(cfg *config.Config) (*talos.ConfigGenerator, error) {
	endpoint := fmt.Sprintf("https://%s-kube-api:6443", cfg.ClusterName)

	gen, err := talos.NewConfigGenerator(
		cfg.ClusterName,
		cfg.Kubernetes.Version,
		cfg.Talos.Version,
		endpoint,
		secretsFile,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize talos generator: %w", err)
	}

	return gen, nil
}

func reconcileCluster(ctx context.Context, client *hcloud.RealClient, talosGen *talos.ConfigGenerator, cfg *config.Config) ([]byte, error) {
	reconciler := cluster.NewReconciler(client, talosGen, cfg)
	kubeconfig, err := reconciler.Reconcile(ctx)
	if err != nil {
		return nil, fmt.Errorf("reconciliation failed: %w", err)
	}

	return kubeconfig, nil
}

func writeTalosFiles(talosGen *talos.ConfigGenerator) error {
	clientCfgBytes, err := talosGen.GetClientConfig()
	if err != nil {
		return fmt.Errorf("failed to generate talosconfig: %w", err)
	}

	if err := os.WriteFile(talosConfigPath, clientCfgBytes, 0600); err != nil {
		return fmt.Errorf("failed to write talosconfig: %w", err)
	}

	return nil
}

func writeKubeconfig(kubeconfig []byte) error {
	if len(kubeconfig) == 0 {
		return nil
	}

	if err := os.WriteFile(kubeconfigPath, kubeconfig, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	return nil
}

func printSuccess(kubeconfig []byte) {
	fmt.Printf("\nReconciliation complete!\n")
	fmt.Printf("Secrets saved to: %s\n", secretsFile)
	fmt.Printf("Talos config saved to: %s\n", talosConfigPath)

	if len(kubeconfig) > 0 {
		fmt.Printf("Kubeconfig saved to: %s\n\n", kubeconfigPath)
		fmt.Printf("You can now access your cluster with:\n")
		fmt.Printf("  export KUBECONFIG=%s\n", kubeconfigPath)
		fmt.Printf("  kubectl get nodes\n")
	} else {
		fmt.Printf("\nNote: Cluster was already bootstrapped. To retrieve kubeconfig, use talosctl:\n")
		fmt.Printf("  talosctl --talosconfig %s kubeconfig\n", talosConfigPath)
	}
}
