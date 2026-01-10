package naming

import "testing"

func TestNetwork(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		expected string
	}{
		{"simple cluster name", "my-cluster", "my-cluster"},
		{"single word", "production", "production"},
		{"with numbers", "cluster-01", "cluster-01"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Network(tt.cluster)
			if result != tt.expected {
				t.Errorf("Network(%q) = %q, want %q", tt.cluster, result, tt.expected)
			}
		})
	}
}

func TestKubeAPILoadBalancer(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		expected string
	}{
		{"simple cluster name", "my-cluster", "my-cluster-kube-api"},
		{"single word", "production", "production-kube-api"},
		{"with numbers", "cluster-01", "cluster-01-kube-api"},
		{"empty string", "", "-kube-api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := KubeAPILoadBalancer(tt.cluster)
			if result != tt.expected {
				t.Errorf("KubeAPILoadBalancer(%q) = %q, want %q", tt.cluster, result, tt.expected)
			}
		})
	}
}

func TestIngressLoadBalancer(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		expected string
	}{
		{"simple cluster name", "my-cluster", "my-cluster-ingress"},
		{"single word", "production", "production-ingress"},
		{"with numbers", "cluster-01", "cluster-01-ingress"},
		{"empty string", "", "-ingress"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IngressLoadBalancer(tt.cluster)
			if result != tt.expected {
				t.Errorf("IngressLoadBalancer(%q) = %q, want %q", tt.cluster, result, tt.expected)
			}
		})
	}
}

func TestControlPlaneFloatingIP(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		expected string
	}{
		{"simple cluster name", "my-cluster", "my-cluster-control-plane-ipv4"},
		{"single word", "production", "production-control-plane-ipv4"},
		{"with numbers", "cluster-01", "cluster-01-control-plane-ipv4"},
		{"empty string", "", "-control-plane-ipv4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ControlPlaneFloatingIP(tt.cluster)
			if result != tt.expected {
				t.Errorf("ControlPlaneFloatingIP(%q) = %q, want %q", tt.cluster, result, tt.expected)
			}
		})
	}
}

func TestFirewall(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		expected string
	}{
		{"simple cluster name", "my-cluster", "my-cluster"},
		{"single word", "production", "production"},
		{"with numbers", "cluster-01", "cluster-01"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Firewall(tt.cluster)
			if result != tt.expected {
				t.Errorf("Firewall(%q) = %q, want %q", tt.cluster, result, tt.expected)
			}
		})
	}
}

func TestPlacementGroup(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		poolName string
		expected string
	}{
		{"simple names", "my-cluster", "workers", "my-cluster-workers"},
		{"control plane", "prod", "control-plane", "prod-control-plane"},
		{"with numbers", "cluster-01", "pool-2", "cluster-01-pool-2"},
		{"empty cluster", "", "workers", "-workers"},
		{"empty pool", "my-cluster", "", "my-cluster-"},
		{"both empty", "", "", "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PlacementGroup(tt.cluster, tt.poolName)
			if result != tt.expected {
				t.Errorf("PlacementGroup(%q, %q) = %q, want %q", tt.cluster, tt.poolName, result, tt.expected)
			}
		})
	}
}

func TestWorkerPlacementGroupShard(t *testing.T) {
	tests := []struct {
		name       string
		cluster    string
		poolName   string
		shardIndex int
		expected   string
	}{
		{"first shard", "my-cluster", "workers", 0, "my-cluster-workers-pg-0"},
		{"second shard", "my-cluster", "workers", 1, "my-cluster-workers-pg-1"},
		{"large index", "prod", "compute", 99, "prod-compute-pg-99"},
		{"empty names", "", "", 0, "--pg-0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WorkerPlacementGroupShard(tt.cluster, tt.poolName, tt.shardIndex)
			if result != tt.expected {
				t.Errorf("WorkerPlacementGroupShard(%q, %q, %d) = %q, want %q",
					tt.cluster, tt.poolName, tt.shardIndex, result, tt.expected)
			}
		})
	}
}

func TestServer(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		poolName string
		index    int
		expected string
	}{
		{"first server", "my-cluster", "workers", 0, "my-cluster-workers-0"},
		{"fifth server", "my-cluster", "workers", 4, "my-cluster-workers-4"},
		{"control plane", "prod", "control-plane", 1, "prod-control-plane-1"},
		{"large index", "cluster", "pool", 100, "cluster-pool-100"},
		{"empty names", "", "", 0, "--0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Server(tt.cluster, tt.poolName, tt.index)
			if result != tt.expected {
				t.Errorf("Server(%q, %q, %d) = %q, want %q",
					tt.cluster, tt.poolName, tt.index, result, tt.expected)
			}
		})
	}
}

func TestStateMarker(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		expected string
	}{
		{"simple cluster name", "my-cluster", "my-cluster-state"},
		{"single word", "production", "production-state"},
		{"with numbers", "cluster-01", "cluster-01-state"},
		{"empty string", "", "-state"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StateMarker(tt.cluster)
			if result != tt.expected {
				t.Errorf("StateMarker(%q) = %q, want %q", tt.cluster, result, tt.expected)
			}
		})
	}
}

func TestNamingConsistency(t *testing.T) {
	cluster := "test-cluster"
	poolName := "workers"

	t.Run("Network and Firewall use same name", func(t *testing.T) {
		network := Network(cluster)
		firewall := Firewall(cluster)
		if network != firewall {
			t.Errorf("Network and Firewall should return same name: %q vs %q", network, firewall)
		}
	})

	t.Run("Server name contains pool name", func(t *testing.T) {
		server := Server(cluster, poolName, 0)
		placement := PlacementGroup(cluster, poolName)
		// Server name should start with placement group name
		if server[:len(placement)] != placement {
			t.Errorf("Server name %q should start with placement group %q", server, placement)
		}
	})
}
