package wizard

import "github.com/charmbracelet/huh"

// LocationOption represents a Hetzner Cloud datacenter location.
type LocationOption struct {
	Value       string
	Label       string
	Description string
}

// ServerTypeOption represents a Hetzner Cloud server type.
type ServerTypeOption struct {
	Value        string
	Label        string
	Description  string
	Architecture string // "x86" or "arm"
	Category     string // "shared", "dedicated", or "cost-optimized"
}

// VersionOption represents a Talos or Kubernetes version.
type VersionOption struct {
	Value       string
	Label       string
	Description string
}

// Architecture options for server type filtering.
const (
	ArchX86 = "x86"
	ArchARM = "arm"
)

// Server category options.
const (
	CategoryShared        = "shared"
	CategoryDedicated     = "dedicated"
	CategoryCostOptimized = "cost-optimized"
)

// ArchitectureOptions for the first selection step.
var ArchitectureOptions = []huh.Option[string]{
	huh.NewOption("x86 (AMD/Intel) - Wider compatibility", ArchX86),
	huh.NewOption("ARM (Ampere Altra) - Better price/performance", ArchARM),
}

// ServerCategoryOptions for x86 architecture.
var ServerCategoryOptions = []huh.Option[string]{
	huh.NewOption("Shared vCPU (CPX) - Good for most workloads (Recommended)", CategoryShared),
	huh.NewOption("Cost-Optimized (CX) - Budget-friendly, EU only", CategoryCostOptimized),
	huh.NewOption("Dedicated vCPU (CCX) - Guaranteed performance", CategoryDedicated),
}

// Locations contains all valid Hetzner Cloud datacenter locations.
// Note: ARM (CAX) servers are only available in Germany and Finland.
// Note: CX (cost-optimized) servers are only available in EU regions.
var Locations = []LocationOption{
	{Value: "nbg1", Label: "nbg1", Description: "Nuremberg, Germany"},
	{Value: "fsn1", Label: "fsn1", Description: "Falkenstein, Germany"},
	{Value: "hel1", Label: "hel1", Description: "Helsinki, Finland"},
	{Value: "ash", Label: "ash", Description: "Ashburn, USA"},
	{Value: "hil", Label: "hil", Description: "Hillsboro, USA"},
	{Value: "sin", Label: "sin", Description: "Singapore"},
}

// EULocations contains EU datacenter locations (for ARM and cost-optimized servers).
var EULocations = []LocationOption{
	{Value: "nbg1", Label: "nbg1", Description: "Nuremberg, Germany"},
	{Value: "fsn1", Label: "fsn1", Description: "Falkenstein, Germany"},
	{Value: "hel1", Label: "hel1", Description: "Helsinki, Finland"},
}

// AllServerTypes contains all Hetzner Cloud server types with metadata.
// Data sourced from Hetzner Cloud pricing (January 2025).
var AllServerTypes = []ServerTypeOption{
	// CX - Cost-Optimized Shared (Intel/AMD mix, EU only)
	{Value: "cx22", Label: "cx22", Description: "2 vCPU, 4GB RAM, 40GB SSD", Architecture: ArchX86, Category: CategoryCostOptimized},
	{Value: "cx32", Label: "cx32", Description: "4 vCPU, 8GB RAM, 80GB SSD", Architecture: ArchX86, Category: CategoryCostOptimized},
	{Value: "cx42", Label: "cx42", Description: "8 vCPU, 16GB RAM, 160GB SSD", Architecture: ArchX86, Category: CategoryCostOptimized},
	{Value: "cx52", Label: "cx52", Description: "16 vCPU, 32GB RAM, 320GB SSD", Architecture: ArchX86, Category: CategoryCostOptimized},

	// CPX - Shared vCPU (AMD Genoa, all regions)
	{Value: "cpx11", Label: "cpx11", Description: "2 vCPU, 2GB RAM, 40GB SSD", Architecture: ArchX86, Category: CategoryShared},
	{Value: "cpx21", Label: "cpx21", Description: "3 vCPU, 4GB RAM, 80GB SSD", Architecture: ArchX86, Category: CategoryShared},
	{Value: "cpx31", Label: "cpx31", Description: "4 vCPU, 8GB RAM, 160GB SSD", Architecture: ArchX86, Category: CategoryShared},
	{Value: "cpx41", Label: "cpx41", Description: "8 vCPU, 16GB RAM, 240GB SSD", Architecture: ArchX86, Category: CategoryShared},
	{Value: "cpx51", Label: "cpx51", Description: "16 vCPU, 32GB RAM, 360GB SSD", Architecture: ArchX86, Category: CategoryShared},

	// CCX - Dedicated vCPU (AMD EPYC, all regions)
	{Value: "ccx13", Label: "ccx13", Description: "2 vCPU, 8GB RAM, 80GB SSD", Architecture: ArchX86, Category: CategoryDedicated},
	{Value: "ccx23", Label: "ccx23", Description: "4 vCPU, 16GB RAM, 160GB SSD", Architecture: ArchX86, Category: CategoryDedicated},
	{Value: "ccx33", Label: "ccx33", Description: "8 vCPU, 32GB RAM, 240GB SSD", Architecture: ArchX86, Category: CategoryDedicated},
	{Value: "ccx43", Label: "ccx43", Description: "16 vCPU, 64GB RAM, 360GB SSD", Architecture: ArchX86, Category: CategoryDedicated},
	{Value: "ccx53", Label: "ccx53", Description: "32 vCPU, 128GB RAM, 600GB SSD", Architecture: ArchX86, Category: CategoryDedicated},
	{Value: "ccx63", Label: "ccx63", Description: "48 vCPU, 192GB RAM, 960GB SSD", Architecture: ArchX86, Category: CategoryDedicated},

	// CAX - ARM Shared (Ampere Altra, Germany/Finland only)
	{Value: "cax11", Label: "cax11", Description: "2 vCPU, 4GB RAM, 40GB SSD", Architecture: ArchARM, Category: CategoryShared},
	{Value: "cax21", Label: "cax21", Description: "4 vCPU, 8GB RAM, 80GB SSD", Architecture: ArchARM, Category: CategoryShared},
	{Value: "cax31", Label: "cax31", Description: "8 vCPU, 16GB RAM, 160GB SSD", Architecture: ArchARM, Category: CategoryShared},
	{Value: "cax41", Label: "cax41", Description: "16 vCPU, 32GB RAM, 320GB SSD", Architecture: ArchARM, Category: CategoryShared},
}

// FilterServerTypes filters server types by architecture and category.
func FilterServerTypes(arch, category string) []ServerTypeOption {
	var filtered []ServerTypeOption
	for _, st := range AllServerTypes {
		if st.Architecture == arch && st.Category == category {
			filtered = append(filtered, st)
		}
	}
	return filtered
}

// ControlPlaneServerTypes contains recommended server types for control plane nodes.
// Deprecated: Use FilterServerTypes with architecture and category instead.
var ControlPlaneServerTypes = []ServerTypeOption{
	{Value: "cpx21", Label: "cpx21", Description: "3 vCPU, 4GB RAM (AMD)", Architecture: ArchX86, Category: CategoryShared},
	{Value: "cpx31", Label: "cpx31", Description: "4 vCPU, 8GB RAM (AMD)", Architecture: ArchX86, Category: CategoryShared},
	{Value: "cpx41", Label: "cpx41", Description: "8 vCPU, 16GB RAM (AMD)", Architecture: ArchX86, Category: CategoryShared},
	{Value: "cax21", Label: "cax21", Description: "4 vCPU, 8GB RAM (ARM)", Architecture: ArchARM, Category: CategoryShared},
	{Value: "cax31", Label: "cax31", Description: "8 vCPU, 16GB RAM (ARM)", Architecture: ArchARM, Category: CategoryShared},
	{Value: "ccx13", Label: "ccx13", Description: "2 vCPU, 8GB RAM (Dedicated)", Architecture: ArchX86, Category: CategoryDedicated},
}

// WorkerServerTypes contains recommended server types for worker nodes.
// Deprecated: Use FilterServerTypes with architecture and category instead.
var WorkerServerTypes = []ServerTypeOption{
	{Value: "cpx21", Label: "cpx21", Description: "3 vCPU, 4GB RAM (AMD)", Architecture: ArchX86, Category: CategoryShared},
	{Value: "cpx31", Label: "cpx31", Description: "4 vCPU, 8GB RAM (AMD)", Architecture: ArchX86, Category: CategoryShared},
	{Value: "cpx41", Label: "cpx41", Description: "8 vCPU, 16GB RAM (AMD)", Architecture: ArchX86, Category: CategoryShared},
	{Value: "cpx51", Label: "cpx51", Description: "16 vCPU, 32GB RAM (AMD)", Architecture: ArchX86, Category: CategoryShared},
	{Value: "cax21", Label: "cax21", Description: "4 vCPU, 8GB RAM (ARM)", Architecture: ArchARM, Category: CategoryShared},
	{Value: "cax31", Label: "cax31", Description: "8 vCPU, 16GB RAM (ARM)", Architecture: ArchARM, Category: CategoryShared},
	{Value: "cax41", Label: "cax41", Description: "16 vCPU, 32GB RAM (ARM)", Architecture: ArchARM, Category: CategoryShared},
	{Value: "ccx13", Label: "ccx13", Description: "2 vCPU, 8GB RAM (Dedicated)", Architecture: ArchX86, Category: CategoryDedicated},
	{Value: "ccx23", Label: "ccx23", Description: "4 vCPU, 16GB RAM (Dedicated)", Architecture: ArchX86, Category: CategoryDedicated},
	{Value: "ccx33", Label: "ccx33", Description: "8 vCPU, 32GB RAM (Dedicated)", Architecture: ArchX86, Category: CategoryDedicated},
}

// TalosVersions contains available Talos versions.
var TalosVersions = []VersionOption{
	{Value: "v1.9.0", Label: "v1.9.0", Description: "Latest stable"},
	{Value: "v1.8.3", Label: "v1.8.3", Description: "Previous stable"},
}

// KubernetesVersions contains available Kubernetes versions.
var KubernetesVersions = []VersionOption{
	{Value: "v1.32.0", Label: "v1.32.0", Description: "Latest stable"},
	{Value: "v1.31.0", Label: "v1.31.0", Description: "Previous stable"},
}

// ControlPlaneCountOptions contains valid control plane node counts.
var ControlPlaneCountOptions = []huh.Option[int]{
	huh.NewOption("1 (Development only)", 1),
	huh.NewOption("3 (Recommended for HA)", 3),
	huh.NewOption("5 (Large clusters)", 5),
}

// WorkerCountOptions contains common worker node counts.
var WorkerCountOptions = []huh.Option[int]{
	huh.NewOption("1", 1),
	huh.NewOption("2", 2),
	huh.NewOption("3", 3),
	huh.NewOption("5", 5),
	huh.NewOption("10", 10),
}

// AddonOption represents a cluster addon that can be enabled.
type AddonOption struct {
	Key         string
	Label       string
	Description string
	Default     bool
}

// CNI options for networking.
const (
	CNICilium      = "cilium"
	CNITalosNative = "talos"
	CNINone        = "none"
)

// CNIOptions contains available CNI choices.
var CNIOptions = []huh.Option[string]{
	huh.NewOption("Cilium - Advanced networking with eBPF (Recommended)", CNICilium),
	huh.NewOption("Talos Default (Flannel) - Simple, built-in networking", CNITalosNative),
	huh.NewOption("None - I'll install my own CNI", CNINone),
}

// BasicAddons contains addons shown in basic mode (excluding CNI which is now separate).
var BasicAddons = []AddonOption{
	{Key: "ccm", Label: "Hetzner CCM", Description: "Cloud Controller Manager for load balancers", Default: true},
	{Key: "csi", Label: "Hetzner CSI", Description: "Container Storage Interface for volumes", Default: true},
	{Key: "metrics_server", Label: "Metrics Server", Description: "Resource metrics for HPA/VPA", Default: true},
	{Key: "cert_manager", Label: "Cert Manager", Description: "Automatic TLS certificate management", Default: false},
	{Key: "ingress_nginx", Label: "Ingress NGINX", Description: "HTTP/HTTPS ingress controller", Default: false},
	{Key: "longhorn", Label: "Longhorn", Description: "Distributed block storage", Default: false},
}

// CiliumEncryptionTypes contains encryption type options.
var CiliumEncryptionTypes = []huh.Option[string]{
	huh.NewOption("WireGuard (Recommended)", "wireguard"),
	huh.NewOption("IPsec", "ipsec"),
}

// ClusterAccessModes contains cluster access mode options.
var ClusterAccessModes = []huh.Option[string]{
	huh.NewOption("Public (Recommended)", "public"),
	huh.NewOption("Private", "private"),
}

// LocationsToOptions converts LocationOption slice to huh.Option slice.
func LocationsToOptions() []huh.Option[string] {
	opts := make([]huh.Option[string], len(Locations))
	for i, loc := range Locations {
		opts[i] = huh.NewOption(loc.Label+" - "+loc.Description, loc.Value)
	}
	return opts
}

// ServerTypesToOptions converts ServerTypeOption slice to huh.Option slice.
func ServerTypesToOptions(types []ServerTypeOption) []huh.Option[string] {
	opts := make([]huh.Option[string], len(types))
	for i, st := range types {
		opts[i] = huh.NewOption(st.Label+" - "+st.Description, st.Value)
	}
	return opts
}

// VersionsToOptions converts VersionOption slice to huh.Option slice.
func VersionsToOptions(versions []VersionOption) []huh.Option[string] {
	opts := make([]huh.Option[string], len(versions))
	for i, v := range versions {
		opts[i] = huh.NewOption(v.Label+" - "+v.Description, v.Value)
	}
	return opts
}
