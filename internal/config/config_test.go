package config_test

import (
	"testing"

	"github.com/sak-d/hcloud-k8s/internal/config"
)

func TestLoadConfig(t *testing.T) {
	input := map[string]interface{}{
		"hcloud_token": "test-token",
		"cluster_name": "test-cluster",
	}

	cfg, err := config.Load(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.HCloudToken != "test-token" {
		t.Errorf("expected token 'test-token', got '%s'", cfg.HCloudToken)
	}
	if cfg.ClusterName != "test-cluster" {
		t.Errorf("expected cluster name 'test-cluster', got '%s'", cfg.ClusterName)
	}
}

func TestValidateConfig_MissingToken(t *testing.T) {
	cfg := &config.Config{
		ClusterName: "test-cluster",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
}
