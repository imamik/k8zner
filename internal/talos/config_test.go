package talos_test

import (
	"testing"

	"github.com/sak-d/hcloud-k8s/internal/talos"
)

func TestGenerateConfig(t *testing.T) {
	generator := talos.NewConfigGenerator("test-cluster", "1.2.3.4")

	config, err := generator.Generate("controlplane", "10.0.0.1", []string{"1.2.3.4", "10.0.0.1"})
	if err != nil {
		t.Fatalf("failed to generate config: %v", err)
	}

	if config.Machine().Type().String() != "controlplane" {
		t.Errorf("expected machine type controlplane, got %s", config.Machine().Type().String())
	}
}
