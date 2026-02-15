// Package naming provides consistent naming functions for Hetzner Cloud resources.
//
// All resources follow predictable naming patterns to enable easy identification,
// cleanup, and management. These functions are shared across packages to ensure
// naming consistency throughout the application.
//
// Naming convention:
//   - Infrastructure: {cluster}-{type} where type is: net, fw, lb, pg
//   - Servers: {cluster}-{role}-{5char} where role is: cp (control-plane), w (worker)
//   - The 5-char suffix is a random alphanumeric ID (a-z0-9) for uniqueness
package naming

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// Role abbreviations for server naming
const (
	RoleControlPlane = "cp"
	RoleWorker       = "w"
)

// Infrastructure type suffixes
const (
	SuffixNetwork        = "net"
	SuffixFirewall       = "fw"
	SuffixKubeAPI        = "kube"    // Load balancer for kubectl/talosctl
	SuffixIngress        = "ingress" // Load balancer for HTTP(S) ingress
	SuffixSSHKey = "key"
	SuffixState  = "state"
)

// IDLength is the length of random IDs for servers
const IDLength = 5

// charset for random ID generation (lowercase alphanumeric)
const charset = "abcdefghijklmnopqrstuvwxyz0123456789"

// GenerateID generates a random alphanumeric ID of the specified length.
// Uses crypto/rand for secure randomness.
func GenerateID(length int) string {
	result := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			// Fallback to a simple incrementing pattern if crypto/rand fails
			result[i] = charset[i%len(charset)]
			continue
		}
		result[i] = charset[n.Int64()]
	}

	return string(result)
}

// Network returns the name for the cluster's private network.
// Format: {cluster}-net
func Network(cluster string) string {
	return fmt.Sprintf("%s-%s", cluster, SuffixNetwork)
}

// KubeAPILoadBalancer returns the name for the Kubernetes API load balancer.
// Format: {cluster}-kube (for kubectl and talosctl access)
func KubeAPILoadBalancer(cluster string) string {
	return fmt.Sprintf("%s-%s", cluster, SuffixKubeAPI)
}

// IngressLoadBalancer returns the name for the ingress load balancer.
// Format: {cluster}-ingress (for HTTP/HTTPS worker traffic)
func IngressLoadBalancer(cluster string) string {
	return fmt.Sprintf("%s-%s", cluster, SuffixIngress)
}

// Firewall returns the name for the cluster firewall.
// Format: {cluster}-fw
func Firewall(cluster string) string {
	return fmt.Sprintf("%s-%s", cluster, SuffixFirewall)
}

// SSHKey returns the name for the cluster SSH key.
// Format: {cluster}-key
func SSHKey(cluster string) string {
	return fmt.Sprintf("%s-%s", cluster, SuffixSSHKey)
}

// PlacementGroup returns the name for a placement group.
// Format: {cluster}-{role}-pg
func PlacementGroup(cluster, role string) string {
	abbrev := roleAbbrev(role)
	return fmt.Sprintf("%s-%s-pg", cluster, abbrev)
}

// WorkerPlacementGroupShard returns the name for a worker placement group shard.
// Workers are distributed across multiple placement groups (10 servers per group).
// Format: {cluster}-w-pg-{shardIndex}
func WorkerPlacementGroupShard(cluster, poolName string, shardIndex int) string {
	return fmt.Sprintf("%s-%s-pg-%d", cluster, RoleWorker, shardIndex)
}

// ControlPlane returns a name for a control plane server with a random ID.
// Format: {cluster}-cp-{5char}
func ControlPlane(cluster string) string {
	return fmt.Sprintf("%s-%s-%s", cluster, RoleControlPlane, GenerateID(IDLength))
}

// ControlPlaneWithID returns a name for a control plane server with a specific ID.
// Format: {cluster}-cp-{id}
func ControlPlaneWithID(cluster, id string) string {
	return fmt.Sprintf("%s-%s-%s", cluster, RoleControlPlane, id)
}

// Worker returns a name for a worker server with a random ID.
// Format: {cluster}-w-{5char}
func Worker(cluster string) string {
	return fmt.Sprintf("%s-%s-%s", cluster, RoleWorker, GenerateID(IDLength))
}

// WorkerWithID returns a name for a worker server with a specific ID.
// Format: {cluster}-w-{id}
func WorkerWithID(cluster, id string) string {
	return fmt.Sprintf("%s-%s-%s", cluster, RoleWorker, id)
}

// StateMarker returns the name for the cluster state marker.
// Format: {cluster}-state
func StateMarker(cluster string) string {
	return fmt.Sprintf("%s-%s", cluster, SuffixState)
}

// ParseServerName extracts cluster name, role, and ID from a server name.
// Returns cluster, role (cp/w), id, and ok (true if parsed successfully).
func ParseServerName(name string) (cluster, role, id string, ok bool) {
	parts := strings.Split(name, "-")
	if len(parts) < 3 {
		return "", "", "", false
	}

	// Last part is the ID
	id = parts[len(parts)-1]

	// Second-to-last part is the role
	role = parts[len(parts)-2]

	// Everything before is the cluster name
	cluster = strings.Join(parts[:len(parts)-2], "-")

	// Validate role
	if role != RoleControlPlane && role != RoleWorker {
		return "", "", "", false
	}

	return cluster, role, id, true
}

// IsControlPlane checks if a server name indicates a control plane node.
func IsControlPlane(name string) bool {
	_, role, _, ok := ParseServerName(name)
	return ok && role == RoleControlPlane
}

// IsWorker checks if a server name indicates a worker node.
func IsWorker(name string) bool {
	_, role, _, ok := ParseServerName(name)
	return ok && role == RoleWorker
}

// roleAbbrev converts a full role name to its abbreviation.
func roleAbbrev(role string) string {
	switch role {
	case "control-plane", "controlplane", "cp":
		return RoleControlPlane
	case "worker", "workers", "w":
		return RoleWorker
	default:
		// For unknown roles, take first letter or first two letters
		if len(role) >= 2 {
			return strings.ToLower(role[:2])
		}
		return strings.ToLower(role)
	}
}

// E2E test cluster name prefixes
const (
	E2EFullStack = "e2e-fs"
	E2EHA        = "e2e-ha"
	E2EUpgrade   = "e2e-up"
)

// E2ECluster generates a short E2E test cluster name.
// Format: {prefix}-{5char} (e.g., e2e-fs-abc12)
func E2ECluster(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, GenerateID(IDLength))
}
