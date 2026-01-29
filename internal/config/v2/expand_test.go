package v2

import (
	"os"
	"testing"

	"github.com/imamik/k8zner/internal/config"
)

func TestExpand_BasicConfig(t *testing.T) {
	cfg := &Config{
		Name:   "test-cluster",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Verify basic fields
	if expanded.ClusterName != "test-cluster" {
		t.Errorf("ClusterName = %q, want %q", expanded.ClusterName, "test-cluster")
	}
	if expanded.Location != "fsn1" {
		t.Errorf("Location = %q, want %q", expanded.Location, "fsn1")
	}
}

func TestExpand_ControlPlane_DevMode(t *testing.T) {
	cfg := &Config{
		Name:   "dev-cluster",
		Region: RegionNuremberg,
		Mode:   ModeDev,
		Workers: Worker{
			Count: 1,
			Size:  SizeCX22,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Dev mode should have 1 control plane
	if len(expanded.ControlPlane.NodePools) != 1 {
		t.Errorf("ControlPlane.NodePools length = %d, want 1", len(expanded.ControlPlane.NodePools))
	}

	cp := expanded.ControlPlane.NodePools[0]
	if cp.Count != 1 {
		t.Errorf("ControlPlane count = %d, want 1", cp.Count)
	}
	if cp.ServerType != ControlPlaneServerType {
		t.Errorf("ControlPlane type = %q, want %q", cp.ServerType, ControlPlaneServerType)
	}
	if cp.Location != "nbg1" {
		t.Errorf("ControlPlane location = %q, want %q", cp.Location, "nbg1")
	}
}

func TestExpand_ControlPlane_HAMode(t *testing.T) {
	cfg := &Config{
		Name:   "ha-cluster",
		Region: RegionHelsinki,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// HA mode should have 3 control planes
	if len(expanded.ControlPlane.NodePools) != 1 {
		t.Errorf("ControlPlane.NodePools length = %d, want 1", len(expanded.ControlPlane.NodePools))
	}

	cp := expanded.ControlPlane.NodePools[0]
	if cp.Count != 3 {
		t.Errorf("ControlPlane count = %d, want 3", cp.Count)
	}
}

func TestExpand_Workers(t *testing.T) {
	cfg := &Config{
		Name:   "worker-test",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 5,
			Size:  SizeCX52,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Should have exactly 1 worker pool
	if len(expanded.Workers) != 1 {
		t.Errorf("Workers length = %d, want 1", len(expanded.Workers))
	}

	workers := expanded.Workers[0]
	if workers.Count != 5 {
		t.Errorf("Workers count = %d, want 5", workers.Count)
	}
	if workers.ServerType != "cx52" {
		t.Errorf("Workers type = %q, want %q", workers.ServerType, "cx52")
	}
	if workers.Name != "workers" {
		t.Errorf("Workers name = %q, want %q", workers.Name, "workers")
	}
}

func TestExpand_Network(t *testing.T) {
	cfg := &Config{
		Name:   "network-test",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Verify network CIDRs
	if expanded.Network.IPv4CIDR != NetworkCIDR {
		t.Errorf("Network.IPv4CIDR = %q, want %q", expanded.Network.IPv4CIDR, NetworkCIDR)
	}
	if expanded.Network.PodIPv4CIDR != PodCIDR {
		t.Errorf("Network.PodIPv4CIDR = %q, want %q", expanded.Network.PodIPv4CIDR, PodCIDR)
	}
	if expanded.Network.ServiceIPv4CIDR != ServiceCIDR {
		t.Errorf("Network.ServiceIPv4CIDR = %q, want %q", expanded.Network.ServiceIPv4CIDR, ServiceCIDR)
	}
	if expanded.Network.Zone != "eu-central" {
		t.Errorf("Network.Zone = %q, want %q", expanded.Network.Zone, "eu-central")
	}
}

func TestExpand_Talos(t *testing.T) {
	cfg := &Config{
		Name:   "talos-test",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	vm := DefaultVersionMatrix()

	if expanded.Talos.Version != vm.Talos {
		t.Errorf("Talos.Version = %q, want %q", expanded.Talos.Version, vm.Talos)
	}

	// IPv6-only nodes should not have public IPv4
	if expanded.Talos.Machine.PublicIPv4Enabled == nil || *expanded.Talos.Machine.PublicIPv4Enabled {
		t.Error("Talos.Machine.PublicIPv4Enabled should be false for IPv6-only")
	}
}

func TestExpand_Addons(t *testing.T) {
	cfg := &Config{
		Name:   "addon-test",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// All addons should be enabled
	if !expanded.Addons.Cilium.Enabled {
		t.Error("Cilium should be enabled")
	}
	if !expanded.Addons.Traefik.Enabled {
		t.Error("Traefik should be enabled")
	}
	if !expanded.Addons.CertManager.Enabled {
		t.Error("CertManager should be enabled")
	}
	if !expanded.Addons.MetricsServer.Enabled {
		t.Error("MetricsServer should be enabled")
	}
	if !expanded.Addons.CCM.Enabled {
		t.Error("CCM should be enabled")
	}
	if !expanded.Addons.CSI.Enabled {
		t.Error("CSI should be enabled")
	}

	// Ingress-nginx should be disabled (we use Traefik)
	if expanded.Addons.IngressNginx.Enabled {
		t.Error("IngressNginx should be disabled")
	}
}

func TestExpand_WithDomain(t *testing.T) {
	os.Setenv("CF_API_TOKEN", "test-token")
	defer os.Unsetenv("CF_API_TOKEN")

	cfg := &Config{
		Name:   "domain-test",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
		Domain: "example.com",
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Cloudflare should be enabled when domain is set
	if !expanded.Addons.Cloudflare.Enabled {
		t.Error("Cloudflare should be enabled when domain is set")
	}
	if expanded.Addons.Cloudflare.Domain != "example.com" {
		t.Errorf("Cloudflare.Domain = %q, want %q", expanded.Addons.Cloudflare.Domain, "example.com")
	}

	// External DNS should be enabled
	if !expanded.Addons.ExternalDNS.Enabled {
		t.Error("ExternalDNS should be enabled when domain is set")
	}

	// cert-manager Cloudflare should be enabled
	if !expanded.Addons.CertManager.Cloudflare.Enabled {
		t.Error("CertManager Cloudflare should be enabled when domain is set")
	}
}

func TestExpand_WithoutDomain(t *testing.T) {
	cfg := &Config{
		Name:   "no-domain",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Cloudflare should be disabled when no domain
	if expanded.Addons.Cloudflare.Enabled {
		t.Error("Cloudflare should be disabled when no domain")
	}

	// External DNS should be disabled
	if expanded.Addons.ExternalDNS.Enabled {
		t.Error("ExternalDNS should be disabled when no domain")
	}
}

func TestExpand_Ingress_DevMode(t *testing.T) {
	cfg := &Config{
		Name:   "dev-ingress",
		Region: RegionFalkenstein,
		Mode:   ModeDev,
		Workers: Worker{
			Count: 1,
			Size:  SizeCX22,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Dev mode: No separate ingress LB (Traefik uses hostNetwork)
	// This keeps costs low with only 1 API LB
	if expanded.Ingress.Enabled {
		t.Error("Ingress should be disabled in dev mode (no separate LB)")
	}
}

func TestExpand_Traefik_DevMode(t *testing.T) {
	cfg := &Config{
		Name:   "dev-traefik",
		Region: RegionFalkenstein,
		Mode:   ModeDev,
		Workers: Worker{
			Count: 1,
			Size:  SizeCX22,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Traefik always uses hostNetwork with DaemonSet (both modes)
	// In dev mode, traffic goes directly to worker nodes or through shared API LB
	if expanded.Addons.Traefik.HostNetwork == nil || !*expanded.Addons.Traefik.HostNetwork {
		t.Error("Traefik.HostNetwork should be true")
	}
	if expanded.Addons.Traefik.Kind != "DaemonSet" {
		t.Errorf("Traefik.Kind = %q, want %q", expanded.Addons.Traefik.Kind, "DaemonSet")
	}
}

func TestExpand_Traefik_HAMode(t *testing.T) {
	cfg := &Config{
		Name:   "ha-traefik",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Traefik always uses hostNetwork with DaemonSet (both modes)
	// In HA mode, dedicated ingress LB routes to Traefik on workers
	if expanded.Addons.Traefik.HostNetwork == nil || !*expanded.Addons.Traefik.HostNetwork {
		t.Error("Traefik.HostNetwork should be true")
	}
	if expanded.Addons.Traefik.Kind != "DaemonSet" {
		t.Errorf("Traefik.Kind = %q, want %q", expanded.Addons.Traefik.Kind, "DaemonSet")
	}
}

func TestExpand_Ingress_HAMode(t *testing.T) {
	cfg := &Config{
		Name:   "ha-ingress",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// HA mode: Dedicated ingress LB for high availability
	if !expanded.Ingress.Enabled {
		t.Error("Ingress should be enabled in HA mode (dedicated LB)")
	}
	if expanded.Ingress.LoadBalancerType != LoadBalancerType {
		t.Errorf("Ingress.LoadBalancerType = %q, want %q", expanded.Ingress.LoadBalancerType, LoadBalancerType)
	}
}

func TestExpand_Kubernetes(t *testing.T) {
	cfg := &Config{
		Name:   "k8s-test",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	vm := DefaultVersionMatrix()

	if expanded.Kubernetes.Version != vm.Kubernetes {
		t.Errorf("Kubernetes.Version = %q, want %q", expanded.Kubernetes.Version, vm.Kubernetes)
	}

	// API load balancer should be enabled for HA
	if !expanded.Kubernetes.APILoadBalancerEnabled {
		t.Error("APILoadBalancerEnabled should be true for HA mode")
	}
}

// verifyIPv6Only checks that all nodes are configured for IPv6-only
func TestExpand_IPv6Only(t *testing.T) {
	cfg := &Config{
		Name:   "ipv6-test",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Verify IPv6-only configuration
	if expanded.Talos.Machine.PublicIPv4Enabled == nil || *expanded.Talos.Machine.PublicIPv4Enabled {
		t.Error("PublicIPv4Enabled should be false for IPv6-only")
	}
	if expanded.Talos.Machine.IPv6Enabled == nil || !*expanded.Talos.Machine.IPv6Enabled {
		t.Error("IPv6Enabled should be true")
	}
}

// Helper to check expand returns valid internal config
func TestExpand_ReturnsValidConfig(t *testing.T) {
	cfg := &Config{
		Name:   "valid-test",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Type assertion to verify it's the correct type
	var _ *config.Config = expanded
}

func TestExpand_WithBackupEnabled(t *testing.T) {
	os.Setenv("HETZNER_S3_ACCESS_KEY", "test-access-key")
	os.Setenv("HETZNER_S3_SECRET_KEY", "test-secret-key")
	defer os.Unsetenv("HETZNER_S3_ACCESS_KEY")
	defer os.Unsetenv("HETZNER_S3_SECRET_KEY")

	cfg := &Config{
		Name:   "backup-test",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
		Backup: true,
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// TalosBackup should be enabled
	if !expanded.Addons.TalosBackup.Enabled {
		t.Error("TalosBackup should be enabled when backup is true")
	}

	// Verify bucket name
	expectedBucket := "backup-test-etcd-backups"
	if expanded.Addons.TalosBackup.S3Bucket != expectedBucket {
		t.Errorf("TalosBackup.S3Bucket = %q, want %q", expanded.Addons.TalosBackup.S3Bucket, expectedBucket)
	}

	// Verify S3 region
	if expanded.Addons.TalosBackup.S3Region != "fsn1" {
		t.Errorf("TalosBackup.S3Region = %q, want %q", expanded.Addons.TalosBackup.S3Region, "fsn1")
	}

	// Verify S3 endpoint
	expectedEndpoint := "https://fsn1.your-objectstorage.com"
	if expanded.Addons.TalosBackup.S3Endpoint != expectedEndpoint {
		t.Errorf("TalosBackup.S3Endpoint = %q, want %q", expanded.Addons.TalosBackup.S3Endpoint, expectedEndpoint)
	}

	// Verify credentials are from env vars
	if expanded.Addons.TalosBackup.S3AccessKey != "test-access-key" {
		t.Errorf("TalosBackup.S3AccessKey = %q, want %q", expanded.Addons.TalosBackup.S3AccessKey, "test-access-key")
	}
	if expanded.Addons.TalosBackup.S3SecretKey != "test-secret-key" {
		t.Errorf("TalosBackup.S3SecretKey = %q, want %q", expanded.Addons.TalosBackup.S3SecretKey, "test-secret-key")
	}

	// Verify hourly schedule
	if expanded.Addons.TalosBackup.Schedule != "0 * * * *" {
		t.Errorf("TalosBackup.Schedule = %q, want %q", expanded.Addons.TalosBackup.Schedule, "0 * * * *")
	}

	// Verify compression is enabled
	if !expanded.Addons.TalosBackup.EnableCompression {
		t.Error("TalosBackup.EnableCompression should be true")
	}
}

func TestExpand_WithBackupDisabled(t *testing.T) {
	cfg := &Config{
		Name:   "no-backup",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
		Backup: false,
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// TalosBackup should be disabled
	if expanded.Addons.TalosBackup.Enabled {
		t.Error("TalosBackup should be disabled when backup is false")
	}
}

func TestExpand_BackupDifferentRegions(t *testing.T) {
	os.Setenv("HETZNER_S3_ACCESS_KEY", "test-key")
	os.Setenv("HETZNER_S3_SECRET_KEY", "test-secret")
	defer os.Unsetenv("HETZNER_S3_ACCESS_KEY")
	defer os.Unsetenv("HETZNER_S3_SECRET_KEY")

	tests := []struct {
		region           Region
		expectedEndpoint string
	}{
		{RegionNuremberg, "https://nbg1.your-objectstorage.com"},
		{RegionFalkenstein, "https://fsn1.your-objectstorage.com"},
		{RegionHelsinki, "https://hel1.your-objectstorage.com"},
	}

	for _, tt := range tests {
		t.Run(string(tt.region), func(t *testing.T) {
			cfg := &Config{
				Name:   "region-test",
				Region: tt.region,
				Mode:   ModeDev,
				Workers: Worker{
					Count: 1,
					Size:  SizeCX22,
				},
				Backup: true,
			}

			expanded, err := Expand(cfg)
			if err != nil {
				t.Fatalf("Expand() error = %v", err)
			}

			if expanded.Addons.TalosBackup.S3Endpoint != tt.expectedEndpoint {
				t.Errorf("S3Endpoint = %q, want %q", expanded.Addons.TalosBackup.S3Endpoint, tt.expectedEndpoint)
			}
		})
	}
}

func TestExpand_BackupEncryptionDisabled(t *testing.T) {
	os.Setenv("HETZNER_S3_ACCESS_KEY", "test-access-key")
	os.Setenv("HETZNER_S3_SECRET_KEY", "test-secret-key")
	defer os.Unsetenv("HETZNER_S3_ACCESS_KEY")
	defer os.Unsetenv("HETZNER_S3_SECRET_KEY")

	cfg := &Config{
		Name:   "encryption-test",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX32,
		},
		Backup: true,
	}

	expanded, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// v2 config defaults to EncryptionDisabled=true (private bucket provides security)
	if !expanded.Addons.TalosBackup.EncryptionDisabled {
		t.Error("TalosBackup.EncryptionDisabled should be true for v2 config")
	}
}
