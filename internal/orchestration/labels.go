package orchestration

// LabelBuilder provides a fluent interface for building Hetzner Cloud resource labels.
// Labels are used to identify and group resources belonging to the same cluster.
type LabelBuilder struct {
	labels map[string]string
}

// NewLabelBuilder creates a new label builder with the cluster name pre-set.
func NewLabelBuilder(clusterName string) *LabelBuilder {
	return &LabelBuilder{
		labels: map[string]string{
			"cluster": clusterName,
		},
	}
}

// WithRole adds a role label (e.g., "control-plane", "worker").
func (lb *LabelBuilder) WithRole(role string) *LabelBuilder {
	lb.labels["role"] = role
	return lb
}

// WithPool adds a pool name label.
func (lb *LabelBuilder) WithPool(pool string) *LabelBuilder {
	lb.labels["pool"] = pool
	return lb
}

// WithNodePool adds a nodepool label (used for worker placement groups).
func (lb *LabelBuilder) WithNodePool(pool string) *LabelBuilder {
	lb.labels["nodepool"] = pool
	return lb
}

// WithState adds a state label (used for cluster state tracking).
func (lb *LabelBuilder) WithState(state string) *LabelBuilder {
	lb.labels["state"] = state
	return lb
}

// WithCustom adds a custom key-value label.
func (lb *LabelBuilder) WithCustom(key, value string) *LabelBuilder {
	lb.labels[key] = value
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
