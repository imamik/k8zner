package config

import (
	"os"
	"strconv"
	"time"
)

// Timeouts holds all configurable timeout values.
// These values can be customized via environment variables.
type Timeouts struct {
	ServerCreate      time.Duration // Timeout for server creation operations
	ServerIP          time.Duration // Timeout for waiting for server IP assignment
	Delete            time.Duration // Timeout for all delete operations
	Bootstrap         time.Duration // Timeout for cluster bootstrap operations
	ImageWait         time.Duration // Timeout for waiting for image availability
	RetryMaxAttempts  int           // Maximum number of retry attempts
	RetryInitialDelay time.Duration // Initial delay between retries
}

// LoadTimeouts loads timeout configuration from environment variables.
// If an environment variable is not set or invalid, a default value is used.
//
// Environment Variables:
//   - HCLOUD_TIMEOUT_SERVER_CREATE (default: 10m)
//   - HCLOUD_TIMEOUT_SERVER_IP (default: 60s)
//   - HCLOUD_TIMEOUT_DELETE (default: 5m)
//   - HCLOUD_TIMEOUT_BOOTSTRAP (default: 10m)
//   - HCLOUD_TIMEOUT_IMAGE_WAIT (default: 5m)
//   - HCLOUD_RETRY_MAX_ATTEMPTS (default: 5)
//   - HCLOUD_RETRY_INITIAL_DELAY (default: 1s)
func LoadTimeouts() *Timeouts {
	return &Timeouts{
		ServerCreate:      parseDuration("HCLOUD_TIMEOUT_SERVER_CREATE", 10*time.Minute),
		ServerIP:          parseDuration("HCLOUD_TIMEOUT_SERVER_IP", 60*time.Second),
		Delete:            parseDuration("HCLOUD_TIMEOUT_DELETE", 5*time.Minute),
		Bootstrap:         parseDuration("HCLOUD_TIMEOUT_BOOTSTRAP", 10*time.Minute),
		ImageWait:         parseDuration("HCLOUD_TIMEOUT_IMAGE_WAIT", 5*time.Minute),
		RetryMaxAttempts:  parseInt("HCLOUD_RETRY_MAX_ATTEMPTS", 5),
		RetryInitialDelay: parseDuration("HCLOUD_RETRY_INITIAL_DELAY", 1*time.Second),
	}
}

// parseDuration parses a duration from an environment variable.
// If the variable is not set or parsing fails, the default value is returned.
func parseDuration(envVar string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(envVar)
	if val == "" {
		return defaultVal
	}

	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}

	return d
}

// parseInt parses an integer from an environment variable.
// If the variable is not set or parsing fails, the default value is returned.
func parseInt(envVar string, defaultVal int) int {
	val := os.Getenv(envVar)
	if val == "" {
		return defaultVal
	}

	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}

	return i
}
