package naming

import (
	"regexp"
	"strings"
	"testing"
)

func TestGenerateID(t *testing.T) {
	t.Run("generates correct length", func(t *testing.T) {
		id := GenerateID(5)
		if len(id) != 5 {
			t.Errorf("GenerateID(5) returned %d chars, want 5", len(id))
		}

		id = GenerateID(10)
		if len(id) != 10 {
			t.Errorf("GenerateID(10) returned %d chars, want 10", len(id))
		}
	})

	t.Run("only contains valid characters", func(t *testing.T) {
		validChars := regexp.MustCompile(`^[a-z0-9]+$`)
		for i := 0; i < 100; i++ {
			id := GenerateID(IDLength)
			if !validChars.MatchString(id) {
				t.Errorf("GenerateID() = %q contains invalid characters", id)
			}
		}
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 1000; i++ {
			id := GenerateID(IDLength)
			if seen[id] {
				// Not necessarily an error, but very unlikely with 36^5 possibilities
				t.Logf("Warning: duplicate ID generated: %s", id)
			}
			seen[id] = true
		}
	})
}

func TestNetwork(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		expected string
	}{
		{"simple cluster name", "my-cluster", "my-cluster-net"},
		{"single word", "production", "production-net"},
		{"with numbers", "cluster-01", "cluster-01-net"},
		{"empty string", "", "-net"},
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
		{"simple cluster name", "my-cluster", "my-cluster-lb"},
		{"single word", "production", "production-lb"},
		{"with numbers", "cluster-01", "cluster-01-lb"},
		{"empty string", "", "-lb"},
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
		{"simple cluster name", "my-cluster", "my-cluster-lb-ingress"},
		{"single word", "production", "production-lb-ingress"},
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
		{"simple cluster name", "my-cluster", "my-cluster-cp-ip"},
		{"single word", "production", "production-cp-ip"},
		{"with numbers", "cluster-01", "cluster-01-cp-ip"},
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
		{"simple cluster name", "my-cluster", "my-cluster-fw"},
		{"single word", "production", "production-fw"},
		{"with numbers", "cluster-01", "cluster-01-fw"},
		{"empty string", "", "-fw"},
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

func TestSSHKey(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		expected string
	}{
		{"simple cluster name", "my-cluster", "my-cluster-key"},
		{"single word", "production", "production-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SSHKey(tt.cluster)
			if result != tt.expected {
				t.Errorf("SSHKey(%q) = %q, want %q", tt.cluster, result, tt.expected)
			}
		})
	}
}

func TestPlacementGroup(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		role     string
		expected string
	}{
		{"control plane", "my-cluster", "control-plane", "my-cluster-cp-pg"},
		{"workers", "prod", "workers", "prod-w-pg"},
		{"short role", "cluster", "cp", "cluster-cp-pg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PlacementGroup(tt.cluster, tt.role)
			if result != tt.expected {
				t.Errorf("PlacementGroup(%q, %q) = %q, want %q", tt.cluster, tt.role, result, tt.expected)
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
		{"first shard", "my-cluster", "workers", 0, "my-cluster-w-pg-0"},
		{"second shard", "my-cluster", "workers", 1, "my-cluster-w-pg-1"},
		{"large index", "prod", "compute", 99, "prod-w-pg-99"},
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

func TestControlPlane(t *testing.T) {
	t.Run("format is correct", func(t *testing.T) {
		name := ControlPlane("my-cluster")
		if !strings.HasPrefix(name, "my-cluster-cp-") {
			t.Errorf("ControlPlane() = %q, should start with 'my-cluster-cp-'", name)
		}
		// Should be cluster-cp-{5char}
		parts := strings.Split(name, "-")
		if len(parts) != 4 { // my-cluster-cp-xxxxx
			t.Errorf("ControlPlane() = %q, expected 4 parts got %d", name, len(parts))
		}
		if len(parts[3]) != IDLength {
			t.Errorf("ControlPlane() ID part = %q, expected length %d", parts[3], IDLength)
		}
	})

	t.Run("generates unique names", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			name := ControlPlane("test")
			if seen[name] {
				t.Errorf("ControlPlane() generated duplicate name: %s", name)
			}
			seen[name] = true
		}
	})
}

func TestControlPlaneWithID(t *testing.T) {
	tests := []struct {
		cluster  string
		id       string
		expected string
	}{
		{"my-cluster", "abc12", "my-cluster-cp-abc12"},
		{"prod", "xyz99", "prod-cp-xyz99"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := ControlPlaneWithID(tt.cluster, tt.id)
			if result != tt.expected {
				t.Errorf("ControlPlaneWithID(%q, %q) = %q, want %q", tt.cluster, tt.id, result, tt.expected)
			}
		})
	}
}

func TestWorker(t *testing.T) {
	t.Run("format is correct", func(t *testing.T) {
		name := Worker("my-cluster")
		if !strings.HasPrefix(name, "my-cluster-w-") {
			t.Errorf("Worker() = %q, should start with 'my-cluster-w-'", name)
		}
		parts := strings.Split(name, "-")
		if len(parts) != 4 { // my-cluster-w-xxxxx
			t.Errorf("Worker() = %q, expected 4 parts got %d", name, len(parts))
		}
		if len(parts[3]) != IDLength {
			t.Errorf("Worker() ID part = %q, expected length %d", parts[3], IDLength)
		}
	})
}

func TestWorkerWithID(t *testing.T) {
	tests := []struct {
		cluster  string
		id       string
		expected string
	}{
		{"my-cluster", "abc12", "my-cluster-w-abc12"},
		{"prod", "xyz99", "prod-w-xyz99"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := WorkerWithID(tt.cluster, tt.id)
			if result != tt.expected {
				t.Errorf("WorkerWithID(%q, %q) = %q, want %q", tt.cluster, tt.id, result, tt.expected)
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

func TestParseServerName(t *testing.T) {
	tests := []struct {
		name        string
		serverName  string
		wantCluster string
		wantRole    string
		wantID      string
		wantOK      bool
	}{
		{"control plane", "my-cluster-cp-abc12", "my-cluster", "cp", "abc12", true},
		{"worker", "my-cluster-w-xyz99", "my-cluster", "w", "xyz99", true},
		{"complex cluster name", "my-prod-cluster-cp-abc12", "my-prod-cluster", "cp", "abc12", true},
		{"single word cluster", "prod-w-abc12", "prod", "w", "abc12", true},
		{"invalid - too few parts", "cluster-abc12", "", "", "", false},
		{"invalid - unknown role", "cluster-x-abc12", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster, role, id, ok := ParseServerName(tt.serverName)
			if ok != tt.wantOK {
				t.Errorf("ParseServerName(%q) ok = %v, want %v", tt.serverName, ok, tt.wantOK)
			}
			if ok && cluster != tt.wantCluster {
				t.Errorf("ParseServerName(%q) cluster = %q, want %q", tt.serverName, cluster, tt.wantCluster)
			}
			if ok && role != tt.wantRole {
				t.Errorf("ParseServerName(%q) role = %q, want %q", tt.serverName, role, tt.wantRole)
			}
			if ok && id != tt.wantID {
				t.Errorf("ParseServerName(%q) id = %q, want %q", tt.serverName, id, tt.wantID)
			}
		})
	}
}

func TestIsControlPlane(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"my-cluster-cp-abc12", true},
		{"prod-cp-xyz99", true},
		{"my-cluster-w-abc12", false},
		{"invalid-name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsControlPlane(tt.name)
			if result != tt.expected {
				t.Errorf("IsControlPlane(%q) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestIsWorker(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"my-cluster-w-abc12", true},
		{"prod-w-xyz99", true},
		{"my-cluster-cp-abc12", false},
		{"invalid-name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWorker(tt.name)
			if result != tt.expected {
				t.Errorf("IsWorker(%q) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestNamingConsistency(t *testing.T) {
	cluster := "test-cluster"

	t.Run("ControlPlane can be parsed", func(t *testing.T) {
		name := ControlPlane(cluster)
		parsedCluster, role, _, ok := ParseServerName(name)
		if !ok {
			t.Errorf("ParseServerName failed for ControlPlane name: %s", name)
		}
		if parsedCluster != cluster {
			t.Errorf("ParseServerName cluster = %q, want %q", parsedCluster, cluster)
		}
		if role != RoleControlPlane {
			t.Errorf("ParseServerName role = %q, want %q", role, RoleControlPlane)
		}
	})

	t.Run("Worker can be parsed", func(t *testing.T) {
		name := Worker(cluster)
		parsedCluster, role, _, ok := ParseServerName(name)
		if !ok {
			t.Errorf("ParseServerName failed for Worker name: %s", name)
		}
		if parsedCluster != cluster {
			t.Errorf("ParseServerName cluster = %q, want %q", parsedCluster, cluster)
		}
		if role != RoleWorker {
			t.Errorf("ParseServerName role = %q, want %q", role, RoleWorker)
		}
	})
}
