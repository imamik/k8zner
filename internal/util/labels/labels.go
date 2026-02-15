// Package labels provides consistent labeling utilities for Hetzner Cloud resources.
//
// This package enforces uniform labeling patterns across all infrastructure resources,
// enabling easy identification, grouping, and management of resources belonging to
// the same cluster.
//
// Standard label keys use the k8zner.io domain prefix for namespacing.
package labels

// Standard label keys for Hetzner Cloud resources.
// Using k8zner.io prefix for clear namespacing.
const (
	// KeyCluster identifies which cluster a resource belongs to
	KeyCluster = "k8zner.io/cluster"

	// KeyRole identifies the role of a server (control-plane, worker)
	KeyRole = "k8zner.io/role"

	// KeyPool identifies the node pool name
	KeyPool = "k8zner.io/pool"

	// KeyManagedBy identifies the management system
	KeyManagedBy = "k8zner.io/managed-by"

	// Legacy keys (for backward compatibility during migration)
	LegacyKeyCluster = "cluster"
	LegacyKeyTestID  = "test-id"

	// legacyKeyRole, legacyKeyPool, legacyKeyNodePool are set by builder methods for backward compat.
	legacyKeyRole     = "role"
	legacyKeyPool     = "pool"
	legacyKeyNodePool = "nodepool"
)

// Role values
const (
	RoleControlPlane = "control-plane"
	RoleWorker       = "worker"
)

// ManagedBy values
const (
	ManagedByK8zner   = "k8zner"
	ManagedByOperator = "k8zner-operator"
)

// LabelBuilder provides a fluent interface for building Hetzner Cloud resource labels.
// Labels are used to identify and group resources belonging to the same cluster.
type LabelBuilder struct {
	labels map[string]string
}

// NewLabelBuilder creates a new label builder with the cluster name pre-set.
// Sets both new and legacy cluster labels for compatibility.
func NewLabelBuilder(clusterName string) *LabelBuilder {
	return &LabelBuilder{
		labels: map[string]string{
			KeyCluster:       clusterName,
			LegacyKeyCluster: clusterName, // Keep legacy for backward compat
			KeyManagedBy:     ManagedByK8zner,
		},
	}
}

// WithRole adds a role label (e.g., "control-plane", "worker").
// Sets both new and legacy role labels for compatibility.
func (lb *LabelBuilder) WithRole(role string) *LabelBuilder {
	lb.labels[KeyRole] = role
	lb.labels[legacyKeyRole] = role // Keep legacy for backward compat
	return lb
}

// WithPool adds a pool name label.
// Sets both new and legacy pool labels for compatibility.
func (lb *LabelBuilder) WithPool(pool string) *LabelBuilder {
	lb.labels[KeyPool] = pool
	lb.labels[legacyKeyPool] = pool // Keep legacy for backward compat
	return lb
}

// WithNodePool adds a nodepool label (used for worker placement groups).
func (lb *LabelBuilder) WithNodePool(pool string) *LabelBuilder {
	lb.labels[legacyKeyNodePool] = pool
	return lb
}

// WithTestIDIfSet adds a test-id label only if testID is non-empty.
// This simplifies conditional test ID patterns throughout the codebase.
func (lb *LabelBuilder) WithTestIDIfSet(testID string) *LabelBuilder {
	if testID != "" {
		lb.labels[LegacyKeyTestID] = testID
	}
	return lb
}

// WithManagedBy sets who manages this resource.
func (lb *LabelBuilder) WithManagedBy(manager string) *LabelBuilder {
	lb.labels[KeyManagedBy] = manager
	return lb
}

// Merge adds all labels from the provided map.
func (lb *LabelBuilder) Merge(extra map[string]string) *LabelBuilder {
	for k, v := range extra {
		lb.labels[k] = v
	}
	return lb
}

// Build returns a copy of the labels map.
// Returns a copy to prevent external mutations.
func (lb *LabelBuilder) Build() map[string]string {
	result := make(map[string]string, len(lb.labels))
	for k, v := range lb.labels {
		result[k] = v
	}
	return result
}

// SelectorForCluster returns a label selector string for all resources in a cluster.
func SelectorForCluster(clusterName string) string {
	return KeyCluster + "=" + clusterName
}

