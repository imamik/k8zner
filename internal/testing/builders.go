package testing

import (
	"maps"

	"hcloud-k8s/internal/config"
)

// ConfigBuilder provides a fluent interface for constructing test configs.
// Each method returns a new builder (immutable) for chaining.
type ConfigBuilder struct {
	cfg config.Config
}

// NewConfigBuilder creates a new ConfigBuilder with sensible defaults.
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		cfg: config.Config{
			ClusterName: "test-cluster",
			Location:    "nbg1",
			Network: config.NetworkConfig{
				IPv4CIDR: "10.0.0.0/16",
				Zone:     "eu-central",
			},
			Talos: config.TalosConfig{
				Version: "v1.8.3",
			},
			Kubernetes: config.KubernetesConfig{
				Version: "v1.31.0",
			},
		},
	}
}

// WithClusterName sets the cluster name.
func (b *ConfigBuilder) WithClusterName(name string) *ConfigBuilder {
	newBuilder := b.clone()
	newBuilder.cfg.ClusterName = name
	return newBuilder
}

// WithLocation sets the datacenter location.
func (b *ConfigBuilder) WithLocation(location string) *ConfigBuilder {
	newBuilder := b.clone()
	newBuilder.cfg.Location = location
	return newBuilder
}

// WithNetwork sets the network configuration.
func (b *ConfigBuilder) WithNetwork(ipv4CIDR, zone string) *ConfigBuilder {
	newBuilder := b.clone()
	newBuilder.cfg.Network = config.NetworkConfig{
		IPv4CIDR: ipv4CIDR,
		Zone:     zone,
	}
	return newBuilder
}

// WithControlPlane adds a control plane node pool.
func (b *ConfigBuilder) WithControlPlane(name, serverType string, count int) *ConfigBuilder {
	newBuilder := b.clone()
	newBuilder.cfg.ControlPlane.NodePools = append(newBuilder.cfg.ControlPlane.NodePools, config.ControlPlaneNodePool{
		Name:       name,
		ServerType: serverType,
		Count:      count,
	})
	return newBuilder
}

// WithWorkers adds a worker node pool.
func (b *ConfigBuilder) WithWorkers(name, serverType string, count int) *ConfigBuilder {
	newBuilder := b.clone()
	newBuilder.cfg.Workers = append(newBuilder.cfg.Workers, config.WorkerNodePool{
		Name:       name,
		ServerType: serverType,
		Count:      count,
	})
	return newBuilder
}

// WithIngress enables ingress with default settings.
func (b *ConfigBuilder) WithIngress(enabled bool) *ConfigBuilder {
	newBuilder := b.clone()
	newBuilder.cfg.Ingress.Enabled = enabled
	return newBuilder
}

// WithSSHKeys sets the SSH keys.
func (b *ConfigBuilder) WithSSHKeys(keys []string) *ConfigBuilder {
	newBuilder := b.clone()
	newBuilder.cfg.SSHKeys = keys
	return newBuilder
}

// Build returns the constructed config.
func (b *ConfigBuilder) Build() *config.Config {
	cfg := b.cfg // copy
	return &cfg
}

// clone creates a deep copy of the builder for immutability.
func (b *ConfigBuilder) clone() *ConfigBuilder {
	newCfg := b.cfg
	// Deep copy slices and nested maps
	if len(b.cfg.ControlPlane.NodePools) > 0 {
		newCfg.ControlPlane.NodePools = make([]config.ControlPlaneNodePool, len(b.cfg.ControlPlane.NodePools))
		for i, pool := range b.cfg.ControlPlane.NodePools {
			newCfg.ControlPlane.NodePools[i] = cloneControlPlaneNodePool(pool)
		}
	}
	if len(b.cfg.Workers) > 0 {
		newCfg.Workers = make([]config.WorkerNodePool, len(b.cfg.Workers))
		for i, pool := range b.cfg.Workers {
			newCfg.Workers[i] = cloneWorkerNodePool(pool)
		}
	}
	if len(b.cfg.SSHKeys) > 0 {
		newCfg.SSHKeys = make([]string, len(b.cfg.SSHKeys))
		copy(newCfg.SSHKeys, b.cfg.SSHKeys)
	}
	return &ConfigBuilder{cfg: newCfg}
}

// cloneControlPlaneNodePool creates a deep copy of a ControlPlaneNodePool.
func cloneControlPlaneNodePool(pool config.ControlPlaneNodePool) config.ControlPlaneNodePool {
	cloned := pool
	cloned.Labels = cloneStringMap(pool.Labels)
	cloned.Annotations = cloneStringMap(pool.Annotations)
	cloned.Taints = cloneStringSlice(pool.Taints)
	return cloned
}

// cloneWorkerNodePool creates a deep copy of a WorkerNodePool.
func cloneWorkerNodePool(pool config.WorkerNodePool) config.WorkerNodePool {
	cloned := pool
	cloned.Labels = cloneStringMap(pool.Labels)
	cloned.Annotations = cloneStringMap(pool.Annotations)
	cloned.Taints = cloneStringSlice(pool.Taints)
	return cloned
}

// cloneStringMap creates a deep copy of a string map.
func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	cloned := make(map[string]string, len(m))
	maps.Copy(cloned, m)
	return cloned
}

// cloneStringSlice creates a copy of a string slice.
func cloneStringSlice(s []string) []string {
	if s == nil {
		return nil
	}
	cloned := make([]string, len(s))
	copy(cloned, s)
	return cloned
}

// MinimalConfig returns a minimal valid config for simple tests.
func MinimalConfig() *config.Config {
	return NewConfigBuilder().Build()
}

// FullConfig returns a complete config with all components for integration tests.
func FullConfig() *config.Config {
	return NewConfigBuilder().
		WithControlPlane("control-plane", "cx21", 1).
		WithWorkers("worker", "cx21", 1).
		WithIngress(false).
		Build()
}
