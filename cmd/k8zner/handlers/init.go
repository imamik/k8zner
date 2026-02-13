package handlers

import (
	"context"
	"fmt"
	"os"

	"github.com/imamik/k8zner/internal/config"
)

// Factory function variables for init - can be replaced in tests.
var (
	// fileExists checks if a file exists.
	fileExists = func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	// runV2Wizard runs the simplified wizard.
	runV2Wizard = config.RunWizard

	// writeV2Config writes the config to a file.
	writeV2Config = config.WriteSpecYAML
)

// Init runs the v2 configuration wizard and writes the result to a file.
func Init(ctx context.Context, outputPath string) error {
	if fileExists(outputPath) {
		fmt.Printf("Warning: %s already exists and will be overwritten.\n\n", outputPath)
	}

	printWelcome()

	result, err := runV2Wizard(ctx)
	if err != nil {
		return fmt.Errorf("wizard canceled: %w", err)
	}

	cfg := result.ToSpec()

	if err := writeV2Config(cfg, outputPath); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	printInitSuccess(outputPath, cfg)

	return nil
}

// printWelcome prints the welcome message.
func printWelcome() {
	fmt.Println()
	fmt.Println("k8zner - Kubernetes on Hetzner Cloud")
	fmt.Println("====================================")
	fmt.Println()
	fmt.Println("This wizard creates a cluster configuration with sensible defaults.")
	fmt.Println("Just answer 6 simple questions.")
	fmt.Println("The generated YAML is fully expanded and explicit (advanced-mode style).")
	fmt.Println()
}

// printInitSuccess prints the success message with summary and next steps.
func printInitSuccess(outputPath string, cfg *config.Spec) {
	fmt.Println()
	fmt.Println("Configuration saved!")
	fmt.Println()
	fmt.Printf("  File: %s\n", outputPath)
	fmt.Println()

	// Summary
	fmt.Println("Cluster Summary")
	fmt.Println("---------------")
	fmt.Printf("  Name:           %s\n", cfg.Name)
	fmt.Printf("  Region:         %s\n", cfg.Region.String())
	fmt.Printf("  Mode:           %s\n", cfg.Mode)
	fmt.Printf("  Control Planes: %d x %s\n", cfg.ControlPlaneCount(), cfg.ControlPlaneSize())
	fmt.Printf("  Workers:        %d x %s\n", cfg.Workers.Count, cfg.Workers.Size)
	fmt.Printf("  Load Balancers: %d x %s\n", cfg.LoadBalancerCount(), config.LoadBalancerType)
	if cfg.Domain != "" {
		fmt.Printf("  Domain:         %s\n", cfg.Domain)
	}
	fmt.Println()

	// Features included
	fmt.Println("Included Features")
	fmt.Println("-----------------")
	fmt.Println("  - Talos Linux (immutable, secure OS)")
	fmt.Println("  - Cilium CNI (eBPF networking)")
	fmt.Println("  - Hetzner CCM (cloud integration)")
	fmt.Println("  - Hetzner CSI (persistent volumes)")
	fmt.Println("  - Traefik Ingress (with Let's Encrypt)")
	fmt.Println("  - ArgoCD (GitOps)")
	fmt.Println("  - cert-manager (TLS certificates)")
	fmt.Println("  - metrics-server (HPA support)")
	fmt.Println()

	// Next steps
	fmt.Println("Next Steps")
	fmt.Println("----------")
	fmt.Println("  1. Set your Hetzner Cloud API token:")
	fmt.Println("     export HCLOUD_TOKEN=<your-token>")
	fmt.Println()
	fmt.Printf("  2. Review %s if needed\n", outputPath)
	fmt.Println()
	fmt.Println("  3. Create your cluster:")
	fmt.Printf("     k8zner apply\n")
	fmt.Println()
}
