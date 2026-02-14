package hcloud

// Architecture represents a CPU architecture supported by Hetzner Cloud.
type Architecture string

const (
	// ArchAMD64 represents x86_64 architecture (Intel/AMD processors).
	ArchAMD64 Architecture = "amd64"

	// ArchARM64 represents ARM64 architecture (CAX servers).
	ArchARM64 Architecture = "arm64"
)

// DetectArchitecture determines the CPU architecture from a Hetzner Cloud server type.
// CAX server types (e.g., cax11, cax21) use ARM64 architecture.
// All other server types use AMD64 (x86_64) architecture.
//
// Examples:
//   - "cpx22", "cpx21", "ccx33" -> amd64
//   - "cax11", "cax21", "cax31" -> arm64
func DetectArchitecture(serverType string) Architecture {
	if len(serverType) >= 3 && serverType[:3] == "cax" {
		return ArchARM64
	}
	return ArchAMD64
}

// GetDefaultServerType returns a default server type for the given architecture.
// These are used for image building and must have disk sizes compatible with
// the smallest production server types. The snapshot disk size must be <= the
// target server disk size, so we use the smallest disk available per arch.
//
// For production, users should explicitly specify appropriate server types
// based on their workload requirements.
func GetDefaultServerType(arch Architecture) string {
	switch arch {
	case ArchARM64:
		return "cax11" // 2 ARM cores, 4 GB RAM, 40 GB disk
	default:
		return "cx23" // 2 shared AMD cores, 4 GB RAM, 40 GB disk (cpx22 has 80GB - too large for cx23 targets)
	}
}

// String returns the string representation of the architecture.
func (a Architecture) String() string {
	return string(a)
}
