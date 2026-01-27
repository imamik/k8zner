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
	assert.Equal(t, "Interactively create a cluster configuration", cmd.Short)
	assert.Contains(t, cmd.Long, "Interactively create a cluster configuration file")
}

func TestInit_OutputFlag(t *testing.T) {
	cmd := Init()

	flag := cmd.Flags().Lookup("output")
	require.NotNil(t, flag, "output flag should exist")
	assert.Equal(t, "o", flag.Shorthand)
	assert.Equal(t, "cluster.yaml", flag.DefValue)
	assert.Equal(t, "Output file path", flag.Usage)
}

func TestInit_AdvancedFlag(t *testing.T) {
	cmd := Init()

	flag := cmd.Flags().Lookup("advanced")
	require.NotNil(t, flag, "advanced flag should exist")
	assert.Equal(t, "a", flag.Shorthand)
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "Show advanced configuration options", flag.Usage)
}

func TestInit_FullFlag(t *testing.T) {
	cmd := Init()

	flag := cmd.Flags().Lookup("full")
	require.NotNil(t, flag, "full flag should exist")
	assert.Equal(t, "f", flag.Shorthand)
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "Output full YAML with all options", flag.Usage)
}

func TestInit_RunE(t *testing.T) {
	cmd := Init()
	assert.NotNil(t, cmd.RunE, "Init command should have RunE function")
}
