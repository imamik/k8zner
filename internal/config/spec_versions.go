package config

// VersionMatrix contains all pinned, tested versions for the k8zner stack.
// These versions are validated to work together and are not configurable.
type VersionMatrix struct {
	// Core infrastructure
	Talos      string // Talos Linux version (e.g., "v1.9.0")
	Kubernetes string // Kubernetes version without 'v' prefix (e.g., "1.32.0")

	// CNI and networking
	Cilium string // Cilium CNI version

	// Ingress
	Traefik string // Traefik ingress controller version

	// TLS and DNS
	CertManager string // cert-manager version
	ExternalDNS string // external-dns version

	// GitOps
	ArgoCD string // ArgoCD version

	// Observability
	MetricsServer string // metrics-server version

	// Cloud integration
	HCloudCCM string // Hetzner Cloud Controller Manager version
	HCloudCSI string // Hetzner CSI Driver version
	TalosCCM  string // Talos Cloud Controller Manager version

	// Backup
	TalosBackup string // Talos etcd backup tool version
}

// DefaultVersionMatrix returns the default pinned version matrix.
// All versions are tested together and known to be compatible.
func DefaultVersionMatrix() VersionMatrix {
	return VersionMatrix{
		// Core infrastructure - pinned to stable, tested versions
		Talos:      "v1.9.0",
		Kubernetes: "1.32.0",

		// CNI - Cilium with kube-proxy replacement
		Cilium: "1.16.5",

		// Ingress - Traefik for automatic TLS
		Traefik: "34.3.0", // Chart version (app version ~3.2.x)

		// TLS and DNS
		CertManager: "v1.16.2",
		ExternalDNS: "0.15.1",

		// GitOps
		ArgoCD: "7.7.12", // Chart version (app version ~2.13.x)

		// Observability
		MetricsServer: "3.12.2",

		// Cloud integration
		HCloudCCM: "1.22.0",
		HCloudCSI: "2.12.0",
		TalosCCM:  "v1.11.0",

		// Backup
		// NOTE: Using dev build as latest stable (v0.1.0-beta.2) lacks required features.
		// TODO: Update to stable release when v0.1.0 or later is released.
		// Track: https://github.com/siderolabs/talos-backup/releases
		TalosBackup: "v0.1.0-beta.3-3-g38dad7c",
	}
}

// Hardcoded infrastructure constants
const (
	// DefaultWorkerServerType is the default Hetzner server type for workers.
	// CPX22 (2 shared AMD cores, 4GB RAM) offers better availability than dedicated types.
	DefaultWorkerServerType = "cpx22"

	// LoadBalancerType is the Hetzner load balancer type.
	// LB11 is sufficient for most workloads.
	LoadBalancerType = "lb11"

	// Architecture is the CPU architecture (AMD64 only, no ARM).
	Architecture = "amd64"
)

// Network CIDRs - hardcoded best practices
// IMPORTANT: Pod CIDR must be WITHIN the Network CIDR for Cilium native routing to work.
// Cilium's ipv4NativeRoutingCIDR is set to NetworkCIDR, so all pod traffic must be routable within it.
const (
	// NetworkCIDR is the Hetzner private network CIDR.
	NetworkCIDR = "10.0.0.0/16"

	// NodeCIDR is the CIDR for node IPs within the private network.
	// Uses the lower half of the network CIDR (10.0.0.0 - 10.0.127.255).
	NodeCIDR = "10.0.0.0/17"

	// PodCIDR is the CIDR for pod IPs.
	// MUST be within NetworkCIDR for Cilium native routing.
	// Uses the upper half of the network CIDR (10.0.128.0 - 10.0.255.255).
	PodCIDR = "10.0.128.0/17"

	// ServiceCIDR is the CIDR for Kubernetes service IPs.
	// This is outside the network CIDR but handled by Cilium's kube-proxy replacement.
	ServiceCIDR = "10.96.0.0/12"
)

// Network zone mapping
var regionToZone = map[Region]string{
	RegionNuremberg:   "eu-central",
	RegionFalkenstein: "eu-central",
	RegionHelsinki:    "eu-central",
}

// NetworkZone returns the Hetzner network zone for a region.
func NetworkZone(region Region) string {
	if zone, ok := regionToZone[region]; ok {
		return zone
	}
	return "eu-central"
}
