// Package naming provides consistent naming functions for Hetzner Cloud resources.
//
// All resources follow predictable naming patterns to enable easy identification,
// cleanup, and management. These functions are shared across packages to ensure
// naming consistency throughout the application.
package naming

import "fmt"

// Network returns the name for the cluster's private network.
// Format: {cluster}
func Network(cluster string) string {
	return cluster
}

// KubeAPILoadBalancer returns the name for the Kubernetes API load balancer.
// Format: {cluster}-kube-api
func KubeAPILoadBalancer(cluster string) string {
	return fmt.Sprintf("%s-kube-api", cluster)
}

// IngressLoadBalancer returns the name for the ingress load balancer.
// Format: {cluster}-ingress
func IngressLoadBalancer(cluster string) string {
	return fmt.Sprintf("%s-ingress", cluster)
}

// ControlPlaneFloatingIP returns the name for the control plane floating IP.
// Format: {cluster}-control-plane-ipv4
func ControlPlaneFloatingIP(cluster string) string {
	return fmt.Sprintf("%s-control-plane-ipv4", cluster)
}

// Firewall returns the name for the cluster firewall.
// Format: {cluster}
func Firewall(cluster string) string {
	return cluster
}

// PlacementGroup returns the name for a placement group.
// Format: {cluster}-{poolName}
func PlacementGroup(cluster, poolName string) string {
	return fmt.Sprintf("%s-%s", cluster, poolName)
}

// WorkerPlacementGroupShard returns the name for a worker placement group shard.
// Workers are distributed across multiple placement groups (10 servers per group).
// Format: {cluster}-{poolName}-pg-{shardIndex}
func WorkerPlacementGroupShard(cluster, poolName string, shardIndex int) string {
	return fmt.Sprintf("%s-%s-pg-%d", cluster, poolName, shardIndex)
}

// Server returns the name for a server.
// Format: {cluster}-{poolName}-{index}
func Server(cluster, poolName string, index int) string {
	return fmt.Sprintf("%s-%s-%d", cluster, poolName, index)
}

// StateMarker returns the name for the cluster state marker.
// Format: {cluster}-state
func StateMarker(cluster string) string {
	return fmt.Sprintf("%s-state", cluster)
}
