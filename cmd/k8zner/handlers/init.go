package handlers

import (
	"context"
	"fmt"
	"os"

	v2config "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/pricing"
)

// Factory function variables for init - can be replaced in tests.
var (
	// fileExists checks if a file exists.
	fileExists = func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	// runV2Wizard runs the simplified wizard.
	runV2Wizard = v2config.RunWizard

	// writeV2Config writes the config to a file.
	writeV2Config = v2config.WriteYAML
)

// Init runs the simplified v2 configuration wizard and writes the result to a file.
//
// This function orchestrates the v2 configuration workflow:
//  1. Checks if the output file already exists and warns the user
//  2. Runs the simplified wizard to collect 5 key options
//  3. Converts the wizard result to a v2 Config
//  4. Writes the configuration to the specified output file
//  5. Shows cost estimate and next steps
func Init(ctx context.Context, outputPath string) error {
	// Check if file exists
	if fileExists(outputPath) {
		fmt.Printf("Warning: %s already exists and will be overwritten.\n\n", outputPath)
	}

	// Print welcome message
	printWelcome()

	// Run the simplified v2 wizard
	result, err := runV2Wizard(ctx)
	if err != nil {
		return fmt.Errorf("wizard canceled: %w", err)
	}

	// Convert to config
	cfg := result.ToConfig()

	// Write config to file
	if err := writeV2Config(cfg, outputPath); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Print success message with cost estimate
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
	fmt.Println("Just answer 5 simple questions.")
	fmt.Println()
}

// printInitSuccess prints the success message with cost estimate and next steps.
func printInitSuccess(outputPath string, cfg *v2config.Config) {
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
	fmt.Printf("  Load Balancers: %d x %s\n", cfg.LoadBalancerCount(), v2config.LoadBalancerType)
	if cfg.Domain != "" {
		fmt.Printf("  Domain:         %s\n", cfg.Domain)
	}
	fmt.Println()

	// Cost estimate
	fmt.Println("Cost Estimate (with 19% VAT)")
	fmt.Println("----------------------------")

	// Use default prices for quick estimate (live prices need API token)
	calc := pricing.NewCalculator()
	estimate := calc.Calculate(cfg)

	for _, item := range estimate.Items {
		fmt.Printf("  %s: %d x %s @ %.2f = %.2f/mo\n",
			item.Description, item.Quantity, item.UnitType, item.UnitPrice, item.Total)
	}
	fmt.Println("  ---")
	fmt.Printf("  Subtotal:       %.2f/mo\n", estimate.Subtotal)
	fmt.Printf("  VAT (19%%):      %.2f/mo\n", estimate.VAT)
	fmt.Printf("  Total:          %.2f/mo\n", estimate.Total)
	fmt.Printf("  Annual:         ~%.0f/yr\n", estimate.AnnualCost())
	fmt.Println()
	fmt.Printf("  IPv6 Savings:   %.2f/mo (no IPv4 on nodes)\n", estimate.IPv6Savings)
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
