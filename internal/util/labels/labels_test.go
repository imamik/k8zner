package labels

import "testing"

func TestNewLabelBuilder(t *testing.T) {
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
			lb := NewLabelBuilder(tt.clusterName)
			if lb == nil {
				t.Fatal("NewLabelBuilder returned nil")
			}

			labels := lb.Build()
			if labels["cluster"] != tt.clusterName {
				t.Errorf("expected cluster=%q, got %q", tt.clusterName, labels["cluster"])
			}
		})
	}
}

func TestWithRole(t *testing.T) {
	tests := []struct {
		name string
		role string
	}{
		{"control plane", "control-plane"},
		{"worker", "worker"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := NewLabelBuilder("test-cluster").WithRole(tt.role)
			labels := lb.Build()

			if labels["role"] != tt.role {
				t.Errorf("expected role=%q, got %q", tt.role, labels["role"])
			}
			if labels["cluster"] != "test-cluster" {
				t.Error("cluster label should be preserved")
			}
		})
	}
}

func TestWithPool(t *testing.T) {
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
			lb := NewLabelBuilder("test-cluster").WithPool(tt.pool)
			labels := lb.Build()

			if labels["pool"] != tt.pool {
				t.Errorf("expected pool=%q, got %q", tt.pool, labels["pool"])
			}
		})
	}
}

func TestWithNodePool(t *testing.T) {
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
			lb := NewLabelBuilder("test-cluster").WithNodePool(tt.nodepool)
			labels := lb.Build()

			if labels["nodepool"] != tt.nodepool {
				t.Errorf("expected nodepool=%q, got %q", tt.nodepool, labels["nodepool"])
			}
		})
	}
}

func TestWithState(t *testing.T) {
	tests := []struct {
		name  string
		state string
	}{
		{"ready state", "ready"},
		{"provisioning state", "provisioning"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := NewLabelBuilder("test-cluster").WithState(tt.state)
			labels := lb.Build()

			if labels["state"] != tt.state {
				t.Errorf("expected state=%q, got %q", tt.state, labels["state"])
			}
		})
	}
}

func TestWithCustom(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"environment label", "env", "production"},
		{"team label", "team", "platform"},
		{"empty key", "", "value"},
		{"empty value", "key", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := NewLabelBuilder("test-cluster").WithCustom(tt.key, tt.value)
			labels := lb.Build()

			if labels[tt.key] != tt.value {
				t.Errorf("expected %s=%q, got %q", tt.key, tt.value, labels[tt.key])
			}
		})
	}
}

func TestMerge(t *testing.T) {
	t.Run("merge empty map", func(t *testing.T) {
		lb := NewLabelBuilder("test-cluster").Merge(nil)
		labels := lb.Build()

		if len(labels) != 1 {
			t.Errorf("expected 1 label, got %d", len(labels))
		}
	})

	t.Run("merge new labels", func(t *testing.T) {
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
		if labels["cluster"] != "test-cluster" {
			t.Error("cluster label should be preserved")
		}
	})

	t.Run("merge overwrites existing", func(t *testing.T) {
		extra := map[string]string{
			"cluster": "overwritten",
		}
		lb := NewLabelBuilder("test-cluster").Merge(extra)
		labels := lb.Build()

		if labels["cluster"] != "overwritten" {
			t.Errorf("expected merge to overwrite cluster, got %q", labels["cluster"])
		}
	})
}

func TestBuild(t *testing.T) {
	t.Run("returns copy", func(t *testing.T) {
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
	t.Run("full chain", func(t *testing.T) {
		labels := NewLabelBuilder("test-cluster").
			WithRole("worker").
			WithPool("workers").
			WithNodePool("workers").
			WithState("ready").
			WithCustom("env", "production").
			Build()

		expected := map[string]string{
			"cluster":  "test-cluster",
			"role":     "worker",
			"pool":     "workers",
			"nodepool": "workers",
			"state":    "ready",
			"env":      "production",
		}

		if len(labels) != len(expected) {
			t.Errorf("expected %d labels, got %d", len(expected), len(labels))
		}

		for k, v := range expected {
			if labels[k] != v {
				t.Errorf("expected %s=%q, got %q", k, v, labels[k])
			}
		}
	})

	t.Run("order independent", func(t *testing.T) {
		labels1 := NewLabelBuilder("cluster").
			WithRole("worker").
			WithPool("pool").
			Build()

		labels2 := NewLabelBuilder("cluster").
			WithPool("pool").
			WithRole("worker").
			Build()

		if labels1["role"] != labels2["role"] || labels1["pool"] != labels2["pool"] {
			t.Error("label values should be independent of method call order")
		}
	})

	t.Run("last value wins on duplicate calls", func(t *testing.T) {
		labels := NewLabelBuilder("cluster").
			WithRole("first").
			WithRole("second").
			Build()

		if labels["role"] != "second" {
			t.Errorf("expected role=second, got %q", labels["role"])
		}
	})
}

func TestBuilderIsolation(t *testing.T) {
	t.Run("separate builders are independent", func(t *testing.T) {
		lb1 := NewLabelBuilder("cluster-1")
		lb2 := NewLabelBuilder("cluster-2")

		lb1.WithRole("worker")

		labels2 := lb2.Build()
		if _, exists := labels2["role"]; exists {
			t.Error("builders should be isolated from each other")
		}
	})
}
