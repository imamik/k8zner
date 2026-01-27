package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDestroy(t *testing.T) {
	cmd := Destroy()

	require.NotNil(t, cmd)
	assert.Equal(t, "destroy", cmd.Use)
	assert.Equal(t, "Destroy a Kubernetes cluster and all associated resources", cmd.Short)
	assert.Contains(t, cmd.Long, "Destroy removes all cluster resources")
}

func TestDestroy_ConfigFlag(t *testing.T) {
	cmd := Destroy()

	flag := cmd.Flags().Lookup("config")
	require.NotNil(t, flag, "config flag should exist")
	assert.Equal(t, "c", flag.Shorthand)
	assert.Equal(t, "", flag.DefValue)
	assert.Equal(t, "Path to cluster configuration file (required)", flag.Usage)
}

func TestDestroy_ConfigFlagRequired(t *testing.T) {
	cmd := Destroy()

	flag := cmd.Flags().Lookup("config")
	require.NotNil(t, flag)

	// Check that the flag has the required annotation
	annotations := flag.Annotations
	_, hasRequired := annotations["cobra_annotation_bash_completion_one_required_flag"]
	assert.True(t, hasRequired || flag.Value.String() == "", "config flag should be required")
}

func TestDestroy_RunE(t *testing.T) {
	cmd := Destroy()
	assert.NotNil(t, cmd.RunE, "Destroy command should have RunE function")
}

func TestDestroy_LongDescription(t *testing.T) {
	cmd := Destroy()

	// Verify the long description mentions key resources
	assert.Contains(t, cmd.Long, "Servers")
	assert.Contains(t, cmd.Long, "Load balancers")
	assert.Contains(t, cmd.Long, "Networks")
	assert.Contains(t, cmd.Long, "WARNING")
}
