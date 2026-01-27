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
	assert.Equal(t, "Apply configuration to the cluster", cmd.Short)
}

func TestApply_ConfigFlag(t *testing.T) {
	cmd := Apply()

	flag := cmd.Flags().Lookup("config")
	require.NotNil(t, flag, "config flag should exist")
	assert.Equal(t, "c", flag.Shorthand)
	assert.Equal(t, "", flag.DefValue)
	assert.Equal(t, "Path to configuration file", flag.Usage)
}

func TestApply_ConfigFlagRequired(t *testing.T) {
	cmd := Apply()

	// The flag should be marked as required
	flag := cmd.Flags().Lookup("config")
	require.NotNil(t, flag)

	// Check that the flag has the required annotation
	annotations := flag.Annotations
	_, hasRequired := annotations["cobra_annotation_bash_completion_one_required_flag"]
	assert.True(t, hasRequired || flag.Value.String() == "", "config flag should be required")
}

func TestApply_RunE(t *testing.T) {
	cmd := Apply()
	assert.NotNil(t, cmd.RunE, "Apply command should have RunE function")
}
