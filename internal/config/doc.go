// Package config defines the internal configuration model used by all
// provisioning and addon subsystems.
//
// The [Config] struct is the canonical representation of a cluster's
// desired state, including control plane topology, worker pools, network
// CIDRs, addon settings, and version pins. It is produced by expanding
// the simplified v2 user config or by converting a CRD spec in the
// operator path.
package config
