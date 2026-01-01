package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	content := `
clusterName: "test-cluster"
hetzner:
  region: "hel1"
  networkZone: "eu-central"
  sshKeys: ["key1"]
  firewall:
    apiSource: ["0.0.0.0/0"]
nodes:
  controlPlane:
    count: 3
    type: "cpx21"
    floatingIp: true
  workers:
    nodepools:
      - name: "worker-1"
        count: 3
        type: "cpx21"
        placementGroup: true
talos:
  version: "v1.9.0"
kubernetes:
  version: "1.32.0"
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ClusterName != "test-cluster" {
		t.Errorf("expected clusterName 'test-cluster', got %s", cfg.ClusterName)
	}
	if cfg.Hetzner.Region != "hel1" {
		t.Errorf("expected region 'hel1', got %s", cfg.Hetzner.Region)
	}
}

func TestValidate(t *testing.T) {
	cfg := &Config{}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for empty config")
	}
}
