//go:build e2e

package e2e

import (
	"os"
	"strings"
)

// E2EConfig controls which phases of E2E tests to run.
type E2EConfig struct {
	// Phase control
	SkipSnapshots      bool
	SkipCluster        bool
	SkipAddons         bool
	SkipAddonsAdvanced bool
	SkipScale          bool
	SkipUpgrade        bool

	// Cluster reuse
	ReuseCluster   bool   // Use existing cluster instead of creating new one
	ClusterName    string // Name of cluster to reuse (if ReuseCluster=true)
	KubeconfigPath string // Path to existing kubeconfig (if ReuseCluster=true)

	// Snapshot management
	KeepSnapshots bool

	// Version control for upgrade tests
	InitialTalosVersion string
	TargetTalosVersion  string
	InitialK8sVersion   string
	TargetK8sVersion    string
}

// LoadE2EConfig loads configuration from environment variables.
func LoadE2EConfig() *E2EConfig {
	return &E2EConfig{
		// Phase control
		SkipSnapshots:      getEnvBool("E2E_SKIP_SNAPSHOTS"),
		SkipCluster:        getEnvBool("E2E_SKIP_CLUSTER"),
		SkipAddons:         getEnvBool("E2E_SKIP_ADDONS"),
		SkipAddonsAdvanced: getEnvBool("E2E_SKIP_ADDONS_ADVANCED"),
		SkipScale:          getEnvBool("E2E_SKIP_SCALE"),
		SkipUpgrade:        getEnvBool("E2E_SKIP_UPGRADE"),

		// Cluster reuse
		ReuseCluster:   getEnvBool("E2E_REUSE_CLUSTER"),
		ClusterName:    os.Getenv("E2E_CLUSTER_NAME"),
		KubeconfigPath: os.Getenv("E2E_KUBECONFIG_PATH"),

		// Snapshot management
		KeepSnapshots: getEnvBool("E2E_KEEP_SNAPSHOTS"),

		// Version control (with defaults)
		InitialTalosVersion: getEnvOrDefault("E2E_INITIAL_TALOS_VERSION", "v1.8.2"),
		TargetTalosVersion:  getEnvOrDefault("E2E_TARGET_TALOS_VERSION", "v1.8.3"),
		InitialK8sVersion:   getEnvOrDefault("E2E_INITIAL_K8S_VERSION", "v1.30.0"),
		TargetK8sVersion:    getEnvOrDefault("E2E_TARGET_K8S_VERSION", "v1.31.0"),
	}
}

// getEnvBool returns true if the environment variable is set to "true" (case-insensitive).
func getEnvBool(key string) bool {
	val := strings.ToLower(os.Getenv(key))
	return val == "true" || val == "1" || val == "yes"
}

// getEnvOrDefault returns the environment variable value or a default if not set.
func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// RunPhases returns which phases should be run based on config.
func (c *E2EConfig) RunPhases() []string {
	phases := []string{}
	if !c.SkipSnapshots {
		phases = append(phases, "snapshots")
	}
	if !c.SkipCluster {
		phases = append(phases, "cluster")
	}
	if !c.SkipAddons {
		phases = append(phases, "addons")
	}
	if !c.SkipAddonsAdvanced {
		phases = append(phases, "addons-advanced")
	}
	if !c.SkipScale {
		phases = append(phases, "scale")
	}
	if !c.SkipUpgrade {
		phases = append(phases, "upgrade")
	}
	return phases
}
