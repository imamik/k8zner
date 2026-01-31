package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	v2 "github.com/imamik/k8zner/internal/config/v2"
	"github.com/imamik/k8zner/internal/pricing"
)

// costOptions holds the command line options for the cost command.
type costOptions struct {
	configPath string
	jsonOutput bool
	compact    bool
}

// Cost returns the cost command.
func Cost() *cobra.Command {
	opts := &costOptions{}

	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Estimate cluster costs",
		Long: `Calculate the estimated monthly cost for a cluster configuration.

Fetches live pricing from the Hetzner API (requires HCLOUD_TOKEN) and
calculates costs including VAT for German customers.

Examples:
  # Estimate costs for current config
  k8zner cost

  # Estimate costs for specific config file
  k8zner cost -f production.yaml

  # Output as JSON
  k8zner cost --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCost(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.configPath, "file", "f", "", "config file path (default: k8zner.yaml)")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&opts.compact, "compact", false, "output compact single-line format")

	return cmd
}

func runCost(ctx context.Context, opts *costOptions) error {
	// Load config
	configPath := opts.configPath
	if configPath == "" {
		var err error
		configPath, err = v2.FindConfigFile()
		if err != nil {
			return fmt.Errorf("no config file found: %w\nRun 'k8zner init' to create one", err)
		}
	}

	cfg, err := v2.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Fetch pricing (use live API if token available, otherwise defaults)
	token := os.Getenv("HCLOUD_TOKEN")
	prices := pricing.FetchOrDefault(ctx, token)

	// Calculate costs
	calc := pricing.NewCalculatorWithPrices(prices)
	estimate := calc.Calculate(cfg)

	// Format and output
	formatter := pricing.NewFormatter()

	var output string
	switch {
	case opts.jsonOutput:
		output = formatter.FormatJSON(estimate)
	case opts.compact:
		output = formatter.FormatCompact(estimate)
	default:
		output = formatter.Format(estimate)
	}

	fmt.Print(output)
	return nil
}
