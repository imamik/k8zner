package labels

import (
	"reflect"
	"testing"
)

func TestLabelBuilder(t *testing.T) {
	clusterName := "test-cluster"
	lb := NewLabelBuilder(clusterName)

	// Verify initial state
	initial := lb.Build()
	if initial["cluster"] != clusterName {
		t.Errorf("Expected cluster label %s, got %s", clusterName, initial["cluster"])
	}
	if len(initial) != 1 {
		t.Errorf("Expected 1 label, got %d", len(initial))
	}

	// Chain methods
	lb.WithRole("worker").
		WithPool("pool-1").
		WithNodePool("nodepool-1").
		WithState("ready").
		WithCustom("custom-key", "custom-value")

	result := lb.Build()

	expected := map[string]string{
		"cluster":    clusterName,
		"role":       "worker",
		"pool":       "pool-1",
		"nodepool":   "nodepool-1",
		"state":      "ready",
		"custom-key": "custom-value",
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Labels mismatch.\nExpected: %v\nGot:      %v", expected, result)
	}
}

func TestLabelBuilder_Merge(t *testing.T) {
	lb := NewLabelBuilder("cluster-1")
	extra := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	lb.Merge(extra)
	result := lb.Build()

	if result["foo"] != "bar" || result["baz"] != "qux" {
		t.Errorf("Merge failed. Got: %v", result)
	}
	if result["cluster"] != "cluster-1" {
		t.Errorf("Cluster label lost during merge")
	}
}

func TestLabelBuilder_Immutability(t *testing.T) {
	lb := NewLabelBuilder("cluster-1")
	map1 := lb.Build()

	// Modify the returned map
	map1["cluster"] = "modified"

	// Check that internal state is not affected
	map2 := lb.Build()
	if map2["cluster"] != "cluster-1" {
		t.Error("LabelBuilder internal state was modified via returned map")
	}
}
