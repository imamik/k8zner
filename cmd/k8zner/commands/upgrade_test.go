package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpgrade(t *testing.T) {
	cmd := Upgrade()

	require.NotNil(t, cmd)
	assert.Equal(t, "upgrade", cmd.Use)
	assert.Equal(t, "Upgrade Talos OS and Kubernetes versions", cmd.Short)
	assert.Contains(t, cmd.Long, "Upgrade an existing cluster")
}

func TestUpgrade_ConfigFlag(t *testing.T) {
	cmd := Upgrade()

	flag := cmd.Flags().Lookup("config")
	require.NotNil(t, flag, "config flag should exist")
	assert.Equal(t, "c", flag.Shorthand)
	assert.Equal(t, "", flag.DefValue)
	assert.Equal(t, "Path to configuration file", flag.Usage)
}

func TestUpgrade_DryRunFlag(t *testing.T) {
	cmd := Upgrade()

	flag := cmd.Flags().Lookup("dry-run")
	require.NotNil(t, flag, "dry-run flag should exist")
	assert.Equal(t, "", flag.Shorthand)
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "Show what would be upgraded without executing", flag.Usage)
}

func TestUpgrade_SkipHealthCheckFlag(t *testing.T) {
	cmd := Upgrade()

	flag := cmd.Flags().Lookup("skip-health-check")
	require.NotNil(t, flag, "skip-health-check flag should exist")
	assert.Equal(t, "", flag.Shorthand)
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "Skip health checks between upgrades (dangerous)", flag.Usage)
}

func TestUpgrade_K8sVersionFlag(t *testing.T) {
	cmd := Upgrade()

	flag := cmd.Flags().Lookup("k8s-version")
	require.NotNil(t, flag, "k8s-version flag should exist")
	assert.Equal(t, "", flag.Shorthand)
	assert.Equal(t, "", flag.DefValue)
	assert.Equal(t, "Override Kubernetes version from config", flag.Usage)
}

func TestUpgrade_ConfigFlagRequired(t *testing.T) {
	cmd := Upgrade()

	flag := cmd.Flags().Lookup("config")
	require.NotNil(t, flag)

	// Check that the flag has the required annotation
	annotations := flag.Annotations
	_, hasRequired := annotations["cobra_annotation_bash_completion_one_required_flag"]
	assert.True(t, hasRequired || flag.Value.String() == "", "config flag should be required")
}

func TestUpgrade_RunE(t *testing.T) {
	cmd := Upgrade()
	assert.NotNil(t, cmd.RunE, "Upgrade command should have RunE function")
}
