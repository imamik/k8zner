package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegion_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		region Region
		want   bool
	}{
		{"valid nbg1", RegionNuremberg, true},
		{"valid fsn1", RegionFalkenstein, true},
		{"valid hel1", RegionHelsinki, true},
		{"invalid empty", Region(""), false},
		{"invalid random", Region("us-east-1"), false},
		{"invalid typo", Region("nbg"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.region.IsValid(); got != tt.want {
				t.Errorf("Region.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegion_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		region Region
		want   string
	}{
		{RegionNuremberg, "nbg1 (Nuremberg, Germany)"},
		{RegionFalkenstein, "fsn1 (Falkenstein, Germany)"},
		{RegionHelsinki, "hel1 (Helsinki, Finland)"},
		{Region("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.region), func(t *testing.T) {
			t.Parallel()
			if got := tt.region.String(); got != tt.want {
				t.Errorf("Region.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMode_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode Mode
		want bool
	}{
		{"valid dev", ModeDev, true},
		{"valid ha", ModeHA, true},
		{"invalid empty", Mode(""), false},
		{"invalid random", Mode("production"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.mode.IsValid(); got != tt.want {
				t.Errorf("Mode.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMode_ControlPlaneCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode Mode
		want int
	}{
		{ModeDev, 1},
		{ModeHA, 3},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			t.Parallel()
			if got := tt.mode.ControlPlaneCount(); got != tt.want {
				t.Errorf("Mode.ControlPlaneCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMode_LoadBalancerCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode Mode
		want int
	}{
		{ModeDev, 1}, // Shared LB
		{ModeHA, 2},  // Separate LBs
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			t.Parallel()
			if got := tt.mode.LoadBalancerCount(); got != tt.want {
				t.Errorf("Mode.LoadBalancerCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServerSize_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		size ServerSize
		want bool
	}{
		// CPX series (shared vCPU)
		{"valid cpx22", SizeCPX22, true},
		{"valid cpx32", SizeCPX32, true},
		{"valid cpx42", SizeCPX42, true},
		{"valid cpx52", SizeCPX52, true},
		// CX series (legacy names)
		{"valid cx22", SizeCX22, true},
		{"valid cx32", SizeCX32, true},
		{"valid cx42", SizeCX42, true},
		{"valid cx52", SizeCX52, true},
		// CX series (new names)
		{"valid cx23", SizeCX23, true},
		{"valid cx33", SizeCX33, true},
		{"valid cx43", SizeCX43, true},
		{"valid cx53", SizeCX53, true},
		// Invalid
		{"invalid empty", ServerSize(""), false},
		{"invalid cax11", ServerSize("cax11"), false}, // ARM not supported
		{"invalid random", ServerSize("large"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.size.IsValid(); got != tt.want {
				t.Errorf("ServerSize.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServerSize_Specs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		size     ServerSize
		wantVCPU int
		wantRAM  int
		wantDisk int
	}{
		// CPX series (shared vCPU) - same specs as CX
		{SizeCPX22, 2, 4, 40},
		{SizeCPX32, 4, 8, 80},
		{SizeCPX42, 8, 16, 160},
		{SizeCPX52, 16, 32, 320},
		// CX series (legacy names map to new specs)
		{SizeCX22, 2, 4, 40},
		{SizeCX32, 4, 8, 80},
		{SizeCX42, 8, 16, 160},
		{SizeCX52, 16, 32, 320},
	}

	for _, tt := range tests {
		t.Run(string(tt.size), func(t *testing.T) {
			t.Parallel()
			specs := tt.size.Specs()
			if specs.VCPU != tt.wantVCPU {
				t.Errorf("Specs().VCPU = %v, want %v", specs.VCPU, tt.wantVCPU)
			}
			if specs.RAMGB != tt.wantRAM {
				t.Errorf("Specs().RAMGB = %v, want %v", specs.RAMGB, tt.wantRAM)
			}
			if specs.DiskGB != tt.wantDisk {
				t.Errorf("Specs().DiskGB = %v, want %v", specs.DiskGB, tt.wantDisk)
			}
		})
	}
}

func TestSpec_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    Spec
		envVars   map[string]string
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid minimal config",
			config: Spec{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: false,
		},
		{
			name: "valid config with domain",
			config: Spec{
				Name:   "production",
				Region: RegionNuremberg,
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 3,
					Size:  SizeCX32,
				},
				Domain: "example.com",
			},
			envVars:   map[string]string{"CF_API_TOKEN": "test-token"},
			wantError: false,
		},
		{
			name: "missing name",
			config: Spec{
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "name is required",
		},
		{
			name: "invalid name - uppercase",
			config: Spec{
				Name:   "MyCluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "name must be DNS-safe",
		},
		{
			name: "invalid name - underscore",
			config: Spec{
				Name:   "my_cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "name must be DNS-safe",
		},
		{
			name: "invalid name - starts with hyphen",
			config: Spec{
				Name:   "-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "name must be DNS-safe",
		},
		{
			name: "invalid region",
			config: Spec{
				Name:   "my-cluster",
				Region: Region("invalid"),
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "region must be one of",
		},
		{
			name: "invalid mode",
			config: Spec{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   Mode("invalid"),
				Workers: WorkerSpec{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "mode must be",
		},
		{
			name: "workers count too low",
			config: Spec{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 0,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "workers.count must be 1-5",
		},
		{
			name: "workers count too high",
			config: Spec{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 6,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "workers.count must be 1-5",
		},
		{
			name: "invalid worker size",
			config: Spec{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 3,
					Size:  ServerSize("invalid"),
				},
			},
			wantError: true,
			errorMsg:  "workers.size must be one of",
		},
		{
			name: "domain without CF_API_TOKEN",
			config: Spec{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 3,
					Size:  SizeCX32,
				},
				Domain: "example.com",
			},
			wantError: true,
			errorMsg:  "CF_API_TOKEN environment variable required",
		},
		{
			name: "invalid domain",
			config: Spec{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: WorkerSpec{
					Count: 3,
					Size:  SizeCX32,
				},
				Domain: "not a domain",
			},
			envVars:   map[string]string{"CF_API_TOKEN": "test-token"},
			wantError: true,
			errorMsg:  "domain must be a valid domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			err := tt.config.Validate()

			if tt.wantError {
				if err == nil {
					t.Errorf("Spec.Validate() expected error containing %q, got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Spec.Validate() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Spec.Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestSpec_ControlPlaneCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode Mode
		want int
	}{
		{ModeDev, 1},
		{ModeHA, 3},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			t.Parallel()
			c := Spec{Mode: tt.mode}
			if got := c.ControlPlaneCount(); got != tt.want {
				t.Errorf("Spec.ControlPlaneCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpec_LoadBalancerCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode Mode
		want int
	}{
		{ModeDev, 1},
		{ModeHA, 2},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			t.Parallel()
			c := Spec{Mode: tt.mode}
			if got := c.LoadBalancerCount(); got != tt.want {
				t.Errorf("Spec.LoadBalancerCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpec_HasDomain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		domain string
		want   bool
	}{
		{"with domain", "example.com", true},
		{"empty domain", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Spec{Domain: tt.domain}
			if got := c.HasDomain(); got != tt.want {
				t.Errorf("Spec.HasDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpec_HasBackup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		backup bool
		want   bool
	}{
		{"backup enabled", true, true},
		{"backup disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Spec{Backup: tt.backup}
			if got := c.HasBackup(); got != tt.want {
				t.Errorf("Spec.HasBackup() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpec_BackupBucketName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		clusterName string
		want        string
	}{
		{"simple name", "mycluster", "mycluster-etcd-backups"},
		{"hyphenated name", "my-cluster", "my-cluster-etcd-backups"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Spec{Name: tt.clusterName}
			if got := c.BackupBucketName(); got != tt.want {
				t.Errorf("Spec.BackupBucketName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpec_S3Endpoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		region Region
		want   string
	}{
		{"nbg1", RegionNuremberg, "https://nbg1.your-objectstorage.com"},
		{"fsn1", RegionFalkenstein, "https://fsn1.your-objectstorage.com"},
		{"hel1", RegionHelsinki, "https://hel1.your-objectstorage.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Spec{Region: tt.region}
			if got := c.S3Endpoint(); got != tt.want {
				t.Errorf("Spec.S3Endpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpec_Validate_Backup(t *testing.T) {
	validSpec := Spec{
		Name:   "my-cluster",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: WorkerSpec{
			Count: 3,
			Size:  SizeCX32,
		},
	}

	tests := []struct {
		name      string
		backup    bool
		envVars   map[string]string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "backup disabled - no credentials needed",
			backup:    false,
			envVars:   map[string]string{},
			wantError: false,
		},
		{
			name:   "backup enabled - with credentials",
			backup: true,
			envVars: map[string]string{
				"HETZNER_S3_ACCESS_KEY": "test-access-key",
				"HETZNER_S3_SECRET_KEY": "test-secret-key",
			},
			wantError: false,
		},
		{
			name:      "backup enabled - missing access key",
			backup:    true,
			envVars:   map[string]string{"HETZNER_S3_SECRET_KEY": "test-secret-key"},
			wantError: true,
			errorMsg:  "HETZNER_S3_ACCESS_KEY environment variable required",
		},
		{
			name:      "backup enabled - missing secret key",
			backup:    true,
			envVars:   map[string]string{"HETZNER_S3_ACCESS_KEY": "test-access-key"},
			wantError: true,
			errorMsg:  "HETZNER_S3_SECRET_KEY environment variable required",
		},
		{
			name:      "backup enabled - missing both keys",
			backup:    true,
			envVars:   map[string]string{},
			wantError: true,
			errorMsg:  "HETZNER_S3_ACCESS_KEY environment variable required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment first
			os.Unsetenv("HETZNER_S3_ACCESS_KEY")
			os.Unsetenv("HETZNER_S3_SECRET_KEY")

			// Set up environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			cfg := validSpec
			cfg.Backup = tt.backup
			err := cfg.Validate()

			if tt.wantError {
				if err == nil {
					t.Errorf("Spec.Validate() expected error containing %q, got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Spec.Validate() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Spec.Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestSpec_GetCertEmail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		certEmail string
		domain    string
		want      string
	}{
		{"explicit email", "ops@company.com", "example.com", "ops@company.com"},
		{"fallback to admin@domain", "", "example.com", "admin@example.com"},
		{"no domain no email", "", "", ""},
		{"explicit email no domain", "ops@company.com", "", "ops@company.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Spec{CertEmail: tt.certEmail, Domain: tt.domain}
			if got := c.GetCertEmail(); got != tt.want {
				t.Errorf("Spec.GetCertEmail() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpec_ArgoHost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		domain        string
		argoSubdomain string
		want          string
	}{
		{"default subdomain", "example.com", "", "argo.example.com"},
		{"custom subdomain", "example.com", "argocd", "argocd.example.com"},
		{"no domain", "", "", ""},
		{"no domain with subdomain", "", "argocd", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Spec{Domain: tt.domain, ArgoSubdomain: tt.argoSubdomain}
			if got := c.ArgoHost(); got != tt.want {
				t.Errorf("Spec.ArgoHost() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpec_HasMonitoring(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		monitoring bool
		want       bool
	}{
		{"monitoring enabled", true, true},
		{"monitoring disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Spec{Monitoring: tt.monitoring}
			if got := c.HasMonitoring(); got != tt.want {
				t.Errorf("Spec.HasMonitoring() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpec_GetGrafanaSubdomain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		grafanaSubdomain string
		want             string
	}{
		{"default", "", "grafana"},
		{"custom", "metrics", "metrics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Spec{GrafanaSubdomain: tt.grafanaSubdomain}
			if got := c.GetGrafanaSubdomain(); got != tt.want {
				t.Errorf("Spec.GetGrafanaSubdomain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpec_GrafanaHost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		domain           string
		grafanaSubdomain string
		want             string
	}{
		{"default subdomain", "example.com", "", "grafana.example.com"},
		{"custom subdomain", "example.com", "metrics", "metrics.example.com"},
		{"no domain", "", "", ""},
		{"no domain with subdomain", "", "metrics", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Spec{Domain: tt.domain, GrafanaSubdomain: tt.grafanaSubdomain}
			if got := c.GrafanaHost(); got != tt.want {
				t.Errorf("Spec.GrafanaHost() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMode_ControlPlaneCount_Default(t *testing.T) {
	t.Parallel()
	// Unknown mode should return 0
	assert.Equal(t, 0, Mode("unknown").ControlPlaneCount())
}

func TestMode_LoadBalancerCount_Default(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, Mode("unknown").LoadBalancerCount())
}

func TestMode_String_Default(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "custom-mode", Mode("custom-mode").String())
}

func TestSpec_ControlPlaneSize(t *testing.T) {
	t.Parallel()
	t.Run("nil control plane returns default", func(t *testing.T) {
		t.Parallel()
		c := Spec{ControlPlane: nil}
		assert.Equal(t, SizeCX23, c.ControlPlaneSize())
	})

	t.Run("empty size returns default", func(t *testing.T) {
		t.Parallel()
		c := Spec{ControlPlane: &ControlPlaneSpec{Size: ""}}
		assert.Equal(t, SizeCX23, c.ControlPlaneSize())
	})

	t.Run("explicit size is returned", func(t *testing.T) {
		t.Parallel()
		c := Spec{ControlPlane: &ControlPlaneSpec{Size: SizeCX33}}
		assert.Equal(t, SizeCX33, c.ControlPlaneSize())
	})

	t.Run("old size name is normalized", func(t *testing.T) {
		t.Parallel()
		c := Spec{ControlPlane: &ControlPlaneSpec{Size: SizeCX32}}
		assert.Equal(t, SizeCX33, c.ControlPlaneSize())
	})
}

func TestIsValidDNSName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid simple", "myapp", true},
		{"valid with hyphen", "my-app", true},
		{"valid with number", "app1", true},
		{"empty", "", false},
		{"starts with digit", "1app", false},
		{"starts with hyphen", "-app", false},
		{"ends with hyphen", "app-", false},
		{"uppercase", "MyApp", false},
		{"underscore", "my_app", false},
		{"consecutive hyphens", "my--app", false},
		{"too long", "a" + strings.Repeat("b", 63), false},
		{"max length valid", "a" + strings.Repeat("b", 61) + "c", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isValidDNSName(tt.input))
		})
	}
}

func TestIsValidDomain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid", "example.com", true},
		{"valid subdomain", "sub.example.com", true},
		{"empty", "", false},
		{"too long", func() string {
			s := ""
			for i := 0; i < 50; i++ {
				s += "abcde."
			}
			return s + "com"
		}(), false},
		{"no dot", "localhost", false},
		{"spaces", "exa mple.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isValidDomain(tt.input))
		})
	}
}

func TestServerSize_Specs_Default(t *testing.T) {
	t.Parallel()
	// Unknown size should return zero specs
	specs := ServerSize("unknown").Specs()
	assert.Equal(t, 0, specs.VCPU)
	assert.Equal(t, 0, specs.RAMGB)
	assert.Equal(t, 0, specs.DiskGB)
}
