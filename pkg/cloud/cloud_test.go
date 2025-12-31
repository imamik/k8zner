package cloud

import (
	"testing"

    "github.com/hcloud-k8s/hcloud-k8s/pkg/config"
)

// Mock client would be needed for real unit tests, but hcloud-go doesn't provide easy interface mocking without wrapping.
// For now, we verify that the code compiles and basic logic seems correct.

func TestNewCloud(t *testing.T) {
    cfg := &config.ClusterConfig{ClusterName: "test"}
    c := NewCloud("token", cfg)
    if c == nil {
        t.Fatal("NewCloud returned nil")
    }
}
