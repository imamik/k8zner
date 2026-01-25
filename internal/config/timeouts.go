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

	// Cluster provisioning timeouts
	PortWait      time.Duration // Timeout for waiting for a port to be reachable
	NodeReady     time.Duration // Timeout for waiting for a node to become ready
	Kubeconfig    time.Duration // Timeout for waiting for kubeconfig to be available
	TalosAPI      time.Duration // Timeout for Talos API operations
	PortPoll      time.Duration // Interval for polling port connectivity
	NodeReadyPoll time.Duration // Interval for polling node readiness
	DialTimeout   time.Duration // Timeout for TCP dial attempts
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

		// Cluster provisioning timeouts
		PortWait:      parseDuration("HCLOUD_TIMEOUT_PORT_WAIT", 2*time.Minute),
		NodeReady:     parseDuration("HCLOUD_TIMEOUT_NODE_READY", 10*time.Minute),
		Kubeconfig:    parseDuration("HCLOUD_TIMEOUT_KUBECONFIG", 15*time.Minute),
		TalosAPI:      parseDuration("HCLOUD_TIMEOUT_TALOS_API", 10*time.Minute),
		PortPoll:      parseDuration("HCLOUD_TIMEOUT_PORT_POLL", 5*time.Second),
		NodeReadyPoll: parseDuration("HCLOUD_TIMEOUT_NODE_READY_POLL", 10*time.Second),
		DialTimeout:   parseDuration("HCLOUD_TIMEOUT_DIAL", 2*time.Second),
	}
}

// TestTimeouts returns timeouts suitable for testing (very short).
func TestTimeouts() *Timeouts {
	return &Timeouts{
		ServerCreate:      100 * time.Millisecond,
		ServerIP:          100 * time.Millisecond,
		Delete:            100 * time.Millisecond,
		Bootstrap:         100 * time.Millisecond,
		ImageWait:         100 * time.Millisecond,
		RetryMaxAttempts:  2,
		RetryInitialDelay: 10 * time.Millisecond,

		PortWait:      100 * time.Millisecond,
		NodeReady:     100 * time.Millisecond,
		Kubeconfig:    100 * time.Millisecond,
		TalosAPI:      100 * time.Millisecond,
		PortPoll:      10 * time.Millisecond,
		NodeReadyPoll: 10 * time.Millisecond,
		DialTimeout:   50 * time.Millisecond,
	}
}

// parseDuration parses a duration from an environment variable.
// If the variable is not set or parsing fails, the default value is returned silently.
// This implements graceful degradation - invalid configuration does not cause errors.
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
// If the variable is not set or parsing fails, the default value is returned silently.
// This implements graceful degradation - invalid configuration does not cause errors.
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
