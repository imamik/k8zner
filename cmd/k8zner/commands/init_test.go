package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	cmd := Init()

	require.NotNil(t, cmd)
	assert.Equal(t, "init", cmd.Use)
	assert.Equal(t, "Create a cluster configuration interactively", cmd.Short)
	assert.Contains(t, cmd.Long, "6 questions")
	assert.Contains(t, cmd.Long, "Talos Linux")
	assert.Contains(t, cmd.Long, "IPv6-only")
}

func TestInit_OutputFlag(t *testing.T) {
	cmd := Init()

	flag := cmd.Flags().Lookup("output")
	require.NotNil(t, flag, "output flag should exist")
	assert.Equal(t, "o", flag.Shorthand)
	assert.Equal(t, "k8zner.yaml", flag.DefValue, "default should be k8zner.yaml")
	assert.Equal(t, "Output file path", flag.Usage)
}

func TestInit_NoAdvancedFlag(t *testing.T) {
	cmd := Init()

	// Advanced flag should no longer exist in v2
	flag := cmd.Flags().Lookup("advanced")
	assert.Nil(t, flag, "advanced flag should not exist in v2")
}

func TestInit_NoFullFlag(t *testing.T) {
	cmd := Init()

	// Full flag should no longer exist in v2
	flag := cmd.Flags().Lookup("full")
	assert.Nil(t, flag, "full flag should not exist in v2")
}

func TestInit_RunE(t *testing.T) {
	cmd := Init()
	assert.NotNil(t, cmd.RunE, "Init command should have RunE function")
}
