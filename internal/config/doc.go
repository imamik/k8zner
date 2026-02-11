// Package config defines both the user-facing cluster specification and the
// internal configuration model used by all provisioning and addon subsystems.
//
// The [Spec] struct is the simplified, opinionated user configuration requiring
// only 5 fields. [ExpandSpec] converts it to the full [Config] struct that the
// provisioning layer expects. The [Config] struct can also be produced by
// converting a CRD spec in the operator path.
package config
