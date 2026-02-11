// Package provisioning adapts CLI provisioning phases for use by the operator.
//
// The adapter wraps existing infrastructure, image, compute, and cluster
// provisioners with operator-specific logic such as CRD status updates and
// credential loading from Kubernetes secrets. This avoids duplicating
// provisioning code between the CLI and operator paths.
package provisioning
