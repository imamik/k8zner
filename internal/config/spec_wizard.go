package config

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// serverSizeOptions are the options shown in the wizard worker size selector.
// Populated by FetchServerSizeOptions from the Hetzner API.
// Falls back to a static list only when no HCLOUD_TOKEN is available.
var serverSizeOptions []huh.Option[ServerSize]

// FetchServerSizeOptions queries the Hetzner API for available server types
// and populates the wizard options. Filters to x86 architecture, shared vCPU,
// non-deprecated types. Returns an error only if the API call itself fails;
// an empty result set silently falls back to defaults.
func FetchServerSizeOptions(ctx context.Context, client *hcloud.Client) error {
	types, err := client.ServerType.All(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch server types: %w", err)
	}

	var opts []huh.Option[ServerSize]
	for _, st := range types {
		// x86 architecture only (excludes ARM like cax*)
		if st.Architecture != hcloud.ArchitectureX86 {
			continue
		}
		// Shared vCPU only (excludes dedicated like ccx*)
		if st.CPUType != hcloud.CPUTypeShared {
			continue
		}
		// Skip deprecated types
		if st.IsDeprecated() {
			continue
		}

		size := ServerSize(st.Name)

		// Find monthly net price (use first available location)
		priceStr := ""
		for _, p := range st.Pricings {
			if p.Monthly.Net != "" {
				priceStr = p.Monthly.Net
				break
			}
		}

		label := fmt.Sprintf("%s - %d vCPU, %.0fGB RAM",
			strings.ToUpper(st.Name), st.Cores, st.Memory)
		if priceStr != "" {
			if price, err := strconv.ParseFloat(priceStr, 64); err == nil {
				label += fmt.Sprintf(" (~€%.2f/mo)", price)
			} else {
				label += fmt.Sprintf(" (~€%s/mo)", priceStr)
			}
		}

		opts = append(opts, huh.NewOption(label, size))
	}

	if len(opts) > 0 {
		// Sort by CPU cores, then memory
		sort.Slice(opts, func(i, j int) bool {
			a, b := types[indexByName(types, string(opts[i].Value))], types[indexByName(types, string(opts[j].Value))]
			if a.Cores != b.Cores {
				return a.Cores < b.Cores
			}
			return a.Memory < b.Memory
		})
		serverSizeOptions = opts
	}
	return nil
}

// indexByName finds a server type index by name. Returns 0 if not found.
func indexByName(types []*hcloud.ServerType, name string) int {
	for i, st := range types {
		if st.Name == name {
			return i
		}
	}
	return 0
}

// defaultServerSizeOptions returns a static fallback for when no API is available.
func defaultServerSizeOptions() []huh.Option[ServerSize] {
	return []huh.Option[ServerSize]{
		huh.NewOption("CX23 - 2 vCPU, 4GB RAM", SizeCX23),
		huh.NewOption("CX33 - 4 vCPU, 8GB RAM", SizeCX33),
		huh.NewOption("CX43 - 8 vCPU, 16GB RAM", SizeCX43),
		huh.NewOption("CX53 - 16 vCPU, 32GB RAM", SizeCX53),
		huh.NewOption("CPX22 - 2 vCPU, 4GB RAM", SizeCPX22),
		huh.NewOption("CPX32 - 4 vCPU, 8GB RAM", SizeCPX32),
		huh.NewOption("CPX42 - 8 vCPU, 16GB RAM", SizeCPX42),
		huh.NewOption("CPX52 - 16 vCPU, 32GB RAM", SizeCPX52),
	}
}

// WizardResult holds the user's choices from the wizard.
type WizardResult struct {
	Name        string
	Region      Region
	Mode        Mode
	WorkerCount int
	WorkerSize  ServerSize
	Domain      string
	Backup      bool
}

// RunWizard runs the simplified configuration wizard.
func RunWizard(ctx context.Context) (*WizardResult, error) {
	result := &WizardResult{
		// Defaults
		Region:      RegionFalkenstein,
		Mode:        ModeDev,
		WorkerCount: 2,
		WorkerSize:  SizeCX33,
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
				OptionsFunc(func() []huh.Option[ServerSize] {
					if len(serverSizeOptions) > 0 {
						return serverSizeOptions
					}
					return defaultServerSizeOptions()
				}, &result.WorkerSize).
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

		// Backup
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable automated backups?").
				Description("Scheduled etcd snapshots to Hetzner Object Storage (~\u20ac5/mo)").
				Value(&result.Backup),
		),
	)

	// Run the form
	if err := form.RunWithContext(ctx); err != nil {
		return nil, fmt.Errorf("wizard canceled: %w", err)
	}

	return result, nil
}

// ToSpec converts the wizard result to a Spec.
// Populates all fields so the output YAML is explicit and self-documenting.
func (r *WizardResult) ToSpec() *Spec {
	spec := &Spec{
		Name:   r.Name,
		Region: r.Region,
		Mode:   r.Mode,
		Workers: WorkerSpec{
			Count: r.WorkerCount,
			Size:  r.WorkerSize,
		},
		ControlPlane: &ControlPlaneSpec{Size: r.WorkerSize},
		Domain:       r.Domain,
		Backup:       r.Backup,
	}

	// When a domain is set, enable all domain-dependent features with defaults
	if r.Domain != "" {
		spec.ArgoSubdomain = "argo"
		spec.CertEmail = "admin@" + r.Domain
		spec.GrafanaSubdomain = "grafana"
		spec.Monitoring = true
	}

	return spec
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
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
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

// WriteSpecYAML writes the spec config to a YAML file.
// It writes the simplified Spec format, not the expanded Config.
// The expansion to full Config happens at apply/cost time via ExpandSpec.
func WriteSpecYAML(cfg *Spec, path string) error {
	return SaveSpec(cfg, path)
}
