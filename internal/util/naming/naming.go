package naming

import "fmt"

// Naming functions for cluster resources.
// All Hetzner Cloud resources follow consistent naming patterns to enable
// easy identification and cleanup.

func Network(cluster string) string {
	return cluster
}

func KubeAPILoadBalancer(cluster string) string {
	return fmt.Sprintf("%s-kube-api", cluster)
}

func IngressLoadBalancer(cluster string) string {
	return fmt.Sprintf("%s-ingress", cluster)
}

func ControlPlaneFloatingIP(cluster string) string {
	return fmt.Sprintf("%s-control-plane-ipv4", cluster)
}

func Firewall(cluster string) string {
	return cluster
}

func PlacementGroup(cluster, poolName string) string {
	return fmt.Sprintf("%s-%s", cluster, poolName)
}

func WorkerPlacementGroupShard(cluster, poolName string, shardIndex int) string {
	return fmt.Sprintf("%s-%s-pg-%d", cluster, poolName, shardIndex)
}

func Server(cluster, poolName string, index int) string {
	return fmt.Sprintf("%s-%s-%d", cluster, poolName, index)
}

func StateMarker(cluster string) string {
	return fmt.Sprintf("%s-state", cluster)
}
