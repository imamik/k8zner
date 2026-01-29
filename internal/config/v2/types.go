// Package v2 provides the simplified, opinionated configuration schema.
package v2

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// domainRegex is compiled once at package init for domain validation.
var domainRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*\.[a-zA-Z]{2,}$`)

// Config is the simplified, opinionated configuration for k8zner.
// It requires only 5 fields to deploy a production-ready Kubernetes cluster.
type Config struct {
	// Name is the cluster name, used for resource naming and tagging.
	// Must be DNS-safe: lowercase alphanumeric and hyphens, must start with letter.
	Name string `yaml:"name"`

	// Region is the Hetzner datacenter location.
	Region Region `yaml:"region"`

	// Mode defines the cluster topology (dev or ha).
	Mode Mode `yaml:"mode"`

	// Workers defines the worker pool configuration.
	Workers Worker `yaml:"workers"`

	// Domain enables automatic DNS and TLS via Cloudflare.
	// Requires CF_API_TOKEN environment variable.
	Domain string `yaml:"domain,omitempty"`

	// Backup enables automatic etcd backups to Hetzner Object Storage.
	// Requires HETZNER_S3_ACCESS_KEY and HETZNER_S3_SECRET_KEY environment variables.
	// Creates bucket "{cluster-name}-etcd-backups" automatically.
	Backup bool `yaml:"backup,omitempty"`
}

// Region is a Hetzner datacenter location.
type Region string

const (
	// RegionNuremberg is the Nuremberg, Germany datacenter (nbg1).
	RegionNuremberg Region = "nbg1"
	// RegionFalkenstein is the Falkenstein, Germany datacenter (fsn1).
	RegionFalkenstein Region = "fsn1"
	// RegionHelsinki is the Helsinki, Finland datacenter (hel1).
	RegionHelsinki Region = "hel1"
)

// ValidRegions returns all valid regions.
func ValidRegions() []Region {
	return []Region{RegionNuremberg, RegionFalkenstein, RegionHelsinki}
}

// IsValid returns true if the region is a valid Hetzner location.
func (r Region) IsValid() bool {
	switch r {
	case RegionNuremberg, RegionFalkenstein, RegionHelsinki:
		return true
	default:
		return false
	}
}

// String returns a human-readable description of the region.
func (r Region) String() string {
	switch r {
	case RegionNuremberg:
		return "nbg1 (Nuremberg, Germany)"
	case RegionFalkenstein:
		return "fsn1 (Falkenstein, Germany)"
	case RegionHelsinki:
		return "hel1 (Helsinki, Finland)"
	default:
		return string(r)
	}
}

// Mode defines the cluster topology.
type Mode string

const (
	// ModeDev creates a development cluster with 1 control plane and 1 shared load balancer.
	// Best for: development, testing, side projects.
	// Cost: ~€15-25/mo depending on worker size.
	ModeDev Mode = "dev"

	// ModeHA creates a highly available cluster with 3 control planes and 2 separate load balancers.
	// Best for: production workloads requiring high availability.
	// Cost: ~€45-70/mo depending on worker size.
	ModeHA Mode = "ha"
)

// ValidModes returns all valid modes.
func ValidModes() []Mode {
	return []Mode{ModeDev, ModeHA}
}

// IsValid returns true if the mode is valid.
func (m Mode) IsValid() bool {
	switch m {
	case ModeDev, ModeHA:
		return true
	default:
		return false
	}
}

// ControlPlaneCount returns the number of control plane nodes for this mode.
func (m Mode) ControlPlaneCount() int {
	switch m {
	case ModeDev:
		return 1
	case ModeHA:
		return 3
	default:
		return 0
	}
}

// LoadBalancerCount returns the number of load balancers for this mode.
// Dev mode uses 1 shared LB (API on :6443, ingress on :80/:443).
// HA mode uses 2 separate LBs (dedicated API + dedicated ingress).
func (m Mode) LoadBalancerCount() int {
	switch m {
	case ModeDev:
		return 1
	case ModeHA:
		return 2
	default:
		return 0
	}
}

// String returns a human-readable description of the mode.
func (m Mode) String() string {
	switch m {
	case ModeDev:
		return "dev (1 control plane, 1 shared LB)"
	case ModeHA:
		return "ha (3 control planes, 2 separate LBs)"
	default:
		return string(m)
	}
}

// Worker defines the worker pool configuration.
type Worker struct {
	// Count is the number of worker nodes (1-5).
	Count int `yaml:"count"`

	// Size is the Hetzner server type for workers.
	Size ServerSize `yaml:"size"`
}

// ServerSize is a Hetzner shared instance type.
// Note: Hetzner renamed server types in 2024 (cx22 → cx23, etc.).
// Both old and new names are accepted for backwards compatibility.
type ServerSize string

const (
	// SizeCX22 is kept for backwards compatibility, maps to cx23.
	// Deprecated: Use SizeCX23 instead.
	SizeCX22 ServerSize = "cx22"
	// SizeCX23 is 2 vCPU, 4GB RAM, 40GB disk (~€4.35/mo).
	SizeCX23 ServerSize = "cx23"
	// SizeCX32 is kept for backwards compatibility, maps to cx33.
	// Deprecated: Use SizeCX33 instead.
	SizeCX32 ServerSize = "cx32"
	// SizeCX33 is 4 vCPU, 8GB RAM, 80GB disk (~€8.09/mo).
	SizeCX33 ServerSize = "cx33"
	// SizeCX42 is kept for backwards compatibility, maps to cx43.
	// Deprecated: Use SizeCX43 instead.
	SizeCX42 ServerSize = "cx42"
	// SizeCX43 is 8 vCPU, 16GB RAM, 160GB disk (~€15.59/mo).
	SizeCX43 ServerSize = "cx43"
	// SizeCX52 is kept for backwards compatibility, maps to cx53.
	// Deprecated: Use SizeCX53 instead.
	SizeCX52 ServerSize = "cx52"
	// SizeCX53 is 16 vCPU, 32GB RAM, 320GB disk (~€29.59/mo).
	SizeCX53 ServerSize = "cx53"
)

// ValidServerSizes returns all valid server sizes (new names only).
func ValidServerSizes() []ServerSize {
	return []ServerSize{SizeCX23, SizeCX33, SizeCX43, SizeCX53}
}

// IsValid returns true if the server size is valid.
// Accepts both old (cx22) and new (cx23) server type names.
func (s ServerSize) IsValid() bool {
	switch s {
	case SizeCX22, SizeCX23, SizeCX32, SizeCX33, SizeCX42, SizeCX43, SizeCX52, SizeCX53:
		return true
	default:
		return false
	}
}

// Normalize returns the current Hetzner server type name.
// Converts old names (cx22) to new names (cx23).
func (s ServerSize) Normalize() ServerSize {
	switch s {
	case SizeCX22:
		return SizeCX23
	case SizeCX32:
		return SizeCX33
	case SizeCX42:
		return SizeCX43
	case SizeCX52:
		return SizeCX53
	default:
		return s
	}
}

// ServerSpecs contains the specifications for a server size.
type ServerSpecs struct {
	VCPU   int
	RAMGB  int
	DiskGB int
}

// Specs returns the specifications for this server size.
func (s ServerSize) Specs() ServerSpecs {
	// Normalize first to handle old server type names
	normalized := s.Normalize()
	switch normalized {
	case SizeCX23:
		return ServerSpecs{VCPU: 2, RAMGB: 4, DiskGB: 40}
	case SizeCX33:
		return ServerSpecs{VCPU: 4, RAMGB: 8, DiskGB: 80}
	case SizeCX43:
		return ServerSpecs{VCPU: 8, RAMGB: 16, DiskGB: 160}
	case SizeCX53:
		return ServerSpecs{VCPU: 16, RAMGB: 32, DiskGB: 320}
	default:
		return ServerSpecs{}
	}
}

// String returns a human-readable description of the server size.
func (s ServerSize) String() string {
	specs := s.Specs()
	return fmt.Sprintf("%s (%d vCPU, %dGB RAM)", string(s), specs.VCPU, specs.RAMGB)
}

// Validate validates the configuration and returns an error if invalid.
func (c *Config) Validate() error {
	var errs []error

	// Name: required, DNS-safe
	if c.Name == "" {
		errs = append(errs, errors.New("name is required"))
	} else if !isValidDNSName(c.Name) {
		errs = append(errs, errors.New("name must be DNS-safe (lowercase alphanumeric and hyphens, must start with letter)"))
	}

	// Region: must be valid
	if !c.Region.IsValid() {
		errs = append(errs, fmt.Errorf("region must be one of: %v", ValidRegions()))
	}

	// Mode: must be valid
	if !c.Mode.IsValid() {
		errs = append(errs, fmt.Errorf("mode must be one of: %v", ValidModes()))
	}

	// Workers: count 1-5, valid size
	if c.Workers.Count < 1 || c.Workers.Count > 5 {
		errs = append(errs, errors.New("workers.count must be 1-5"))
	}
	if !c.Workers.Size.IsValid() {
		errs = append(errs, fmt.Errorf("workers.size must be one of: %v", ValidServerSizes()))
	}

	// Domain: if set, validate and check for CF_API_TOKEN
	if c.Domain != "" {
		if !isValidDomain(c.Domain) {
			errs = append(errs, errors.New("domain must be a valid domain name"))
		}
		if os.Getenv("CF_API_TOKEN") == "" {
			errs = append(errs, errors.New("CF_API_TOKEN environment variable required when domain is set"))
		}
	}

	// Backup: if enabled, check for S3 credentials
	if c.Backup {
		if os.Getenv("HETZNER_S3_ACCESS_KEY") == "" {
			errs = append(errs, errors.New("HETZNER_S3_ACCESS_KEY environment variable required when backup is enabled"))
		}
		if os.Getenv("HETZNER_S3_SECRET_KEY") == "" {
			errs = append(errs, errors.New("HETZNER_S3_SECRET_KEY environment variable required when backup is enabled"))
		}
	}

	return errors.Join(errs...)
}

// ControlPlaneCount returns the number of control plane nodes.
func (c *Config) ControlPlaneCount() int {
	return c.Mode.ControlPlaneCount()
}

// LoadBalancerCount returns the number of load balancers.
func (c *Config) LoadBalancerCount() int {
	return c.Mode.LoadBalancerCount()
}

// HasDomain returns true if a domain is configured.
func (c *Config) HasDomain() bool {
	return c.Domain != ""
}

// HasBackup returns true if backup is enabled.
func (c *Config) HasBackup() bool {
	return c.Backup
}

// BackupBucketName returns the S3 bucket name for etcd backups.
func (c *Config) BackupBucketName() string {
	return c.Name + "-etcd-backups"
}

// S3Endpoint returns the Hetzner S3 endpoint for the configured region.
func (c *Config) S3Endpoint() string {
	return "https://" + string(c.Region) + ".your-objectstorage.com"
}

// TotalWorkerVCPU returns the total vCPU across all workers.
func (c *Config) TotalWorkerVCPU() int {
	return c.Workers.Count * c.Workers.Size.Specs().VCPU
}

// TotalWorkerRAMGB returns the total RAM in GB across all workers.
func (c *Config) TotalWorkerRAMGB() int {
	return c.Workers.Count * c.Workers.Size.Specs().RAMGB
}

// ControlPlaneSize returns the server size for control planes.
// This is hardcoded to CX23 as it's sufficient for etcd + API server.
func (c *Config) ControlPlaneSize() ServerSize {
	return SizeCX23
}

// isValidDNSName checks if a string is a valid DNS name.
// Must be lowercase, alphanumeric with hyphens, start with a letter, max 63 chars.
func isValidDNSName(name string) bool {
	if len(name) == 0 || len(name) > 63 {
		return false
	}
	// Must start with lowercase letter
	if name[0] < 'a' || name[0] > 'z' {
		return false
	}
	// Must end with lowercase letter or digit
	last := name[len(name)-1]
	if (last < 'a' || last > 'z') && (last < '0' || last > '9') {
		return false
	}
	// Must contain only lowercase letters, digits, and hyphens
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	// Must not have consecutive hyphens
	if strings.Contains(name, "--") {
		return false
	}
	return true
}

// isValidDomain checks if a string is a valid domain name.
func isValidDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}
	return domainRegex.MatchString(domain)
}
