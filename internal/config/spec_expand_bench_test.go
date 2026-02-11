package config

import (
	"testing"
)

// Package-level vars to prevent compiler optimization of benchmark results.
var (
	benchResultSpec      *Spec
	benchResultExpanded  any
	benchResultValidateE error
)

func BenchmarkExpandSpec_DevMode(b *testing.B) {
	cfg := &Spec{
		Name:   "bench-dev",
		Region: RegionFalkenstein,
		Mode:   ModeDev,
		Workers: WorkerSpec{
			Count: 1,
			Size:  SizeCX23,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for b.Loop() {
		benchResultExpanded, err = ExpandSpec(cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExpandSpec_HAMode(b *testing.B) {
	cfg := &Spec{
		Name:   "bench-ha",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: WorkerSpec{
			Count: 3,
			Size:  SizeCX33,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for b.Loop() {
		benchResultExpanded, err = ExpandSpec(cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExpandSpec_FullConfig(b *testing.B) {
	b.Setenv("CF_API_TOKEN", "bench-token")
	b.Setenv("HETZNER_S3_ACCESS_KEY", "bench-s3-key")
	b.Setenv("HETZNER_S3_SECRET_KEY", "bench-s3-secret")

	cfg := &Spec{
		Name:   "bench-full",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: WorkerSpec{
			Count: 5,
			Size:  SizeCX53,
		},
		ControlPlane: &ControlPlaneSpec{
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
		benchResultExpanded, err = ExpandSpec(cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExpandSpec_Parallel(b *testing.B) {
	cfg := &Spec{
		Name:   "bench-parallel",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: WorkerSpec{
			Count: 3,
			Size:  SizeCX33,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result, err := ExpandSpec(cfg)
			if err != nil {
				b.Fatal(err)
			}
			_ = result
		}
	})
}

func BenchmarkLoadSpecFromBytes_MinimalConfig(b *testing.B) {
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
		benchResultSpec, err = LoadSpecFromBytes(yamlData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadSpecFromBytes_FullConfig(b *testing.B) {
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
		benchResultSpec, err = LoadSpecFromBytes(yamlData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadSpecFromBytesWithoutValidation(b *testing.B) {
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
		benchResultSpec, err = LoadSpecFromBytesWithoutValidation(yamlData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidate_ValidSpec(b *testing.B) {
	cfg := &Spec{
		Name:   "bench-valid",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: WorkerSpec{
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

func BenchmarkValidate_FullSpec(b *testing.B) {
	b.Setenv("CF_API_TOKEN", "bench-token")
	b.Setenv("HETZNER_S3_ACCESS_KEY", "bench-key")
	b.Setenv("HETZNER_S3_SECRET_KEY", "bench-secret")

	cfg := &Spec{
		Name:   "bench-valid-full",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: WorkerSpec{
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

func BenchmarkValidate_InvalidSpec(b *testing.B) {
	cfg := &Spec{
		Name:   "INVALID-NAME",
		Region: "invalid-region",
		Mode:   "invalid-mode",
		Workers: WorkerSpec{
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

func BenchmarkLoadSpecFromBytes_Parallel(b *testing.B) {
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
			result, err := LoadSpecFromBytes(yamlData)
			if err != nil {
				b.Fatal(err)
			}
			_ = result
		}
	})
}
