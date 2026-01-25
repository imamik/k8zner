package handlers

import (
	"context"
	"fmt"

	"github.com/imamik/k8zner/internal/config/wizard"
)

// Init runs the interactive configuration wizard and writes the result to a file.
//
// This function orchestrates the configuration wizard workflow:
//  1. Checks if the output file already exists and prompts for confirmation
//  2. Runs the interactive wizard to collect configuration options
//  3. Builds a Config struct from the wizard results
//  4. Writes the configuration to the specified output file
//
// If advanced is true, additional configuration options are shown for network
// settings, security options, and Cilium features.
func Init(ctx context.Context, outputPath string, advanced bool) error {
	// Check if file exists and prompt for confirmation
	if wizard.FileExists(outputPath) {
		confirm, err := wizard.ConfirmOverwrite(outputPath)
		if err != nil {
			return fmt.Errorf("failed to prompt for confirmation: %w", err)
		}
		if !confirm {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Print welcome message
	printWelcome(advanced)

	// Run the interactive wizard
	result, err := wizard.RunWizard(ctx, advanced)
	if err != nil {
		return fmt.Errorf("wizard failed: %w", err)
	}

	// Build config from wizard result
	cfg := wizard.BuildConfig(result)

	// Write config to file
	if err := wizard.WriteConfig(cfg, outputPath); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Print success message
	printInitSuccess(outputPath, result)

	return nil
}

// printWelcome prints the welcome message.
func printWelcome(advanced bool) {
	fmt.Println()
	fmt.Println("k8zner - Kubernetes on Hetzner Cloud with Talos Linux")
	fmt.Println("======================================================")
	fmt.Println()
	if advanced {
		fmt.Println("Running in advanced mode - additional options will be shown.")
		fmt.Println()
	}
	fmt.Println("This wizard will help you create a cluster configuration file.")
	fmt.Println("Press Enter to accept defaults, or type your values.")
	fmt.Println()
}

// printInitSuccess prints the success message and next steps.
func printInitSuccess(outputPath string, result *wizard.WizardResult) {
	fmt.Println()
	fmt.Println("Configuration saved successfully!")
	fmt.Println()
	fmt.Printf("Output file: %s\n", outputPath)
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Printf("  Cluster name:    %s\n", result.ClusterName)
	fmt.Printf("  Location:        %s\n", result.Location)
	fmt.Printf("  Control plane:   %d x %s\n", result.ControlPlaneCount, result.ControlPlaneType)
	if result.AddWorkers {
		fmt.Printf("  Workers:         %d x %s\n", result.WorkerCount, result.WorkerType)
	} else {
		fmt.Println("  Workers:         None (workloads will run on control plane)")
	}
	fmt.Printf("  Talos version:   %s\n", result.TalosVersion)
	fmt.Printf("  K8s version:     %s\n", result.KubernetesVersion)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Set your Hetzner Cloud API token:")
	fmt.Println("     export HCLOUD_TOKEN=<your-token>")
	fmt.Println()
	fmt.Printf("  2. Review and customize %s if needed\n", outputPath)
	fmt.Println()
	fmt.Println("  3. Apply the configuration:")
	fmt.Printf("     k8zner apply -c %s\n", outputPath)
	fmt.Println()
}
