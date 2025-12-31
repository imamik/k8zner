package talos

import (
	"testing"
    "github.com/hcloud-k8s/hcloud-k8s/pkg/config"
)

func TestNewGenerator(t *testing.T) {
    cfg := &config.ClusterConfig{ClusterName: "test"}
    g := NewGenerator(cfg)
    if g == nil {
        t.Fatal("NewGenerator returned nil")
    }
}
