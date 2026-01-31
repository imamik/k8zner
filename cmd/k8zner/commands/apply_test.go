package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApply(t *testing.T) {
	cmd := Apply()

	require.NotNil(t, cmd)
	assert.Equal(t, "apply", cmd.Use)
	assert.Equal(t, "Create or update the cluster", cmd.Short)
	assert.Contains(t, cmd.Long, "Kubernetes cluster")
	assert.Contains(t, cmd.Long, "k8zner init")
}

func TestApply_ConfigFlag(t *testing.T) {
	cmd := Apply()

	flag := cmd.Flags().Lookup("config")
	require.NotNil(t, flag, "config flag should exist")
	assert.Equal(t, "c", flag.Shorthand)
	assert.Equal(t, "", flag.DefValue)
	assert.Contains(t, flag.Usage, "k8zner.yaml")
}

func TestApply_ConfigFlagOptional(t *testing.T) {
	cmd := Apply()

	// The flag should NOT be marked as required (uses default k8zner.yaml)
	flag := cmd.Flags().Lookup("config")
	require.NotNil(t, flag)

	// Check that the flag does NOT have the required annotation
	annotations := flag.Annotations
	_, hasRequired := annotations["cobra_annotation_bash_completion_one_required_flag"]
	assert.False(t, hasRequired, "config flag should be optional")
}

func TestApply_RunE(t *testing.T) {
	cmd := Apply()
	assert.NotNil(t, cmd.RunE, "Apply command should have RunE function")
}
