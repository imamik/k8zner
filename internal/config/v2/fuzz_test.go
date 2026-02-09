package v2

import (
	"testing"
)

// FuzzLoadFromBytes ensures the YAML parser never panics on arbitrary input.
func FuzzLoadFromBytes(f *testing.F) {
	// Seed corpus: valid minimal config
	f.Add([]byte(`name: test
region: fsn1
mode: dev
workers:
  count: 1
  size: cx23
`))
	// Empty
	f.Add([]byte(``))
	// Invalid YAML
	f.Add([]byte(`{{{`))
	// Partial config
	f.Add([]byte(`name: x`))
	// Unexpected types
	f.Add([]byte(`name: 12345
region: true
mode: [1,2,3]
workers: "string"
`))
	// Deeply nested
	f.Add([]byte(`name: test
workers:
  count: 999999999
  size: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
`))
	// Null values
	f.Add([]byte("name: ~\nregion: ~\nmode: ~\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic — errors are fine
		cfg, _ := LoadFromBytesWithoutValidation(data)
		if cfg != nil {
			// If parsing succeeded, Validate must also not panic
			_ = cfg.Validate()
		}
	})
}

// FuzzExpand ensures Expand never panics on arbitrary Config values.
func FuzzExpand(f *testing.F) {
	f.Add("test", "fsn1", "dev", 1, "cx23", "", "", "", false, false, "")
	f.Add("ha-cluster", "nbg1", "ha", 3, "cx32", "example.com", "argo", "ops@example.com", true, true, "grafana")
	f.Add("", "", "", 0, "", "", "", "", false, false, "")
	f.Add("x", "hel1", "ha", 5, "cpx52", "a.b", "cd", "e@f", true, true, "g")

	f.Fuzz(func(t *testing.T, name, region, mode string, workerCount int, workerSize, domain, argoSub, certEmail string, backup, monitoring bool, grafanaSub string) {
		cfg := &Config{
			Name:             name,
			Region:           Region(region),
			Mode:             Mode(mode),
			Workers:          Worker{Count: workerCount, Size: ServerSize(workerSize)},
			Domain:           domain,
			ArgoSubdomain:    argoSub,
			CertEmail:        certEmail,
			Backup:           backup,
			Monitoring:       monitoring,
			GrafanaSubdomain: grafanaSub,
		}

		// Must not panic — errors are fine
		result, err := Expand(cfg)
		if err != nil {
			return
		}
		// If expand succeeded, spot-check basic invariants
		if result.ClusterName != name {
			t.Errorf("ClusterName = %q, want %q", result.ClusterName, name)
		}
		if result.Location != region {
			t.Errorf("Location = %q, want %q", result.Location, region)
		}
	})
}

// FuzzValidate ensures Validate never panics on arbitrary Config values.
func FuzzValidate(f *testing.F) {
	f.Add("test", "fsn1", "dev", 1, "cx23", "")
	f.Add("", "", "", 0, "", "")
	f.Add("a-b-c", "nbg1", "ha", 5, "cpx52", "example.com")
	f.Add("UPPER", "invalid", "x", -1, "invalid", "not a domain")

	f.Fuzz(func(t *testing.T, name, region, mode string, workerCount int, workerSize, domain string) {
		cfg := &Config{
			Name:    name,
			Region:  Region(region),
			Mode:    Mode(mode),
			Workers: Worker{Count: workerCount, Size: ServerSize(workerSize)},
			Domain:  domain,
		}

		// Must not panic — error or nil are both fine
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
		// Must not panic
		normalized := size.Normalize()
		// Normalize is idempotent
		if normalized.Normalize() != normalized {
			t.Errorf("Normalize not idempotent: %q -> %q -> %q", s, normalized, normalized.Normalize())
		}
	})
}
