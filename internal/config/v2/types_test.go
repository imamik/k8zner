package v2

import (
	"os"
	"testing"
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

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		envVars   map[string]string
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid minimal config",
			config: Config{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: Worker{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: false,
		},
		{
			name: "valid config with domain",
			config: Config{
				Name:   "production",
				Region: RegionNuremberg,
				Mode:   ModeHA,
				Workers: Worker{
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
			config: Config{
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: Worker{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "name is required",
		},
		{
			name: "invalid name - uppercase",
			config: Config{
				Name:   "MyCluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: Worker{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "name must be DNS-safe",
		},
		{
			name: "invalid name - underscore",
			config: Config{
				Name:   "my_cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: Worker{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "name must be DNS-safe",
		},
		{
			name: "invalid name - starts with hyphen",
			config: Config{
				Name:   "-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: Worker{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "name must be DNS-safe",
		},
		{
			name: "invalid region",
			config: Config{
				Name:   "my-cluster",
				Region: Region("invalid"),
				Mode:   ModeHA,
				Workers: Worker{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "region must be one of",
		},
		{
			name: "invalid mode",
			config: Config{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   Mode("invalid"),
				Workers: Worker{
					Count: 3,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "mode must be",
		},
		{
			name: "workers count too low",
			config: Config{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: Worker{
					Count: 0,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "workers.count must be 1-5",
		},
		{
			name: "workers count too high",
			config: Config{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: Worker{
					Count: 6,
					Size:  SizeCX32,
				},
			},
			wantError: true,
			errorMsg:  "workers.count must be 1-5",
		},
		{
			name: "invalid worker size",
			config: Config{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: Worker{
					Count: 3,
					Size:  ServerSize("invalid"),
				},
			},
			wantError: true,
			errorMsg:  "workers.size must be one of",
		},
		{
			name: "domain without CF_API_TOKEN",
			config: Config{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: Worker{
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
			config: Config{
				Name:   "my-cluster",
				Region: RegionFalkenstein,
				Mode:   ModeHA,
				Workers: Worker{
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
					t.Errorf("Config.Validate() expected error containing %q, got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Config.Validate() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Config.Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestConfig_ControlPlaneCount(t *testing.T) {
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
			c := Config{Mode: tt.mode}
			if got := c.ControlPlaneCount(); got != tt.want {
				t.Errorf("Config.ControlPlaneCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_LoadBalancerCount(t *testing.T) {
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
			c := Config{Mode: tt.mode}
			if got := c.LoadBalancerCount(); got != tt.want {
				t.Errorf("Config.LoadBalancerCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_HasDomain(t *testing.T) {
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
			c := Config{Domain: tt.domain}
			if got := c.HasDomain(); got != tt.want {
				t.Errorf("Config.HasDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_TotalWorkerVCPU(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		workers Worker
		want    int
	}{
		{"3x cx32", Worker{Count: 3, Size: SizeCX32}, 12},
		{"5x cx52", Worker{Count: 5, Size: SizeCX52}, 80},
		{"1x cx22", Worker{Count: 1, Size: SizeCX22}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Config{Workers: tt.workers}
			if got := c.TotalWorkerVCPU(); got != tt.want {
				t.Errorf("Config.TotalWorkerVCPU() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_TotalWorkerRAMGB(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		workers Worker
		want    int
	}{
		{"3x cx32", Worker{Count: 3, Size: SizeCX32}, 24},
		{"5x cx52", Worker{Count: 5, Size: SizeCX52}, 160},
		{"1x cx22", Worker{Count: 1, Size: SizeCX22}, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Config{Workers: tt.workers}
			if got := c.TotalWorkerRAMGB(); got != tt.want {
				t.Errorf("Config.TotalWorkerRAMGB() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_HasBackup(t *testing.T) {
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
			c := Config{Backup: tt.backup}
			if got := c.HasBackup(); got != tt.want {
				t.Errorf("Config.HasBackup() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_BackupBucketName(t *testing.T) {
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
			c := Config{Name: tt.clusterName}
			if got := c.BackupBucketName(); got != tt.want {
				t.Errorf("Config.BackupBucketName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_S3Endpoint(t *testing.T) {
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
			c := Config{Region: tt.region}
			if got := c.S3Endpoint(); got != tt.want {
				t.Errorf("Config.S3Endpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_Validate_Backup(t *testing.T) {
	validConfig := Config{
		Name:   "my-cluster",
		Region: RegionFalkenstein,
		Mode:   ModeHA,
		Workers: Worker{
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

			cfg := validConfig
			cfg.Backup = tt.backup
			err := cfg.Validate()

			if tt.wantError {
				if err == nil {
					t.Errorf("Config.Validate() expected error containing %q, got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Config.Validate() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Config.Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestConfig_GetCertEmail(t *testing.T) {
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
			c := Config{CertEmail: tt.certEmail, Domain: tt.domain}
			if got := c.GetCertEmail(); got != tt.want {
				t.Errorf("Config.GetCertEmail() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_ArgoHost(t *testing.T) {
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
			c := Config{Domain: tt.domain, ArgoSubdomain: tt.argoSubdomain}
			if got := c.ArgoHost(); got != tt.want {
				t.Errorf("Config.ArgoHost() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_HasMonitoring(t *testing.T) {
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
			c := Config{Monitoring: tt.monitoring}
			if got := c.HasMonitoring(); got != tt.want {
				t.Errorf("Config.HasMonitoring() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_GetGrafanaSubdomain(t *testing.T) {
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
			c := Config{GrafanaSubdomain: tt.grafanaSubdomain}
			if got := c.GetGrafanaSubdomain(); got != tt.want {
				t.Errorf("Config.GetGrafanaSubdomain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_GrafanaHost(t *testing.T) {
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
			c := Config{Domain: tt.domain, GrafanaSubdomain: tt.grafanaSubdomain}
			if got := c.GrafanaHost(); got != tt.want {
				t.Errorf("Config.GrafanaHost() = %v, want %v", got, tt.want)
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
