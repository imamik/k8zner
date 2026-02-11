package config

import (
	"testing"
)

// FuzzLoadSpecFromBytes ensures the YAML parser never panics on arbitrary input.
func FuzzLoadSpecFromBytes(f *testing.F) {
	f.Add([]byte(`name: test
region: fsn1
mode: dev
workers:
  count: 1
  size: cx23
`))
	f.Add([]byte(``))
	f.Add([]byte(`{{{`))
	f.Add([]byte(`name: x`))
	f.Add([]byte(`name: 12345
region: true
mode: [1,2,3]
workers: "string"
`))
	f.Add([]byte(`name: test
workers:
  count: 999999999
  size: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
`))
	f.Add([]byte("name: ~\nregion: ~\nmode: ~\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, _ := LoadSpecFromBytesWithoutValidation(data)
		if cfg != nil {
			_ = cfg.Validate()
		}
	})
}

// FuzzExpandSpec ensures ExpandSpec never panics on arbitrary Spec values.
func FuzzExpandSpec(f *testing.F) {
	f.Add("test", "fsn1", "dev", 1, "cx23", "", "", "", false, false, "")
	f.Add("ha-cluster", "nbg1", "ha", 3, "cx32", "example.com", "argo", "ops@example.com", true, true, "grafana")
	f.Add("", "", "", 0, "", "", "", "", false, false, "")
	f.Add("x", "hel1", "ha", 5, "cpx52", "a.b", "cd", "e@f", true, true, "g")

	f.Fuzz(func(t *testing.T, name, region, mode string, workerCount int, workerSize, domain, argoSub, certEmail string, backup, monitoring bool, grafanaSub string) {
		cfg := &Spec{
			Name:             name,
			Region:           Region(region),
			Mode:             Mode(mode),
			Workers:          WorkerSpec{Count: workerCount, Size: ServerSize(workerSize)},
			Domain:           domain,
			ArgoSubdomain:    argoSub,
			CertEmail:        certEmail,
			Backup:           backup,
			Monitoring:       monitoring,
			GrafanaSubdomain: grafanaSub,
		}

		result, err := ExpandSpec(cfg)
		if err != nil {
			return
		}
		if result.ClusterName != name {
			t.Errorf("ClusterName = %q, want %q", result.ClusterName, name)
		}
		if result.Location != region {
			t.Errorf("Location = %q, want %q", result.Location, region)
		}
	})
}

// FuzzValidateSpec ensures Validate never panics on arbitrary Spec values.
func FuzzValidateSpec(f *testing.F) {
	f.Add("test", "fsn1", "dev", 1, "cx23", "")
	f.Add("", "", "", 0, "", "")
	f.Add("a-b-c", "nbg1", "ha", 5, "cpx52", "example.com")
	f.Add("UPPER", "invalid", "x", -1, "invalid", "not a domain")

	f.Fuzz(func(t *testing.T, name, region, mode string, workerCount int, workerSize, domain string) {
		cfg := &Spec{
			Name:    name,
			Region:  Region(region),
			Mode:    Mode(mode),
			Workers: WorkerSpec{Count: workerCount, Size: ServerSize(workerSize)},
			Domain:  domain,
		}

		_ = cfg.Validate()
	})
}

// FuzzServerSizeNormalize ensures Normalize never panics on arbitrary input.
func FuzzServerSizeNormalize(f *testing.F) {
	f.Add("cx22")
	f.Add("cx32")
	f.Add("cpx22")
	f.Add("")
	f.Add("arbitrary-string")

	f.Fuzz(func(t *testing.T, s string) {
		size := ServerSize(s)
		normalized := size.Normalize()
		if normalized.Normalize() != normalized {
			t.Errorf("Normalize not idempotent: %q -> %q -> %q", s, normalized, normalized.Normalize())
		}
	})
}
