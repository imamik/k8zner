// Package v2 provides the simplified, opinionated configuration schema.
//
// Users specify only 5 required fields (name, region, mode, workers, domain)
// and the package fills in production-ready defaults for networking, addons,
// and version pins via the [Expand] function.
//
// The schema enforces safe limits (1-5 workers, EU regions, x86-64 only)
// and validates all fields before expansion. Optional fields like domain,
// backup, and monitoring enable additional addons when set.
package v2
