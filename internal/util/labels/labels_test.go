package labels

import "testing"

func TestNewLabelBuilder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		clusterName string
	}{
		{"simple cluster name", "my-cluster"},
		{"single word", "production"},
		{"with numbers", "cluster-01"},
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lb := NewLabelBuilder(tt.clusterName)
			if lb == nil {
				t.Fatal("NewLabelBuilder returned nil")
			}

			labels := lb.Build()

			// Check new key
			if labels[KeyCluster] != tt.clusterName {
				t.Errorf("expected %s=%q, got %q", KeyCluster, tt.clusterName, labels[KeyCluster])
			}

			// Check legacy key for backward compat
			if labels[LegacyKeyCluster] != tt.clusterName {
				t.Errorf("expected %s=%q, got %q", LegacyKeyCluster, tt.clusterName, labels[LegacyKeyCluster])
			}

			// Check managed-by is set
			if labels[KeyManagedBy] != ManagedByK8zner {
				t.Errorf("expected %s=%q, got %q", KeyManagedBy, ManagedByK8zner, labels[KeyManagedBy])
			}
		})
	}
}

func TestWithRole(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		role string
	}{
		{"control plane", RoleControlPlane},
		{"worker", RoleWorker},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lb := NewLabelBuilder("test-cluster").WithRole(tt.role)
			labels := lb.Build()

			// Check new key
			if labels[KeyRole] != tt.role {
				t.Errorf("expected %s=%q, got %q", KeyRole, tt.role, labels[KeyRole])
			}

			// Check legacy key for backward compat
			if labels[legacyKeyRole] != tt.role {
				t.Errorf("expected %s=%q, got %q", legacyKeyRole, tt.role, labels[legacyKeyRole])
			}

			if labels[KeyCluster] != "test-cluster" {
				t.Error("cluster label should be preserved")
			}
		})
	}
}

func TestWithPool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		pool string
	}{
		{"workers pool", "workers"},
		{"numbered pool", "pool-1"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lb := NewLabelBuilder("test-cluster").WithPool(tt.pool)
			labels := lb.Build()

			// Check new key
			if labels[KeyPool] != tt.pool {
				t.Errorf("expected %s=%q, got %q", KeyPool, tt.pool, labels[KeyPool])
			}

			// Check legacy key for backward compat
			if labels[legacyKeyPool] != tt.pool {
				t.Errorf("expected %s=%q, got %q", legacyKeyPool, tt.pool, labels[legacyKeyPool])
			}
		})
	}
}

func TestWithNodePool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		nodepool string
	}{
		{"workers nodepool", "workers"},
		{"numbered nodepool", "nodepool-1"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lb := NewLabelBuilder("test-cluster").WithNodePool(tt.nodepool)
			labels := lb.Build()

			if labels[legacyKeyNodePool] != tt.nodepool {
				t.Errorf("expected %s=%q, got %q", legacyKeyNodePool, tt.nodepool, labels[legacyKeyNodePool])
			}
		})
	}
}

func TestWithManagedBy(t *testing.T) {
	t.Parallel()
	lb := NewLabelBuilder("test-cluster").WithManagedBy(ManagedByOperator)
	labels := lb.Build()

	if labels[KeyManagedBy] != ManagedByOperator {
		t.Errorf("expected %s=%q, got %q", KeyManagedBy, ManagedByOperator, labels[KeyManagedBy])
	}
}

func TestMerge(t *testing.T) {
	t.Parallel()
	t.Run("merge empty map", func(t *testing.T) {
		t.Parallel()
		lb := NewLabelBuilder("test-cluster").Merge(nil)
		labels := lb.Build()

		// Should have at least: k8zner.io/cluster, cluster, k8zner.io/managed-by
		if len(labels) < 3 {
			t.Errorf("expected at least 3 labels, got %d", len(labels))
		}
	})

	t.Run("merge new labels", func(t *testing.T) {
		t.Parallel()
		extra := map[string]string{
			"env":  "production",
			"team": "platform",
		}
		lb := NewLabelBuilder("test-cluster").Merge(extra)
		labels := lb.Build()

		if labels["env"] != "production" {
			t.Errorf("expected env=production, got %q", labels["env"])
		}
		if labels["team"] != "platform" {
			t.Errorf("expected team=platform, got %q", labels["team"])
		}
		if labels[KeyCluster] != "test-cluster" {
			t.Error("cluster label should be preserved")
		}
	})

	t.Run("merge overwrites existing", func(t *testing.T) {
		t.Parallel()
		extra := map[string]string{
			KeyCluster: "overwritten",
		}
		lb := NewLabelBuilder("test-cluster").Merge(extra)
		labels := lb.Build()

		if labels[KeyCluster] != "overwritten" {
			t.Errorf("expected merge to overwrite cluster, got %q", labels[KeyCluster])
		}
	})
}

func TestBuild(t *testing.T) {
	t.Parallel()
	t.Run("returns copy", func(t *testing.T) {
		t.Parallel()
		lb := NewLabelBuilder("test-cluster")
		labels1 := lb.Build()
		labels2 := lb.Build()

		// Modify first copy
		labels1["modified"] = "yes"

		// Second copy should not be affected
		if _, exists := labels2["modified"]; exists {
			t.Error("Build should return independent copies")
		}
	})

	t.Run("builder not affected by returned map", func(t *testing.T) {
		t.Parallel()
		lb := NewLabelBuilder("test-cluster")
		labels := lb.Build()

		// Modify returned map
		labels["new-key"] = "new-value"

		// Builder should not be affected
		labels2 := lb.Build()
		if _, exists := labels2["new-key"]; exists {
			t.Error("Builder internal state should not be affected by modifications to returned map")
		}
	})
}

func TestFluentChaining(t *testing.T) {
	t.Parallel()
	t.Run("full chain", func(t *testing.T) {
		t.Parallel()
		labels := NewLabelBuilder("test-cluster").
			WithRole(RoleWorker).
			WithPool("workers").
			WithNodePool("workers").
			WithTestIDIfSet("e2e-123").
			Build()

		expected := map[string]string{
			KeyCluster:   "test-cluster",
			KeyRole:      RoleWorker,
			KeyPool:      "workers",
			KeyManagedBy: ManagedByK8zner,
		}

		for k, v := range expected {
			if labels[k] != v {
				t.Errorf("expected %s=%q, got %q", k, v, labels[k])
			}
		}

		if labels[LegacyKeyCluster] != "test-cluster" {
			t.Errorf("expected legacy cluster label")
		}
		if labels[legacyKeyRole] != RoleWorker {
			t.Errorf("expected legacy role label")
		}
	})

	t.Run("order independent", func(t *testing.T) {
		t.Parallel()
		labels1 := NewLabelBuilder("cluster").
			WithRole(RoleWorker).
			WithPool("pool").
			Build()

		labels2 := NewLabelBuilder("cluster").
			WithPool("pool").
			WithRole(RoleWorker).
			Build()

		if labels1[KeyRole] != labels2[KeyRole] || labels1[KeyPool] != labels2[KeyPool] {
			t.Error("label values should be independent of method call order")
		}
	})

	t.Run("last value wins on duplicate calls", func(t *testing.T) {
		t.Parallel()
		labels := NewLabelBuilder("cluster").
			WithRole("first").
			WithRole("second").
			Build()

		if labels[KeyRole] != "second" {
			t.Errorf("expected %s=second, got %q", KeyRole, labels[KeyRole])
		}
	})
}

func TestBuilderIsolation(t *testing.T) {
	t.Parallel()
	t.Run("separate builders are independent", func(t *testing.T) {
		t.Parallel()
		lb1 := NewLabelBuilder("cluster-1")
		lb2 := NewLabelBuilder("cluster-2")

		lb1.WithRole(RoleWorker)

		labels2 := lb2.Build()
		if _, exists := labels2[KeyRole]; exists {
			t.Error("builders should be isolated from each other")
		}
	})
}

func TestWithTestIDIfSet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		testID      string
		expectLabel bool
		expectedVal string
	}{
		{"non-empty adds label", "e2e-12345", true, "e2e-12345"},
		{"empty does not add label", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lb := NewLabelBuilder("test-cluster").WithTestIDIfSet(tt.testID)
			labels := lb.Build()

			_, exists := labels[LegacyKeyTestID]
			if exists != tt.expectLabel {
				t.Errorf("expected label exists=%v, got %v", tt.expectLabel, exists)
			}
			if tt.expectLabel && labels[LegacyKeyTestID] != tt.expectedVal {
				t.Errorf("expected %s=%q, got %q", LegacyKeyTestID, tt.expectedVal, labels[LegacyKeyTestID])
			}
		})
	}
}

func TestWithTestIDIfSetChaining(t *testing.T) {
	t.Parallel()
	// Test that it works in a fluent chain

	labels := NewLabelBuilder("test-cluster").
		WithRole(RoleWorker).
		WithTestIDIfSet("e2e-test-123").
		WithPool("workers").
		Build()

	if labels[LegacyKeyTestID] != "e2e-test-123" {
		t.Errorf("expected %s=e2e-test-123, got %q", LegacyKeyTestID, labels[LegacyKeyTestID])
	}
	if labels[KeyRole] != RoleWorker {
		t.Errorf("expected %s=%s, got %q", KeyRole, RoleWorker, labels[KeyRole])
	}
	if labels[KeyPool] != "workers" {
		t.Errorf("expected %s=workers, got %q", KeyPool, labels[KeyPool])
	}
}

func TestSelectorForCluster(t *testing.T) {
	t.Parallel()
	selector := SelectorForCluster("my-cluster")
	expected := "k8zner.io/cluster=my-cluster"
	if selector != expected {
		t.Errorf("SelectorForCluster() = %q, want %q", selector, expected)
	}
}
