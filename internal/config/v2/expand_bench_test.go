package v2

import (
	"testing"
)

// Package-level vars to prevent compiler optimization of benchmark results.
var (
	benchResultConfig    *Config
	benchResultExpanded  any
	benchResultValidateE error
)

func BenchmarkExpand_DevMode(b *testing.B) {
	cfg := &Config{
		Name:   "bench-dev",
		Region: RegionFalkenstein,
		Mode:   ModeDev,
		Workers: Worker{
			Count: 1,
			Size:  SizeCX23,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for b.Loop() {
		benchResultExpanded, err = Expand(cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExpand_HAMode(b *testing.B) {
	cfg := &Config{
		Name:   "bench-ha",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX33,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for b.Loop() {
		benchResultExpanded, err = Expand(cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExpand_FullConfig(b *testing.B) {
	b.Setenv("CF_API_TOKEN", "bench-token")
	b.Setenv("HETZNER_S3_ACCESS_KEY", "bench-s3-key")
	b.Setenv("HETZNER_S3_SECRET_KEY", "bench-s3-secret")

	cfg := &Config{
		Name:   "bench-full",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 5,
			Size:  SizeCX53,
		},
		ControlPlane: &ControlPlane{
			Size: SizeCX43,
		},
		Domain:           "example.com",
		CertEmail:        "ops@example.com",
		ArgoSubdomain:    "argocd",
		Monitoring:       true,
		GrafanaSubdomain: "metrics",
		Backup:           true,
	}

	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for b.Loop() {
		benchResultExpanded, err = Expand(cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExpand_Parallel(b *testing.B) {
	cfg := &Config{
		Name:   "bench-parallel",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX33,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result, err := Expand(cfg)
			if err != nil {
				b.Fatal(err)
			}
			_ = result
		}
	})
}

func BenchmarkLoadFromBytes_MinimalConfig(b *testing.B) {
	yamlData := []byte(`name: bench-minimal
region: fsn1
mode: dev
workers:
  count: 1
  size: cx23
`)

	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for b.Loop() {
		benchResultConfig, err = LoadFromBytes(yamlData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadFromBytes_FullConfig(b *testing.B) {
	b.Setenv("CF_API_TOKEN", "bench-token")
	b.Setenv("HETZNER_S3_ACCESS_KEY", "bench-s3-key")
	b.Setenv("HETZNER_S3_SECRET_KEY", "bench-s3-secret")

	yamlData := []byte(`name: bench-full
region: fsn1
mode: ha
workers:
  count: 5
  size: cx53
control_plane:
  size: cx43
domain: example.com
cert_email: ops@example.com
argo_subdomain: argocd
monitoring: true
grafana_subdomain: metrics
backup: true
`)

	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for b.Loop() {
		benchResultConfig, err = LoadFromBytes(yamlData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadFromBytesWithoutValidation(b *testing.B) {
	yamlData := []byte(`name: bench-novalidate
region: fsn1
mode: ha
workers:
  count: 3
  size: cx33
domain: example.com
cert_email: ops@example.com
monitoring: true
backup: true
`)

	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for b.Loop() {
		benchResultConfig, err = LoadFromBytesWithoutValidation(yamlData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidate_ValidConfig(b *testing.B) {
	cfg := &Config{
		Name:   "bench-valid",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 3,
			Size:  SizeCX33,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValidateE = cfg.Validate()
	}
}

func BenchmarkValidate_FullConfig(b *testing.B) {
	b.Setenv("CF_API_TOKEN", "bench-token")
	b.Setenv("HETZNER_S3_ACCESS_KEY", "bench-key")
	b.Setenv("HETZNER_S3_SECRET_KEY", "bench-secret")

	cfg := &Config{
		Name:   "bench-valid-full",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
			Count: 5,
			Size:  SizeCX53,
		},
		Domain:     "example.com",
		CertEmail:  "ops@example.com",
		Monitoring: true,
		Backup:     true,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValidateE = cfg.Validate()
	}
}

func BenchmarkValidate_InvalidConfig(b *testing.B) {
	cfg := &Config{
		Name:   "INVALID-NAME",
		Region: "invalid-region",
		Mode:   "invalid-mode",
		Workers: Worker{
			Count: 99,
			Size:  "invalid",
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchResultValidateE = cfg.Validate()
	}
}

func BenchmarkLoadFromBytes_Parallel(b *testing.B) {
	yamlData := []byte(`name: bench-parallel
region: fsn1
mode: ha
workers:
  count: 3
  size: cx33
`)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result, err := LoadFromBytes(yamlData)
			if err != nil {
				b.Fatal(err)
			}
			_ = result
		}
	})
}
