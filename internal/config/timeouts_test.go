package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadTimeouts_Defaults(t *testing.T) {
	// Clear any existing environment variables
	clearTimeoutEnvVars()

	timeouts := LoadTimeouts()

	// Verify default values
	if timeouts.ServerCreate != 10*time.Minute {
		t.Errorf("Expected ServerCreate default 10m, got %v", timeouts.ServerCreate)
	}
	if timeouts.ServerIP != 60*time.Second {
		t.Errorf("Expected ServerIP default 60s, got %v", timeouts.ServerIP)
	}
	if timeouts.Delete != 5*time.Minute {
		t.Errorf("Expected Delete default 5m, got %v", timeouts.Delete)
	}
	if timeouts.Bootstrap != 10*time.Minute {
		t.Errorf("Expected Bootstrap default 10m, got %v", timeouts.Bootstrap)
	}
	if timeouts.ImageWait != 5*time.Minute {
		t.Errorf("Expected ImageWait default 5m, got %v", timeouts.ImageWait)
	}
	if timeouts.RetryMaxAttempts != 5 {
		t.Errorf("Expected RetryMaxAttempts default 5, got %d", timeouts.RetryMaxAttempts)
	}
	if timeouts.RetryInitialDelay != 1*time.Second {
		t.Errorf("Expected RetryInitialDelay default 1s, got %v", timeouts.RetryInitialDelay)
	}
}

func TestLoadTimeouts_EnvVars(t *testing.T) {
	// Clear any existing environment variables
	clearTimeoutEnvVars()

	// Set custom values
	t.Setenv("HCLOUD_TIMEOUT_SERVER_CREATE", "15m")
	t.Setenv("HCLOUD_TIMEOUT_SERVER_IP", "90s")
	t.Setenv("HCLOUD_TIMEOUT_DELETE", "3m")
	t.Setenv("HCLOUD_TIMEOUT_BOOTSTRAP", "20m")
	t.Setenv("HCLOUD_TIMEOUT_IMAGE_WAIT", "7m")
	t.Setenv("HCLOUD_RETRY_MAX_ATTEMPTS", "10")
	t.Setenv("HCLOUD_RETRY_INITIAL_DELAY", "2s")

	timeouts := LoadTimeouts()

	// Verify custom values are used
	if timeouts.ServerCreate != 15*time.Minute {
		t.Errorf("Expected ServerCreate 15m, got %v", timeouts.ServerCreate)
	}
	if timeouts.ServerIP != 90*time.Second {
		t.Errorf("Expected ServerIP 90s, got %v", timeouts.ServerIP)
	}
	if timeouts.Delete != 3*time.Minute {
		t.Errorf("Expected Delete 3m, got %v", timeouts.Delete)
	}
	if timeouts.Bootstrap != 20*time.Minute {
		t.Errorf("Expected Bootstrap 20m, got %v", timeouts.Bootstrap)
	}
	if timeouts.ImageWait != 7*time.Minute {
		t.Errorf("Expected ImageWait 7m, got %v", timeouts.ImageWait)
	}
	if timeouts.RetryMaxAttempts != 10 {
		t.Errorf("Expected RetryMaxAttempts 10, got %d", timeouts.RetryMaxAttempts)
	}
	if timeouts.RetryInitialDelay != 2*time.Second {
		t.Errorf("Expected RetryInitialDelay 2s, got %v", timeouts.RetryInitialDelay)
	}
}

func TestLoadTimeouts_InvalidEnvVars(t *testing.T) {
	// Clear any existing environment variables
	clearTimeoutEnvVars()

	// Set invalid values
	t.Setenv("HCLOUD_TIMEOUT_SERVER_CREATE", "invalid")
	t.Setenv("HCLOUD_TIMEOUT_SERVER_IP", "not-a-duration")
	t.Setenv("HCLOUD_RETRY_MAX_ATTEMPTS", "not-a-number")

	timeouts := LoadTimeouts()

	// Should fall back to defaults when parsing fails
	if timeouts.ServerCreate != 10*time.Minute {
		t.Errorf("Expected ServerCreate default 10m (invalid env var), got %v", timeouts.ServerCreate)
	}
	if timeouts.ServerIP != 60*time.Second {
		t.Errorf("Expected ServerIP default 60s (invalid env var), got %v", timeouts.ServerIP)
	}
	if timeouts.RetryMaxAttempts != 5 {
		t.Errorf("Expected RetryMaxAttempts default 5 (invalid env var), got %d", timeouts.RetryMaxAttempts)
	}
}

func TestLoadTimeouts_PartialEnvVars(t *testing.T) {
	// Clear any existing environment variables
	clearTimeoutEnvVars()

	// Set only some values
	t.Setenv("HCLOUD_TIMEOUT_SERVER_IP", "120s")
	t.Setenv("HCLOUD_RETRY_MAX_ATTEMPTS", "3")

	timeouts := LoadTimeouts()

	// Custom values should be used where set
	if timeouts.ServerIP != 120*time.Second {
		t.Errorf("Expected ServerIP 120s, got %v", timeouts.ServerIP)
	}
	if timeouts.RetryMaxAttempts != 3 {
		t.Errorf("Expected RetryMaxAttempts 3, got %d", timeouts.RetryMaxAttempts)
	}

	// Defaults should be used for unset values
	if timeouts.ServerCreate != 10*time.Minute {
		t.Errorf("Expected ServerCreate default 10m, got %v", timeouts.ServerCreate)
	}
	if timeouts.Delete != 5*time.Minute {
		t.Errorf("Expected Delete default 5m, got %v", timeouts.Delete)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name       string
		envVar     string
		envValue   string
		defaultVal time.Duration
		expected   time.Duration
	}{
		{
			name:       "Valid duration",
			envVar:     "TEST_DURATION",
			envValue:   "5m",
			defaultVal: 1 * time.Minute,
			expected:   5 * time.Minute,
		},
		{
			name:       "Empty value",
			envVar:     "TEST_DURATION",
			envValue:   "",
			defaultVal: 1 * time.Minute,
			expected:   1 * time.Minute,
		},
		{
			name:       "Invalid value",
			envVar:     "TEST_DURATION",
			envValue:   "invalid",
			defaultVal: 1 * time.Minute,
			expected:   1 * time.Minute,
		},
		{
			name:       "Complex duration",
			envVar:     "TEST_DURATION",
			envValue:   "1h30m45s",
			defaultVal: 1 * time.Minute,
			expected:   1*time.Hour + 30*time.Minute + 45*time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.envVar, tt.envValue)
			} else {
				if err := os.Unsetenv(tt.envVar); err != nil {
					t.Fatalf("Failed to unset env var: %v", err)
				}
			}

			result := parseDuration(tt.envVar, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		name       string
		envVar     string
		envValue   string
		defaultVal int
		expected   int
	}{
		{
			name:       "Valid integer",
			envVar:     "TEST_INT",
			envValue:   "42",
			defaultVal: 10,
			expected:   42,
		},
		{
			name:       "Empty value",
			envVar:     "TEST_INT",
			envValue:   "",
			defaultVal: 10,
			expected:   10,
		},
		{
			name:       "Invalid value",
			envVar:     "TEST_INT",
			envValue:   "not-a-number",
			defaultVal: 10,
			expected:   10,
		},
		{
			name:       "Zero value",
			envVar:     "TEST_INT",
			envValue:   "0",
			defaultVal: 10,
			expected:   0,
		},
		{
			name:       "Negative value",
			envVar:     "TEST_INT",
			envValue:   "-5",
			defaultVal: 10,
			expected:   -5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.envVar, tt.envValue)
			} else {
				if err := os.Unsetenv(tt.envVar); err != nil {
					t.Fatalf("Failed to unset env var: %v", err)
				}
			}

			result := parseInt(tt.envVar, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// clearTimeoutEnvVars clears all timeout-related environment variables
func clearTimeoutEnvVars() {
	_ = os.Unsetenv("HCLOUD_TIMEOUT_SERVER_CREATE")
	_ = os.Unsetenv("HCLOUD_TIMEOUT_SERVER_IP")
	_ = os.Unsetenv("HCLOUD_TIMEOUT_DELETE")
	_ = os.Unsetenv("HCLOUD_TIMEOUT_BOOTSTRAP")
	_ = os.Unsetenv("HCLOUD_TIMEOUT_IMAGE_WAIT")
	_ = os.Unsetenv("HCLOUD_RETRY_MAX_ATTEMPTS")
	_ = os.Unsetenv("HCLOUD_RETRY_INITIAL_DELAY")
}
