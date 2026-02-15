package naming

import (
	"regexp"
	"strings"
	"testing"
)

func TestGenerateID(t *testing.T) {
	t.Parallel()
	t.Run("generates correct length", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
		validChars := regexp.MustCompile(`^[a-z0-9]+$`)
		for i := 0; i < 100; i++ {
			id := GenerateID(IDLength)
			if !validChars.MatchString(id) {
				t.Errorf("GenerateID() = %q contains invalid characters", id)
			}
		}
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
			result := Network(tt.cluster)
			if result != tt.expected {
				t.Errorf("Network(%q) = %q, want %q", tt.cluster, result, tt.expected)
			}
		})
	}
}

func TestKubeAPILoadBalancer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cluster  string
		expected string
	}{
		{"simple cluster name", "my-cluster", "my-cluster-kube"},
		{"single word", "production", "production-kube"},
		{"with numbers", "cluster-01", "cluster-01-kube"},
		{"empty string", "", "-kube"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := KubeAPILoadBalancer(tt.cluster)
			if result != tt.expected {
				t.Errorf("KubeAPILoadBalancer(%q) = %q, want %q", tt.cluster, result, tt.expected)
			}
		})
	}
}

func TestIngressLoadBalancer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cluster  string
		expected string
	}{
		{"simple cluster name", "my-cluster", "my-cluster-ingress"},
		{"single word", "production", "production-ingress"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IngressLoadBalancer(tt.cluster)
			if result != tt.expected {
				t.Errorf("IngressLoadBalancer(%q) = %q, want %q", tt.cluster, result, tt.expected)
			}
		})
	}
}

func TestFirewall(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			result := Firewall(tt.cluster)
			if result != tt.expected {
				t.Errorf("Firewall(%q) = %q, want %q", tt.cluster, result, tt.expected)
			}
		})
	}
}

func TestPlacementGroup(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			result := PlacementGroup(tt.cluster, tt.role)
			if result != tt.expected {
				t.Errorf("PlacementGroup(%q, %q) = %q, want %q", tt.cluster, tt.role, result, tt.expected)
			}
		})
	}
}

func TestWorkerPlacementGroupShard(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			result := WorkerPlacementGroupShard(tt.cluster, tt.poolName, tt.shardIndex)
			if result != tt.expected {
				t.Errorf("WorkerPlacementGroupShard(%q, %q, %d) = %q, want %q",
					tt.cluster, tt.poolName, tt.shardIndex, result, tt.expected)
			}
		})
	}
}

func TestControlPlane(t *testing.T) {
	t.Parallel()
	t.Run("format is correct", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
			result := ControlPlaneWithID(tt.cluster, tt.id)
			if result != tt.expected {
				t.Errorf("ControlPlaneWithID(%q, %q) = %q, want %q", tt.cluster, tt.id, result, tt.expected)
			}
		})
	}
}

func TestWorker(t *testing.T) {
	t.Parallel()
	t.Run("format is correct", func(t *testing.T) {
		t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
			result := WorkerWithID(tt.cluster, tt.id)
			if result != tt.expected {
				t.Errorf("WorkerWithID(%q, %q) = %q, want %q", tt.cluster, tt.id, result, tt.expected)
			}
		})
	}
}

func TestIsWorker(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			result := IsWorker(tt.name)
			if result != tt.expected {
				t.Errorf("IsWorker(%q) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestE2ECluster(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		prefix string
	}{
		{"fullstack", E2EFullStack},
		{"ha", E2EHA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := E2ECluster(tt.prefix)
			if !strings.HasPrefix(result, tt.prefix+"-") {
				t.Errorf("E2ECluster(%q) = %q, should start with %q-", tt.prefix, result, tt.prefix)
			}
			// Should be prefix-{5char}
			parts := strings.Split(result, "-")
			expectedParts := len(strings.Split(tt.prefix, "-")) + 1
			if len(parts) != expectedParts {
				t.Errorf("E2ECluster(%q) = %q, expected %d parts got %d", tt.prefix, result, expectedParts, len(parts))
			}
			// Last part should be 5 chars
			lastPart := parts[len(parts)-1]
			if len(lastPart) != IDLength {
				t.Errorf("E2ECluster() ID part = %q, expected length %d", lastPart, IDLength)
			}
		})
	}
}

func TestRoleAbbrev(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		role     string
		expected string
	}{
		{"control-plane", "control-plane", "cp"},
		{"controlplane", "controlplane", "cp"},
		{"cp", "cp", "cp"},
		{"worker", "worker", "w"},
		{"workers", "workers", "w"},
		{"w", "w", "w"},
		{"unknown long role", "autoscaler", "au"},
		{"unknown short role", "x", "x"},
		{"unknown empty role", "", ""},
		{"unknown uppercase", "MyRole", "my"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := roleAbbrev(tt.role)
			if result != tt.expected {
				t.Errorf("roleAbbrev(%q) = %q, want %q", tt.role, result, tt.expected)
			}
		})
	}
}

func TestGenerateIDZeroLength(t *testing.T) {
	t.Parallel()
	id := GenerateID(0)
	if id != "" {
		t.Errorf("GenerateID(0) = %q, want empty string", id)
	}
}

func TestNamingConsistency(t *testing.T) {
	t.Parallel()
	cluster := "test-cluster"

	t.Run("ControlPlane can be parsed", func(t *testing.T) {
		t.Parallel()
		name := ControlPlane(cluster)
		parsedCluster, role, _, ok := parseServerName(name)
		if !ok {
			t.Errorf("parseServerName failed for ControlPlane name: %s", name)
		}
		if parsedCluster != cluster {
			t.Errorf("parseServerName cluster = %q, want %q", parsedCluster, cluster)
		}
		if role != roleControlPlane {
			t.Errorf("parseServerName role = %q, want %q", role, roleControlPlane)
		}
	})

	t.Run("Worker can be parsed", func(t *testing.T) {
		t.Parallel()
		name := Worker(cluster)
		parsedCluster, role, _, ok := parseServerName(name)
		if !ok {
			t.Errorf("parseServerName failed for Worker name: %s", name)
		}
		if parsedCluster != cluster {
			t.Errorf("parseServerName cluster = %q, want %q", parsedCluster, cluster)
		}
		if role != roleWorker {
			t.Errorf("parseServerName role = %q, want %q", role, roleWorker)
		}
	})
}
