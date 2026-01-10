package naming

import "testing"

func TestNamingFunctions(t *testing.T) {
	cluster := "test-cluster"
	pool := "worker-pool"

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{
			name:     "Network",
			got:      Network(cluster),
			expected: "test-cluster",
		},
		{
			name:     "KubeAPILoadBalancer",
			got:      KubeAPILoadBalancer(cluster),
			expected: "test-cluster-kube-api",
		},
		{
			name:     "IngressLoadBalancer",
			got:      IngressLoadBalancer(cluster),
			expected: "test-cluster-ingress",
		},
		{
			name:     "ControlPlaneFloatingIP",
			got:      ControlPlaneFloatingIP(cluster),
			expected: "test-cluster-control-plane-ipv4",
		},
		{
			name:     "Firewall",
			got:      Firewall(cluster),
			expected: "test-cluster",
		},
		{
			name:     "PlacementGroup",
			got:      PlacementGroup(cluster, pool),
			expected: "test-cluster-worker-pool",
		},
		{
			name:     "WorkerPlacementGroupShard",
			got:      WorkerPlacementGroupShard(cluster, pool, 1),
			expected: "test-cluster-worker-pool-pg-1",
		},
		{
			name:     "Server",
			got:      Server(cluster, pool, 5),
			expected: "test-cluster-worker-pool-5",
		},
		{
			name:     "StateMarker",
			got:      StateMarker(cluster),
			expected: "test-cluster-state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, tt.got)
			}
		})
	}
}
