package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCost(t *testing.T) {
	cmd := Cost()
	require.NotNil(t, cmd)
	assert.Equal(t, "cost", cmd.Use)
	assert.Equal(t, "Show current and planned cluster cost", cmd.Short)
	assert.NotNil(t, cmd.RunE)
}

func TestCostFlags(t *testing.T) {
	cmd := Cost()

	config := cmd.Flags().Lookup("config")
	require.NotNil(t, config)
	assert.Equal(t, "c", config.Shorthand)

	jsonFlag := cmd.Flags().Lookup("json")
	require.NotNil(t, jsonFlag)

	s3 := cmd.Flags().Lookup("s3-storage-gb")
	require.NotNil(t, s3)
	assert.Equal(t, "100", s3.DefValue)
}
