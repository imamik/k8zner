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
	Value       string
	Label       string
	Description string
}

// VersionOption represents a Talos or Kubernetes version.
type VersionOption struct {
	Value       string
	Label       string
	Description string
}

// Locations contains all valid Hetzner Cloud datacenter locations.
var Locations = []LocationOption{
	{Value: "nbg1", Label: "nbg1", Description: "Nuremberg, Germany"},
	{Value: "fsn1", Label: "fsn1", Description: "Falkenstein, Germany"},
	{Value: "hel1", Label: "hel1", Description: "Helsinki, Finland"},
	{Value: "ash", Label: "ash", Description: "Ashburn, USA"},
	{Value: "hil", Label: "hil", Description: "Hillsboro, USA"},
	{Value: "sin", Label: "sin", Description: "Singapore"},
}

// ControlPlaneServerTypes contains recommended server types for control plane nodes.
var ControlPlaneServerTypes = []ServerTypeOption{
	{Value: "cpx21", Label: "cpx21", Description: "3 vCPU, 4GB RAM (AMD)"},
	{Value: "cpx31", Label: "cpx31", Description: "4 vCPU, 8GB RAM (AMD)"},
	{Value: "cpx41", Label: "cpx41", Description: "8 vCPU, 16GB RAM (AMD)"},
	{Value: "cax21", Label: "cax21", Description: "4 vCPU, 8GB RAM (ARM)"},
	{Value: "cax31", Label: "cax31", Description: "8 vCPU, 16GB RAM (ARM)"},
	{Value: "ccx13", Label: "ccx13", Description: "2 vCPU, 8GB RAM (Dedicated)"},
}

// WorkerServerTypes contains recommended server types for worker nodes.
var WorkerServerTypes = []ServerTypeOption{
	{Value: "cpx21", Label: "cpx21", Description: "3 vCPU, 4GB RAM (AMD)"},
	{Value: "cpx31", Label: "cpx31", Description: "4 vCPU, 8GB RAM (AMD)"},
	{Value: "cpx41", Label: "cpx41", Description: "8 vCPU, 16GB RAM (AMD)"},
	{Value: "cpx51", Label: "cpx51", Description: "16 vCPU, 32GB RAM (AMD)"},
	{Value: "cax21", Label: "cax21", Description: "4 vCPU, 8GB RAM (ARM)"},
	{Value: "cax31", Label: "cax31", Description: "8 vCPU, 16GB RAM (ARM)"},
	{Value: "cax41", Label: "cax41", Description: "16 vCPU, 32GB RAM (ARM)"},
	{Value: "ccx13", Label: "ccx13", Description: "2 vCPU, 8GB RAM (Dedicated)"},
	{Value: "ccx23", Label: "ccx23", Description: "4 vCPU, 16GB RAM (Dedicated)"},
	{Value: "ccx33", Label: "ccx33", Description: "8 vCPU, 32GB RAM (Dedicated)"},
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

// BasicAddons contains addons shown in basic mode.
var BasicAddons = []AddonOption{
	{Key: "cilium", Label: "Cilium CNI", Description: "Container networking with eBPF", Default: true},
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
