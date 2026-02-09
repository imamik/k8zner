package hcloud

import "testing"

func TestDetectArchitecture(t *testing.T) {
	t.Parallel()
	tests := []struct {
		serverType string
		expected   Architecture
	}{
		// AMD64 server types
		{"cpx11", ArchAMD64},
		{"cpx21", ArchAMD64},
		{"cpx31", ArchAMD64},
		{"cpx41", ArchAMD64},
		{"cpx51", ArchAMD64},
		{"cx11", ArchAMD64},
		{"cx21", ArchAMD64},
		{"cx31", ArchAMD64},
		{"cx41", ArchAMD64},
		{"cx51", ArchAMD64},
		{"ccx11", ArchAMD64},
		{"ccx21", ArchAMD64},
		{"ccx31", ArchAMD64},
		{"ccx33", ArchAMD64},

		// ARM64 server types (CAX)
		{"cax11", ArchARM64},
		{"cax21", ArchARM64},
		{"cax31", ArchARM64},
		{"cax41", ArchARM64},

		// Edge cases
		{"", ArchAMD64},      // Empty string defaults to AMD64
		{"ca", ArchAMD64},    // Too short for CAX prefix
		{"CAX11", ArchAMD64}, // Case sensitive - uppercase doesn't match
	}

	for _, tt := range tests {
		t.Run(tt.serverType, func(t *testing.T) {
			t.Parallel()
			result := DetectArchitecture(tt.serverType)
			if result != tt.expected {
				t.Errorf("DetectArchitecture(%q) = %q, want %q", tt.serverType, result, tt.expected)
			}
		})
	}
}

func TestGetDefaultServerType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		arch     Architecture
		expected string
	}{
		{ArchAMD64, "cpx22"},
		{ArchARM64, "cax11"},
		{"unknown", "cpx22"}, // Unknown defaults to AMD64 server type
	}

	for _, tt := range tests {
		t.Run(string(tt.arch), func(t *testing.T) {
			t.Parallel()
			result := GetDefaultServerType(tt.arch)
			if result != tt.expected {
				t.Errorf("GetDefaultServerType(%q) = %q, want %q", tt.arch, result, tt.expected)
			}
		})
	}
}

func TestArchitecture_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		arch     Architecture
		expected string
	}{
		{ArchAMD64, "amd64"},
		{ArchARM64, "arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			result := tt.arch.String()
			if result != tt.expected {
				t.Errorf("Architecture.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDetectArchitecture_RoundTrip(t *testing.T) {
	t.Parallel(
	// Test that GetDefaultServerType returns a server type
	// that DetectArchitecture correctly identifies
	)

	archs := []Architecture{ArchAMD64, ArchARM64}

	for _, arch := range archs {
		t.Run(string(arch), func(t *testing.T) {
			t.Parallel()
			serverType := GetDefaultServerType(arch)
			detectedArch := DetectArchitecture(serverType)
			if detectedArch != arch {
				t.Errorf("Round-trip failed: arch=%q -> serverType=%q -> detectedArch=%q",
					arch, serverType, detectedArch)
			}
		})
	}
}
