package cluster

import "fmt"

// Names provides centralized resource naming conventions for cluster resources.
// All Hetzner Cloud resources follow consistent naming patterns to enable
// easy identification and cleanup.
type Names struct {
	cluster string
}

// NewNames creates a new Names helper for the given cluster.
func NewNames(clusterName string) *Names {
	return &Names{cluster: clusterName}
}

// Network returns the name for the cluster's VPC network.
// Pattern: ${cluster}
func (n *Names) Network() string {
	return n.cluster
}

// KubeAPILoadBalancer returns the name for the Kubernetes API load balancer.
// Pattern: ${cluster}-kube-api
func (n *Names) KubeAPILoadBalancer() string {
	return fmt.Sprintf("%s-kube-api", n.cluster)
}

// IngressLoadBalancer returns the name for the ingress load balancer.
// Pattern: ${cluster}-ingress
func (n *Names) IngressLoadBalancer() string {
	return fmt.Sprintf("%s-ingress", n.cluster)
}

// ControlPlaneFloatingIP returns the name for the control plane floating IP.
// Pattern: ${cluster}-control-plane-ipv4
func (n *Names) ControlPlaneFloatingIP() string {
	return fmt.Sprintf("%s-control-plane-ipv4", n.cluster)
}

// Firewall returns the name for the cluster firewall.
// Pattern: ${cluster}
func (n *Names) Firewall() string {
	return n.cluster
}

// PlacementGroup returns the name for a placement group.
// Pattern: ${cluster}-${poolName}
func (n *Names) PlacementGroup(poolName string) string {
	return fmt.Sprintf("%s-%s", n.cluster, poolName)
}

// WorkerPlacementGroupShard returns the name for a sharded worker placement group.
// Worker pools with more than 10 servers are split across multiple placement groups
// to avoid Hetzner Cloud's 10-server-per-placement-group limit.
// Pattern: ${cluster}-${poolName}-pg-${shardIndex}
func (n *Names) WorkerPlacementGroupShard(poolName string, shardIndex int) string {
	return fmt.Sprintf("%s-%s-pg-%d", n.cluster, poolName, shardIndex)
}

// Server returns the name for a server in a node pool.
// Pattern: ${cluster}-${poolName}-${index}
func (n *Names) Server(poolName string, index int) string {
	return fmt.Sprintf("%s-%s-%d", n.cluster, poolName, index)
}

// StateMarker returns the name for the cluster state marker certificate.
// This certificate is used to track whether the cluster has been bootstrapped.
// Pattern: ${cluster}-state
func (n *Names) StateMarker() string {
	return fmt.Sprintf("%s-state", n.cluster)
}
