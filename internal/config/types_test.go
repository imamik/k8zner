package config

import "testing"

func boolPtr(b bool) *bool {
	return &b
}

func TestIsPrivateFirst(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		clusterAccess string
		want          bool
	}{
		{"private access returns true", "private", true},
		{"public access returns false", "public", false},
		{"empty access returns false", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &Config{ClusterAccess: tt.clusterAccess}
			got := c.IsPrivateFirst()
			if got != tt.want {
				t.Errorf("IsPrivateFirst() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldEnablePublicIPv4(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		clusterAccess string
		ipv4Enabled   *bool
		want          bool
	}{
		{"explicit true overrides public access", "public", boolPtr(true), true},
		{"explicit true overrides private access", "private", boolPtr(true), true},
		{"explicit false overrides public access", "public", boolPtr(false), false},
		{"explicit false overrides private access", "private", boolPtr(false), false},
		{"nil with public access defaults to true", "public", nil, true},
		{"nil with private access defaults to false", "private", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &Config{
				ClusterAccess: tt.clusterAccess,
				Talos: TalosConfig{
					Machine: TalosMachineConfig{
						PublicIPv4Enabled: tt.ipv4Enabled,
					},
				},
			}
			got := c.ShouldEnablePublicIPv4()
			if got != tt.want {
				t.Errorf("ShouldEnablePublicIPv4() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldEnablePublicIPv6(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		ipv6Enabled *bool
		want        bool
	}{
		{"explicit true returns true", boolPtr(true), true},
		{"explicit false returns false", boolPtr(false), false},
		{"nil defaults to true", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &Config{
				Talos: TalosConfig{
					Machine: TalosMachineConfig{
						PublicIPv6Enabled: tt.ipv6Enabled,
					},
				},
			}
			got := c.ShouldEnablePublicIPv6()
			if got != tt.want {
				t.Errorf("ShouldEnablePublicIPv6() = %v, want %v", got, tt.want)
			}
		})
	}
}
