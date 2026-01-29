package v2

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// WizardResult holds the user's choices from the v2 wizard.
type WizardResult struct {
	Name        string
	Region      Region
	Mode        Mode
	WorkerCount int
	WorkerSize  ServerSize
	Domain      string
}

// RunWizard runs the simplified v2 configuration wizard.
func RunWizard(ctx context.Context) (*WizardResult, error) {
	result := &WizardResult{
		// Defaults
		Region:      RegionFalkenstein,
		Mode:        ModeDev,
		WorkerCount: 2,
		WorkerSize:  SizeCX32,
	}

	// Build the form
	form := huh.NewForm(
		// Cluster identity
		huh.NewGroup(
			huh.NewInput().
				Title("Cluster name").
				Description("A unique name for your cluster (DNS-safe, lowercase)").
				Placeholder("my-cluster").
				Value(&result.Name).
				Validate(validateClusterName),
		),

		// Region selection
		huh.NewGroup(
			huh.NewSelect[Region]().
				Title("Region").
				Description("Hetzner Cloud datacenter location").
				Options(
					huh.NewOption("Falkenstein, Germany (fsn1)", RegionFalkenstein),
					huh.NewOption("Nuremberg, Germany (nbg1)", RegionNuremberg),
					huh.NewOption("Helsinki, Finland (hel1)", RegionHelsinki),
				).
				Value(&result.Region),
		),

		// Mode selection
		huh.NewGroup(
			huh.NewSelect[Mode]().
				Title("Cluster mode").
				Description("dev: 1 CP node, lower cost | ha: 3 CP nodes, high availability").
				Options(
					huh.NewOption("Development (1 control plane)", ModeDev),
					huh.NewOption("High Availability (3 control planes)", ModeHA),
				).
				Value(&result.Mode),
		),

		// Worker configuration
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Number of workers").
				Description("Worker nodes run your application workloads").
				Options(
					huh.NewOption("1 worker", 1),
					huh.NewOption("2 workers", 2),
					huh.NewOption("3 workers", 3),
					huh.NewOption("4 workers", 4),
					huh.NewOption("5 workers", 5),
				).
				Value(&result.WorkerCount),

			huh.NewSelect[ServerSize]().
				Title("Worker size").
				Description("Shared vCPU instances (cost-effective)").
				Options(
					huh.NewOption("CX22 - 2 vCPU, 4GB RAM (~€4.35/mo)", SizeCX22),
					huh.NewOption("CX32 - 4 vCPU, 8GB RAM (~€8.15/mo)", SizeCX32),
					huh.NewOption("CX42 - 8 vCPU, 16GB RAM (~€16.25/mo)", SizeCX42),
					huh.NewOption("CX52 - 16 vCPU, 32GB RAM (~€32.45/mo)", SizeCX52),
				).
				Value(&result.WorkerSize),
		),

		// Optional domain
		huh.NewGroup(
			huh.NewInput().
				Title("Domain (optional)").
				Description("Your domain for DNS + TLS. Leave empty to skip.").
				Placeholder("example.com").
				Value(&result.Domain).
				Validate(validateDomain),
		),
	)

	// Run the form
	if err := form.RunWithContext(ctx); err != nil {
		return nil, fmt.Errorf("wizard canceled: %w", err)
	}

	return result, nil
}

// ToConfig converts the wizard result to a v2 Config.
func (r *WizardResult) ToConfig() *Config {
	return &Config{
		Name:   r.Name,
		Region: r.Region,
		Mode:   r.Mode,
		Workers: Worker{
			Count: r.WorkerCount,
			Size:  r.WorkerSize,
		},
		Domain: r.Domain,
	}
}

// validateClusterName validates the cluster name.
func validateClusterName(s string) error {
	if s == "" {
		return fmt.Errorf("cluster name is required")
	}
	s = strings.ToLower(s)
	if len(s) > 63 {
		return fmt.Errorf("cluster name must be 63 characters or less")
	}
	// Basic DNS-safe validation
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return fmt.Errorf("cluster name can only contain lowercase letters, numbers, and hyphens")
		}
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return fmt.Errorf("cluster name cannot start or end with a hyphen")
	}
	return nil
}

// validateDomain validates the optional domain.
func validateDomain(s string) error {
	if s == "" {
		return nil // Optional
	}
	// Basic domain validation
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid domain format (expected example.com)")
	}
	return nil
}

// WriteYAML writes the v2 config to a YAML file.
func WriteYAML(cfg *Config, path string) error {
	return Save(cfg, path)
}
