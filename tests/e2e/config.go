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
	SkipMonitoring     bool
	SkipScale          bool
	SkipOperatorScale  bool // Operator-centric scaling via CRD
	SkipSelfHealing    bool

	// Cluster reuse
	ReuseCluster   bool   // Use existing cluster instead of creating new one
	ClusterName    string // Name of cluster to reuse (if ReuseCluster=true)
	KubeconfigPath string // Path to existing kubeconfig (if ReuseCluster=true)

	// Snapshot management
	KeepSnapshots bool
}

// LoadE2EConfig loads configuration from environment variables.
func LoadE2EConfig() *E2EConfig {
	return &E2EConfig{
		// Phase control
		SkipSnapshots:      getEnvBool("E2E_SKIP_SNAPSHOTS"),
		SkipCluster:        getEnvBool("E2E_SKIP_CLUSTER"),
		SkipAddons:         getEnvBool("E2E_SKIP_ADDONS"),
		SkipAddonsAdvanced: getEnvBool("E2E_SKIP_ADDONS_ADVANCED"),
		SkipMonitoring:     getEnvBool("E2E_SKIP_MONITORING"),
		SkipScale:          getEnvBool("E2E_SKIP_SCALE"),
		SkipOperatorScale:  getEnvBool("E2E_SKIP_OPERATOR_SCALE"),
		SkipSelfHealing:    getEnvBool("E2E_SKIP_SELF_HEALING"),

		// Cluster reuse
		ReuseCluster:   getEnvBool("E2E_REUSE_CLUSTER"),
		ClusterName:    os.Getenv("E2E_CLUSTER_NAME"),
		KubeconfigPath: os.Getenv("E2E_KUBECONFIG_PATH"),

		// Snapshot management
		KeepSnapshots: getEnvBool("E2E_KEEP_SNAPSHOTS"),
	}
}

// getEnvBool returns true if the environment variable is set to "true" (case-insensitive).
func getEnvBool(key string) bool {
	val := strings.ToLower(os.Getenv(key))
	return val == "true" || val == "1" || val == "yes"
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
	if !c.SkipMonitoring {
		phases = append(phases, "monitoring")
	}
	if !c.SkipScale {
		phases = append(phases, "scale")
	}
	if !c.SkipOperatorScale {
		phases = append(phases, "operator-scale")
	}
	if !c.SkipSelfHealing {
		phases = append(phases, "self-healing")
	}
	return phases
}
